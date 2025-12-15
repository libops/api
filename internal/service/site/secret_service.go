package site

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/libops/api/internal/audit"
	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/service/organization"
	"github.com/libops/api/internal/validation"
	"github.com/libops/api/internal/vault"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// SiteSecretService implements the SiteSecretService API.
type SiteSecretService struct {
	db          db.Querier
	auditLogger *audit.Logger
}

// Compile-time check to ensure SiteSecretService implements the interface.
var _ libopsv1connect.SiteSecretServiceHandler = (*SiteSecretService)(nil)

// NewSiteSecretService creates a new SiteSecretService instance.
func NewSiteSecretService(querier db.Querier, auditLogger *audit.Logger) *SiteSecretService {
	return &SiteSecretService{
		db:          querier,
		auditLogger: auditLogger,
	}
}

// GetSiteVaultClient returns or creates a Vault client for the site's organization.
func (s *SiteSecretService) GetSiteVaultClient(ctx context.Context, organizationID int64) (*vault.Client, error) {
	// Get organization's libops project (where vault server runs)
	project, err := s.db.GetOrganizationProjectByOrganizationID(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization project: %w", err)
	}

	// Parse project number from string
	var projectNumber int64
	if project.GcpProjectNumber.Valid {
		_, _ = fmt.Sscanf(project.GcpProjectNumber.String, "%d", &projectNumber)
	}

	// Get region from project
	region := "us-central1" // default
	if project.GcpRegion.Valid && project.GcpRegion.String != "" {
		region = project.GcpRegion.String
	}

	client, err := vault.NewCustomerVaultClient(ctx, organizationID, projectNumber, region)
	if err != nil {
		return nil, fmt.Errorf("failed to create customer vault client: %w", err)
	}

	return client, nil
}

// CreateSiteSecret creates a new site-level secret.
func (s *SiteSecretService) CreateSiteSecret(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateSiteSecretRequest],
) (*connect.Response[libopsv1.CreateSiteSecretResponse], error) {
	// 1. Parse and validate request
	if err := organization.ValidateSecretName(req.Msg.Name); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	// Validate value
	if err := validation.RequiredString("value", req.Msg.Value); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if len(req.Msg.Value) > 65536 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("value too long (max 64KB)"))
	}

	siteUUID, err := uuid.Parse(req.Msg.SiteId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id"))
	}

	// 2. Get site from DB
	site, err := s.db.GetSite(ctx, siteUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// 3. Get project to find organization ID
	project, err := s.db.GetProjectByID(ctx, site.ProjectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project: %w", err))
	}

	// 4. Get user info (authorization already done by scope interceptor)
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok || userInfo == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	// 5. Check if secret already exists
	_, err = s.db.GetSiteSecretByName(ctx, db.GetSiteSecretByNameParams{
		SiteID: site.ID,
		Name:   req.Msg.Name,
	})
	if err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("secret %s already exists", req.Msg.Name))
	}
	if err != sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// 6. Build Vault path (uses site public ID)
	vaultPath := vault.BuildSiteSecretPath(siteUUID.String(), req.Msg.Name)

	// 7. Write to organization's Vault
	vaultClient, err := s.GetSiteVaultClient(ctx, project.OrganizationID)
	if err != nil {
		slog.Error("failed to get vault client", "err", err, "organization_id", project.OrganizationID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to access vault"))
	}

	err = vaultClient.WriteSecret(ctx, vaultPath, map[string]any{
		"value": req.Msg.Value,
	})
	if err != nil {
		slog.Error("failed to write secret to vault", "err", err, "path", vaultPath)
		// Audit log for vault failure
		s.auditLogger.Log(ctx, userInfo.AccountID, site.ID, audit.SiteEntityType, audit.SiteSecretCreateFailed, map[string]any{
			"secret_name": req.Msg.Name,
			"vault_path":  vaultPath,
			"error":       "vault_write_failed",
			"error_msg":   err.Error(),
		})
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write secret"))
	}

	// 8. Create database record
	secretUUID := uuid.New()
	now := time.Now().Unix()
	_, err = s.db.CreateSiteSecret(ctx, db.CreateSiteSecretParams{
		PublicID:  secretUUID.String(),
		SiteID:    site.ID,
		Name:      req.Msg.Name,
		VaultPath: vaultPath,
		Status:    db.NullSiteSecretsStatus{SiteSecretsStatus: db.SiteSecretsStatusActive, Valid: true},
		CreatedAt: now,
		UpdatedAt: now,
		CreatedBy: sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
		UpdatedBy: sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
	})
	if err != nil {
		// Rollback: delete from Vault
		_ = vaultClient.DeleteSecret(ctx, vaultPath)
		slog.Error("failed to create secret record", "err", err)
		// Audit log for database failure
		s.auditLogger.Log(ctx, userInfo.AccountID, site.ID, audit.SiteEntityType, audit.SiteSecretCreateFailed, map[string]any{
			"secret_name": req.Msg.Name,
			"vault_path":  vaultPath,
			"error":       "database_write_failed",
			"error_msg":   err.Error(),
		})
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create secret"))
	}

	// 9. Get created secret
	secret, err := s.db.GetSiteSecretByName(ctx, db.GetSiteSecretByNameParams{
		SiteID: site.ID,
		Name:   req.Msg.Name,
	})
	if err != nil {
		slog.Error("failed to retrieve created secret", "err", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to retrieve secret"))
	}

	// Audit log for success
	s.auditLogger.Log(ctx, userInfo.AccountID, secret.ID, audit.SiteEntityType, audit.SiteSecretCreateSuccess, map[string]any{
		"secret_id":   secret.PublicID,
		"secret_name": secret.Name,
		"vault_path":  secret.VaultPath,
	})

	// 10. Return response
	return connect.NewResponse(&libopsv1.CreateSiteSecretResponse{
		Secret: &libopsv1.SiteSecret{
			SecretId: secret.PublicID,
			SiteId:   siteUUID.String(),
			Name:     secret.Name,
			Status:   dbSiteStatusToProto(secret.Status),
		},
	}), nil
}

// GetSiteSecret retrieves a site secret by ID.
func (s *SiteSecretService) GetSiteSecret(
	ctx context.Context,
	req *connect.Request[libopsv1.GetSiteSecretRequest],
) (*connect.Response[libopsv1.GetSiteSecretResponse], error) {
	siteUUID, err := uuid.Parse(req.Msg.SiteId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id"))
	}

	secretUUID, err := uuid.Parse(req.Msg.SecretId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret_id"))
	}

	// Get site
	site, err := s.db.GetSite(ctx, siteUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Authorization already done by interceptor - just verify user is authenticated
	_, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	// Get secret
	secret, err := s.db.GetSiteSecretByPublicID(ctx, secretUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("secret not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Verify secret belongs to site
	if secret.SiteID != site.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("secret does not belong to site"))
	}

	return connect.NewResponse(&libopsv1.GetSiteSecretResponse{
		Secret: &libopsv1.SiteSecret{
			SecretId: secret.PublicID,
			SiteId:   siteUUID.String(),
			Name:     secret.Name,
			Status:   dbSiteStatusToProto(secret.Status),
		},
	}), nil
}

// ListSiteSecrets lists all secrets for a site.
func (s *SiteSecretService) ListSiteSecrets(
	ctx context.Context,
	req *connect.Request[libopsv1.ListSiteSecretsRequest],
) (*connect.Response[libopsv1.ListSiteSecretsResponse], error) {
	siteUUID, err := uuid.Parse(req.Msg.SiteId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id"))
	}

	// Get site
	site, err := s.db.GetSite(ctx, siteUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Authorization already done by interceptor - just verify user is authenticated
	_, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	// Pagination
	pageSize := int32(50)
	if req.Msg.PageSize > 0 && req.Msg.PageSize <= 100 {
		pageSize = req.Msg.PageSize
	}
	offset := int32(0)
	// TODO: Implement page token parsing

	// List secrets
	secrets, err := s.db.ListSiteSecrets(ctx, db.ListSiteSecretsParams{
		SiteID: site.ID,
		Limit:  pageSize,
		Offset: offset,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Convert to proto
	protoSecrets := make([]*libopsv1.SiteSecret, len(secrets))
	for i, secret := range secrets {
		protoSecrets[i] = &libopsv1.SiteSecret{
			SecretId: secret.PublicID,
			SiteId:   siteUUID.String(),
			Name:     secret.Name,
			Status:   dbSiteStatusToProto(secret.Status),
		}
	}

	return connect.NewResponse(&libopsv1.ListSiteSecretsResponse{
		Secrets:       protoSecrets,
		NextPageToken: "", // TODO: Implement pagination token
	}), nil
}

// UpdateSiteSecret updates a site secret value.
func (s *SiteSecretService) UpdateSiteSecret(
	ctx context.Context,
	req *connect.Request[libopsv1.UpdateSiteSecretRequest],
) (*connect.Response[libopsv1.UpdateSiteSecretResponse], error) {
	siteUUID, err := uuid.Parse(req.Msg.SiteId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id"))
	}

	secretUUID, err := uuid.Parse(req.Msg.SecretId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret_id"))
	}

	// Get site
	site, err := s.db.GetSite(ctx, siteUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Get project to find organization ID
	project, err := s.db.GetProjectByID(ctx, site.ProjectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project: %w", err))
	}

	// Authorization already done by interceptor - get user info for audit logging
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok || userInfo == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	// Get secret
	secret, err := s.db.GetSiteSecretByPublicID(ctx, secretUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("secret not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Verify secret belongs to site
	if secret.SiteID != site.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("secret does not belong to site"))
	}

	// Update value if provided
	if req.Msg.Value != nil && *req.Msg.Value != "" {
		if len(*req.Msg.Value) > 65536 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("value too long (max 64KB)"))
		}

		// Write to Vault
		vaultClient, err := s.GetSiteVaultClient(ctx, project.OrganizationID)
		if err != nil {
			slog.Error("failed to get vault client", "err", err)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to access vault"))
		}

		err = vaultClient.WriteSecret(ctx, secret.VaultPath, map[string]any{
			"value": *req.Msg.Value,
		})
		if err != nil {
			slog.Error("failed to update secret in vault", "err", err)
			// Audit log for vault failure
			s.auditLogger.Log(ctx, userInfo.AccountID, site.ID, audit.SiteEntityType, audit.SiteSecretUpdateFailed, map[string]any{
				"secret_id":   secret.PublicID,
				"secret_name": secret.Name,
				"vault_path":  secret.VaultPath,
				"error":       "vault_write_failed",
				"error_msg":   err.Error(),
			})
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update secret"))
		}

		// Update database timestamp
		now := time.Now().Unix()
		err = s.db.UpdateSiteSecret(ctx, db.UpdateSiteSecretParams{
			VaultPath: secret.VaultPath,
			UpdatedBy: sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
			UpdatedAt: now,
			ID:        secret.ID,
		})
		if err != nil {
			slog.Error("failed to update secret record", "err", err)
			// Audit log for database failure
			s.auditLogger.Log(ctx, userInfo.AccountID, site.ID, audit.SiteEntityType, audit.SiteSecretUpdateFailed, map[string]any{
				"secret_id":   secret.PublicID,
				"secret_name": secret.Name,
				"vault_path":  secret.VaultPath,
				"error":       "database_update_failed",
				"error_msg":   err.Error(),
			})
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update secret"))
		}
	}

	// Get updated secret
	secret, err = s.db.GetSiteSecretByPublicID(ctx, secretUUID.String())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Audit log for success
	s.auditLogger.Log(ctx, userInfo.AccountID, secret.ID, audit.SiteEntityType, audit.SiteSecretUpdateSuccess, map[string]any{
		"secret_id":   secret.PublicID,
		"secret_name": secret.Name,
		"vault_path":  secret.VaultPath,
	})

	return connect.NewResponse(&libopsv1.UpdateSiteSecretResponse{
		Secret: &libopsv1.SiteSecret{
			SecretId: secret.PublicID,
			SiteId:   siteUUID.String(),
			Name:     secret.Name,
			Status:   dbSiteStatusToProto(secret.Status),
		},
	}), nil
}

// DeleteSiteSecret deletes a site secret.
func (s *SiteSecretService) DeleteSiteSecret(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteSiteSecretRequest],
) (*connect.Response[emptypb.Empty], error) {
	siteUUID, err := uuid.Parse(req.Msg.SiteId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id"))
	}

	secretUUID, err := uuid.Parse(req.Msg.SecretId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret_id"))
	}

	// Get site
	site, err := s.db.GetSite(ctx, siteUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Get project to find organization ID
	project, err := s.db.GetProjectByID(ctx, site.ProjectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project: %w", err))
	}

	// Authorization already done by interceptor - get user info for audit logging
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok || userInfo == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	// Get secret
	secret, err := s.db.GetSiteSecretByPublicID(ctx, secretUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("secret not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Verify secret belongs to site
	if secret.SiteID != site.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("secret does not belong to site"))
	}

	// Delete from Vault
	vaultClient, err := s.GetSiteVaultClient(ctx, project.OrganizationID)
	if err != nil {
		slog.Error("failed to get vault client", "err", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to access vault"))
	}

	err = vaultClient.DeleteSecret(ctx, secret.VaultPath)
	if err != nil {
		slog.Error("failed to delete secret from vault", "err", err)
		// Audit log for vault failure
		s.auditLogger.Log(ctx, userInfo.AccountID, site.ID, audit.SiteEntityType, audit.SiteSecretDeleteFailed, map[string]any{
			"secret_id":   secret.PublicID,
			"secret_name": secret.Name,
			"vault_path":  secret.VaultPath,
			"error":       "vault_delete_failed",
			"error_msg":   err.Error(),
		})
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete secret"))
	}

	// Mark as deleted in database
	now := time.Now().Unix()
	err = s.db.DeleteSiteSecret(ctx, db.DeleteSiteSecretParams{
		UpdatedBy: sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
		UpdatedAt: now,
		ID:        secret.ID,
	})
	if err != nil {
		slog.Error("failed to delete secret record", "err", err)
		// Audit log for database failure
		s.auditLogger.Log(ctx, userInfo.AccountID, site.ID, audit.SiteEntityType, audit.SiteSecretDeleteFailed, map[string]any{
			"secret_id":   secret.PublicID,
			"secret_name": secret.Name,
			"vault_path":  secret.VaultPath,
			"error":       "database_delete_failed",
			"error_msg":   err.Error(),
		})
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete secret"))
	}

	// Audit log for success
	s.auditLogger.Log(ctx, userInfo.AccountID, secret.ID, audit.SiteEntityType, audit.SiteSecretDeleteSuccess, map[string]any{
		"secret_id":   secret.PublicID,
		"secret_name": secret.Name,
		"vault_path":  secret.VaultPath,
	})

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// dbSiteStatusToProto converts database site secret status to proto status.
func dbSiteStatusToProto(status db.NullSiteSecretsStatus) commonv1.Status {
	if !status.Valid {
		return commonv1.Status_STATUS_UNSPECIFIED
	}

	switch status.SiteSecretsStatus {
	case db.SiteSecretsStatusActive:
		return commonv1.Status_STATUS_ACTIVE
	case db.SiteSecretsStatusProvisioning:
		return commonv1.Status_STATUS_PROVISIONING
	case db.SiteSecretsStatusFailed:
		return commonv1.Status_STATUS_FAILED
	case db.SiteSecretsStatusSuspended:
		return commonv1.Status_STATUS_SUSPENDED
	case db.SiteSecretsStatusDeleted:
		return commonv1.Status_STATUS_DELETED
	default:
		return commonv1.Status_STATUS_UNSPECIFIED
	}
}
