package organization

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"regexp"
	"time"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/libops/api/db"
	"github.com/libops/api/internal/audit"
	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/service"
	"github.com/libops/api/internal/validation"
	"github.com/libops/api/internal/vault"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

var secretNameRegex = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)

// OrganizationSecretService implements the OrganizationSecretService API.
type OrganizationSecretService struct {
	db          db.Querier
	auditLogger *audit.Logger
}

// Compile-time check to ensure OrganizationSecretService implements the interface.
var _ libopsv1connect.OrganizationSecretServiceHandler = (*OrganizationSecretService)(nil)

// NewOrganizationSecretService creates a new OrganizationSecretService instance.
func NewOrganizationSecretService(querier db.Querier, auditLogger *audit.Logger) *OrganizationSecretService {
	return &OrganizationSecretService{
		db:          querier,
		auditLogger: auditLogger,
	}
}

// GetOrganizationVaultClient returns or creates a Vault client for the organization.
func (s *OrganizationSecretService) GetOrganizationVaultClient(ctx context.Context, organizationID int64) (*vault.Client, error) {
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

// authorizeOrganizationSecretRead checks if user can read organization secrets.
func (s *OrganizationSecretService) authorizeOrganizationSecretRead(ctx context.Context, organizationID int64) (*auth.UserInfo, error) {
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok || userInfo == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	// Check organization membership
	_, err := s.db.GetOrganizationMemberByAccountAndOrganization(ctx, db.GetOrganizationMemberByAccountAndOrganizationParams{
		AccountID:      userInfo.AccountID,
		OrganizationID: organizationID,
	})
	if err == nil {
		return userInfo, nil
	}

	if err == sql.ErrNoRows {
		// Check relationships
		if relErr := service.CheckRelationshipAccess(ctx, s.db, userInfo.AccountID, organizationID, false); relErr != nil {
			return nil, relErr
		}
		return userInfo, nil
	}

	return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
}

// validateSecretName validates secret name format.
func ValidateSecretName(name string) error {
	if name == "" {
		return fmt.Errorf("name is required")
	}
	if len(name) > 255 {
		return fmt.Errorf("name too long (max 255 characters)")
	}
	if !secretNameRegex.MatchString(name) {
		return fmt.Errorf("name must match pattern ^[A-Z][A-Z0-9_]*$ (uppercase, starts with letter)")
	}
	return nil
}

// CreateOrganizationSecret creates a new organization-level secret.
func (s *OrganizationSecretService) CreateOrganizationSecret(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateOrganizationSecretRequest],
) (*connect.Response[libopsv1.CreateOrganizationSecretResponse], error) {
	// 1. Parse and validate request
	if err := ValidateSecretName(req.Msg.Name); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	// Validate value
	if err := validation.RequiredString("value", req.Msg.Value); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if len(req.Msg.Value) > 65536 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("value too long (max 64KB)"))
	}

	organizationUUID, err := uuid.Parse(req.Msg.OrganizationId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id"))
	}

	// 2. Get user info (authorization already done by scope interceptor)
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok || userInfo == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	// 3. Get organization from DB
	organization, err := s.db.GetOrganization(ctx, organizationUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// 4. Check if secret already exists
	_, err = s.db.GetOrganizationSecretByName(ctx, db.GetOrganizationSecretByNameParams{
		OrganizationID: organization.ID,
		Name:           req.Msg.Name,
	})
	if err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("secret %s already exists", req.Msg.Name))
	}
	if err != sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// 5. Build Vault path
	vaultPath := vault.BuildOrganizationSecretPath(req.Msg.Name)

	// 6. Write to organization's Vault
	vaultClient, err := s.GetOrganizationVaultClient(ctx, organization.ID)
	if err != nil {
		slog.Error("failed to get vault client", "err", err, "organization_id", organization.ID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to access vault"))
	}

	err = vaultClient.WriteSecret(ctx, vaultPath, map[string]any{
		"value": req.Msg.Value,
	})
	if err != nil {
		slog.Error("failed to write secret to vault", "err", err, "path", vaultPath)
		s.auditLogger.Log(ctx, userInfo.AccountID, organization.ID, audit.OrganizationEntityType, audit.OrganizationSecretCreateFailed, map[string]any{
			"secret_name": req.Msg.Name,
			"vault_path":  vaultPath,
			"error":       "vault_write_failed",
			"error_msg":   err.Error(),
		})
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write secret"))
	}

	// 7. Create database record
	secretUUID := uuid.New()
	now := time.Now().Unix()
	_, err = s.db.CreateOrganizationSecret(ctx, db.CreateOrganizationSecretParams{
		PublicID:       secretUUID.String(),
		OrganizationID: organization.ID,
		Name:           req.Msg.Name,
		VaultPath:      vaultPath,
		Status:         db.NullOrganizationSecretsStatus{OrganizationSecretsStatus: db.OrganizationSecretsStatusActive, Valid: true},
		CreatedAt:      now,
		UpdatedAt:      now,
		CreatedBy:      sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
		UpdatedBy:      sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
	})
	if err != nil {
		// Rollback: delete from Vault
		_ = vaultClient.DeleteSecret(ctx, vaultPath)
		slog.Error("failed to create secret record", "err", err)
		s.auditLogger.Log(ctx, userInfo.AccountID, organization.ID, audit.OrganizationEntityType, audit.OrganizationSecretCreateFailed, map[string]any{
			"secret_name": req.Msg.Name,
			"vault_path":  vaultPath,
			"error":       "database_write_failed",
			"error_msg":   err.Error(),
		})
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create secret"))
	}

	// 8. Get created secret
	secret, err := s.db.GetOrganizationSecretByName(ctx, db.GetOrganizationSecretByNameParams{
		OrganizationID: organization.ID,
		Name:           req.Msg.Name,
	})
	if err != nil {
		slog.Error("failed to retrieve created secret", "err", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to retrieve secret"))
	}

	// Audit log for success
	s.auditLogger.Log(ctx, userInfo.AccountID, secret.ID, audit.OrganizationEntityType, audit.OrganizationSecretCreateSuccess, map[string]any{
		"secret_id":   secret.PublicID,
		"secret_name": secret.Name,
		"vault_path":  vaultPath,
	})

	// 9. Return response
	return connect.NewResponse(&libopsv1.CreateOrganizationSecretResponse{
		Secret: &libopsv1.OrganizationSecret{
			SecretId:       secret.PublicID,
			OrganizationId: organizationUUID.String(),
			Name:           secret.Name,
			Status:         dbStatusToProto(secret.Status),
		},
	}), nil
}

// GetOrganizationSecret retrieves a organization secret by ID.
func (s *OrganizationSecretService) GetOrganizationSecret(
	ctx context.Context,
	req *connect.Request[libopsv1.GetOrganizationSecretRequest],
) (*connect.Response[libopsv1.GetOrganizationSecretResponse], error) {
	organizationUUID, err := uuid.Parse(req.Msg.OrganizationId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id"))
	}

	secretUUID, err := uuid.Parse(req.Msg.SecretId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret_id"))
	}

	// Get organization
	organization, err := s.db.GetOrganization(ctx, organizationUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Check user has any role in organization (read access to list secrets)
	_, err = s.authorizeOrganizationSecretRead(ctx, organization.ID)
	if err != nil {
		return nil, err
	}

	// Get secret
	secret, err := s.db.GetOrganizationSecretByPublicID(ctx, secretUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("secret not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Verify secret belongs to organization
	if secret.OrganizationID != organization.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("secret does not belong to organization"))
	}

	return connect.NewResponse(&libopsv1.GetOrganizationSecretResponse{
		Secret: &libopsv1.OrganizationSecret{
			SecretId:       secret.PublicID,
			OrganizationId: organizationUUID.String(),
			Name:           secret.Name,
			Status:         dbStatusToProto(secret.Status),
		},
	}), nil
}

// ListOrganizationSecrets lists all secrets for a organization.
func (s *OrganizationSecretService) ListOrganizationSecrets(
	ctx context.Context,
	req *connect.Request[libopsv1.ListOrganizationSecretsRequest],
) (*connect.Response[libopsv1.ListOrganizationSecretsResponse], error) {
	organizationUUID, err := uuid.Parse(req.Msg.OrganizationId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id"))
	}

	// Get organization
	organization, err := s.db.GetOrganization(ctx, organizationUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Check user has any role in organization
	_, err = s.authorizeOrganizationSecretRead(ctx, organization.ID)
	if err != nil {
		return nil, err
	}

	// Pagination
	pageSize := int32(50)
	if req.Msg.PageSize > 0 && req.Msg.PageSize <= 100 {
		pageSize = req.Msg.PageSize
	}
	offset := int32(0)
	// TODO: Implement page token parsing

	// List secrets
	secrets, err := s.db.ListOrganizationSecrets(ctx, db.ListOrganizationSecretsParams{
		OrganizationID: organization.ID,
		Limit:          pageSize,
		Offset:         offset,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Convert to proto
	protoSecrets := make([]*libopsv1.OrganizationSecret, len(secrets))
	for i, secret := range secrets {
		protoSecrets[i] = &libopsv1.OrganizationSecret{
			SecretId:       secret.PublicID,
			OrganizationId: organizationUUID.String(),
			Name:           secret.Name,
			Status:         dbStatusToProto(secret.Status),
		}
	}

	return connect.NewResponse(&libopsv1.ListOrganizationSecretsResponse{
		Secrets:       protoSecrets,
		NextPageToken: "", // TODO: Implement pagination token
	}), nil
}

// UpdateOrganizationSecret updates a organization secret value.
func (s *OrganizationSecretService) UpdateOrganizationSecret(
	ctx context.Context,
	req *connect.Request[libopsv1.UpdateOrganizationSecretRequest],
) (*connect.Response[libopsv1.UpdateOrganizationSecretResponse], error) {
	organizationUUID, err := uuid.Parse(req.Msg.OrganizationId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id"))
	}

	secretUUID, err := uuid.Parse(req.Msg.SecretId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret_id"))
	}

	// Get organization
	organization, err := s.db.GetOrganization(ctx, organizationUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Get user info (authorization already done by scope interceptor)
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok || userInfo == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	// Get secret
	secret, err := s.db.GetOrganizationSecretByPublicID(ctx, secretUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("secret not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Verify secret belongs to organization
	if secret.OrganizationID != organization.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("secret does not belong to organization"))
	}

	// Update value if provided
	if req.Msg.Value != nil && *req.Msg.Value != "" {
		if len(*req.Msg.Value) > 65536 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("value too long (max 64KB)"))
		}

		// Write to Vault
		vaultClient, err := s.GetOrganizationVaultClient(ctx, organization.ID)
		if err != nil {
			slog.Error("failed to get vault client", "err", err)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to access vault"))
		}

		err = vaultClient.WriteSecret(ctx, secret.VaultPath, map[string]any{
			"value": *req.Msg.Value,
		})
		if err != nil {
			slog.Error("failed to update secret in vault", "err", err)
			s.auditLogger.Log(ctx, userInfo.AccountID, secret.ID, audit.OrganizationEntityType, audit.OrganizationSecretUpdateFailed, map[string]any{
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
		err = s.db.UpdateOrganizationSecret(ctx, db.UpdateOrganizationSecretParams{
			VaultPath: secret.VaultPath,
			UpdatedBy: sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
			UpdatedAt: now,
			ID:        secret.ID,
		})
		if err != nil {
			slog.Error("failed to update secret record", "err", err)
			s.auditLogger.Log(ctx, userInfo.AccountID, secret.ID, audit.OrganizationEntityType, audit.OrganizationSecretUpdateFailed, map[string]any{
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
	secret, err = s.db.GetOrganizationSecretByPublicID(ctx, secretUUID.String())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Audit log for success
	s.auditLogger.Log(ctx, userInfo.AccountID, secret.ID, audit.OrganizationEntityType, audit.OrganizationSecretUpdateSuccess, map[string]any{
		"secret_id":   secret.PublicID,
		"secret_name": secret.Name,
		"vault_path":  secret.VaultPath,
	})

	return connect.NewResponse(&libopsv1.UpdateOrganizationSecretResponse{
		Secret: &libopsv1.OrganizationSecret{
			SecretId:       secret.PublicID,
			OrganizationId: organizationUUID.String(),
			Name:           secret.Name,
			Status:         dbStatusToProto(secret.Status),
		},
	}), nil
}

// DeleteOrganizationSecret deletes a organization secret.
func (s *OrganizationSecretService) DeleteOrganizationSecret(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteOrganizationSecretRequest],
) (*connect.Response[emptypb.Empty], error) {
	organizationUUID, err := uuid.Parse(req.Msg.OrganizationId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id"))
	}

	secretUUID, err := uuid.Parse(req.Msg.SecretId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret_id"))
	}

	// Get organization
	organization, err := s.db.GetOrganization(ctx, organizationUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Get user info (authorization already done by scope interceptor)
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok || userInfo == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	// Get secret
	secret, err := s.db.GetOrganizationSecretByPublicID(ctx, secretUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("secret not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Verify secret belongs to organization
	if secret.OrganizationID != organization.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("secret does not belong to organization"))
	}

	// Delete from Vault
	vaultClient, err := s.GetOrganizationVaultClient(ctx, organization.ID)
	if err != nil {
		slog.Error("failed to get vault client", "err", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to access vault"))
	}

	err = vaultClient.DeleteSecret(ctx, secret.VaultPath)
	if err != nil {
		slog.Error("failed to delete secret from vault", "err", err)
		// Audit log for vault failure
		s.auditLogger.Log(ctx, userInfo.AccountID, organization.ID, audit.OrganizationEntityType, audit.OrganizationSecretDeleteFailed, map[string]any{
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
	err = s.db.DeleteOrganizationSecret(ctx, db.DeleteOrganizationSecretParams{
		UpdatedBy: sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
		UpdatedAt: now,
		ID:        secret.ID,
	})
	if err != nil {
		slog.Error("failed to delete secret record", "err", err)
		// Audit log for database failure
		s.auditLogger.Log(ctx, userInfo.AccountID, organization.ID, audit.OrganizationEntityType, audit.OrganizationSecretDeleteFailed, map[string]any{
			"secret_id":   secret.PublicID,
			"secret_name": secret.Name,
			"vault_path":  secret.VaultPath,
			"error":       "database_delete_failed",
			"error_msg":   err.Error(),
		})
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete secret"))
	}

	// Audit log for success
	s.auditLogger.Log(ctx, userInfo.AccountID, secret.ID, audit.OrganizationEntityType, audit.OrganizationSecretDeleteSuccess, map[string]any{
		"secret_id":   secret.PublicID,
		"secret_name": secret.Name,
		"vault_path":  secret.VaultPath,
	})

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// dbStatusToProto converts database status to proto status.
func dbStatusToProto(status db.NullOrganizationSecretsStatus) commonv1.Status {
	if !status.Valid {
		return commonv1.Status_STATUS_UNSPECIFIED
	}

	switch status.OrganizationSecretsStatus {
	case db.OrganizationSecretsStatusActive:
		return commonv1.Status_STATUS_ACTIVE
	case db.OrganizationSecretsStatusProvisioning:
		return commonv1.Status_STATUS_PROVISIONING
	case db.OrganizationSecretsStatusFailed:
		return commonv1.Status_STATUS_FAILED
	case db.OrganizationSecretsStatusSuspended:
		return commonv1.Status_STATUS_SUSPENDED
	case db.OrganizationSecretsStatusDeleted:
		return commonv1.Status_STATUS_DELETED
	default:
		return commonv1.Status_STATUS_UNSPECIFIED
	}
}
