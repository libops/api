package project

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
	"github.com/libops/api/internal/service"
	"github.com/libops/api/internal/service/organization"
	"github.com/libops/api/internal/validation"
	"github.com/libops/api/internal/vault"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// ProjectSecretService implements the ProjectSecretService API.
type ProjectSecretService struct {
	db          db.Querier
	auditLogger *audit.Logger
}

// Compile-time check to ensure ProjectSecretService implements the interface.
var _ libopsv1connect.ProjectSecretServiceHandler = (*ProjectSecretService)(nil)

// NewProjectSecretService creates a new ProjectSecretService instance.
func NewProjectSecretService(querier db.Querier, auditLogger *audit.Logger) *ProjectSecretService {
	return &ProjectSecretService{
		db:          querier,
		auditLogger: auditLogger,
	}
}

// GetProjectVaultClient returns or creates a Vault client for the project's organization.
func (s *ProjectSecretService) GetProjectVaultClient(ctx context.Context, organizationID int64) (*vault.Client, error) {
	project, err := s.db.GetOrganizationProjectByOrganizationID(ctx, organizationID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization project: %w", err)
	}

	var projectNumber int64
	if project.GcpProjectNumber.Valid {
		_, _ = fmt.Sscanf(project.GcpProjectNumber.String, "%d", &projectNumber)
	}

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

// authorizeProjectSecretRead checks if user can read project secrets
// Requires: any project membership OR any organization membership.
func (s *ProjectSecretService) authorizeProjectSecretRead(ctx context.Context, projectID, organizationID int64) (*auth.UserInfo, error) {
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok || userInfo == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	_, err := s.db.GetProjectMemberByAccountAndProject(ctx, db.GetProjectMemberByAccountAndProjectParams{
		AccountID: userInfo.AccountID,
		ProjectID: projectID,
	})
	if err == nil {
		return userInfo, nil
	}

	_, err = s.db.GetOrganizationMemberByAccountAndOrganization(ctx, db.GetOrganizationMemberByAccountAndOrganizationParams{
		AccountID:      userInfo.AccountID,
		OrganizationID: organizationID,
	})
	if err == nil {
		return userInfo, nil
	}

	if err == sql.ErrNoRows {
		if relErr := service.CheckRelationshipAccess(ctx, s.db, userInfo.AccountID, organizationID, false); relErr != nil {
			return nil, relErr
		}
		return userInfo, nil
	}

	return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
}

// CreateProjectSecret creates a new project-level secret.
func (s *ProjectSecretService) CreateProjectSecret(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateProjectSecretRequest],
) (*connect.Response[libopsv1.CreateProjectSecretResponse], error) {
	if err := organization.ValidateSecretName(req.Msg.Name); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if err := validation.RequiredString("value", req.Msg.Value); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	if len(req.Msg.Value) > 65536 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("value too long (max 64KB)"))
	}

	projectUUID, err := uuid.Parse(req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id"))
	}

	// Get user info (authorization already done by scope interceptor)
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok || userInfo == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	project, err := s.db.GetProject(ctx, projectUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	_, err = s.db.GetProjectSecretByName(ctx, db.GetProjectSecretByNameParams{
		ProjectID: project.ID,
		Name:      req.Msg.Name,
	})
	if err == nil {
		return nil, connect.NewError(connect.CodeAlreadyExists,
			fmt.Errorf("secret %s already exists", req.Msg.Name))
	}
	if err != sql.ErrNoRows {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	vaultPath := vault.BuildProjectSecretPath(projectUUID.String(), req.Msg.Name)

	vaultClient, err := s.GetProjectVaultClient(ctx, project.OrganizationID)
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
		s.auditLogger.Log(ctx, userInfo.AccountID, project.ID, audit.ProjectEntityType, audit.ProjectSecretCreateFailed, map[string]any{
			"secret_name": req.Msg.Name,
			"vault_path":  vaultPath,
			"error":       "vault_write_failed",
			"error_msg":   err.Error(),
		})
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to write secret"))
	}

	secretUUID := uuid.New()
	now := time.Now().Unix()
	_, err = s.db.CreateProjectSecret(ctx, db.CreateProjectSecretParams{
		PublicID:  secretUUID.String(),
		ProjectID: project.ID,
		Name:      req.Msg.Name,
		VaultPath: vaultPath,
		Status:    db.NullProjectSecretsStatus{ProjectSecretsStatus: db.ProjectSecretsStatusActive, Valid: true},
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
		s.auditLogger.Log(ctx, userInfo.AccountID, project.ID, audit.ProjectEntityType, audit.ProjectSecretCreateFailed, map[string]any{
			"secret_name": req.Msg.Name,
			"vault_path":  vaultPath,
			"error":       "database_write_failed",
			"error_msg":   err.Error(),
		})
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create secret"))
	}

	secret, err := s.db.GetProjectSecretByName(ctx, db.GetProjectSecretByNameParams{
		ProjectID: project.ID,
		Name:      req.Msg.Name,
	})
	if err != nil {
		slog.Error("failed to retrieve created secret", "err", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to retrieve secret"))
	}

	// Audit log for success
	s.auditLogger.Log(ctx, userInfo.AccountID, secret.ID, audit.ProjectEntityType, audit.ProjectSecretCreateSuccess, map[string]any{
		"secret_id":   secret.PublicID,
		"secret_name": secret.Name,
		"vault_path":  secret.VaultPath,
	})

	return connect.NewResponse(&libopsv1.CreateProjectSecretResponse{
		Secret: &libopsv1.ProjectSecret{
			SecretId:  secret.PublicID,
			ProjectId: projectUUID.String(),
			Name:      secret.Name,
			Status:    dbProjectStatusToProto(secret.Status),
		},
	}), nil
}

// GetProjectSecret retrieves a project secret by ID.
func (s *ProjectSecretService) GetProjectSecret(
	ctx context.Context,
	req *connect.Request[libopsv1.GetProjectSecretRequest],
) (*connect.Response[libopsv1.GetProjectSecretResponse], error) {
	projectUUID, err := uuid.Parse(req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id"))
	}

	secretUUID, err := uuid.Parse(req.Msg.SecretId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret_id"))
	}

	project, err := s.db.GetProject(ctx, projectUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	_, err = s.authorizeProjectSecretRead(ctx, project.ID, project.OrganizationID)
	if err != nil {
		return nil, err
	}

	secret, err := s.db.GetProjectSecretByPublicID(ctx, secretUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("secret not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	if secret.ProjectID != project.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("secret does not belong to project"))
	}

	return connect.NewResponse(&libopsv1.GetProjectSecretResponse{
		Secret: &libopsv1.ProjectSecret{
			SecretId:  secret.PublicID,
			ProjectId: projectUUID.String(),
			Name:      secret.Name,
			Status:    dbProjectStatusToProto(secret.Status),
		},
	}), nil
}

// ListProjectSecrets lists all secrets for a project.
func (s *ProjectSecretService) ListProjectSecrets(
	ctx context.Context,
	req *connect.Request[libopsv1.ListProjectSecretsRequest],
) (*connect.Response[libopsv1.ListProjectSecretsResponse], error) {
	projectUUID, err := uuid.Parse(req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id"))
	}

	project, err := s.db.GetProject(ctx, projectUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	_, err = s.authorizeProjectSecretRead(ctx, project.ID, project.OrganizationID)
	if err != nil {
		return nil, err
	}

	pageSize := int32(50)
	if req.Msg.PageSize > 0 && req.Msg.PageSize <= 100 {
		pageSize = req.Msg.PageSize
	}
	offset := int32(0)
	// TODO: Implement page token parsing

	secrets, err := s.db.ListProjectSecrets(ctx, db.ListProjectSecretsParams{
		ProjectID: project.ID,
		Limit:     pageSize,
		Offset:    offset,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	protoSecrets := make([]*libopsv1.ProjectSecret, len(secrets))
	for i, secret := range secrets {
		protoSecrets[i] = &libopsv1.ProjectSecret{
			SecretId:  secret.PublicID,
			ProjectId: projectUUID.String(),
			Name:      secret.Name,
			Status:    dbProjectStatusToProto(secret.Status),
		}
	}

	return connect.NewResponse(&libopsv1.ListProjectSecretsResponse{
		Secrets:       protoSecrets,
		NextPageToken: "", // TODO: Implement pagination token
	}), nil
}

// UpdateProjectSecret updates a project secret value.
func (s *ProjectSecretService) UpdateProjectSecret(
	ctx context.Context,
	req *connect.Request[libopsv1.UpdateProjectSecretRequest],
) (*connect.Response[libopsv1.UpdateProjectSecretResponse], error) {
	projectUUID, err := uuid.Parse(req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id"))
	}

	secretUUID, err := uuid.Parse(req.Msg.SecretId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret_id"))
	}

	// Get user info (authorization already done by scope interceptor)
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok || userInfo == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	project, err := s.db.GetProject(ctx, projectUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	secret, err := s.db.GetProjectSecretByPublicID(ctx, secretUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("secret not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	if secret.ProjectID != project.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("secret does not belong to project"))
	}

	if req.Msg.Value != nil && *req.Msg.Value != "" {
		if len(*req.Msg.Value) > 65536 {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("value too long (max 64KB)"))
		}

		vaultClient, err := s.GetProjectVaultClient(ctx, project.OrganizationID)
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
			s.auditLogger.Log(ctx, userInfo.AccountID, project.ID, audit.ProjectEntityType, audit.ProjectSecretUpdateFailed, map[string]any{
				"secret_id":   secret.PublicID,
				"secret_name": secret.Name,
				"vault_path":  secret.VaultPath,
				"error":       "vault_write_failed",
				"error_msg":   err.Error(),
			})
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update secret"))
		}

		now := time.Now().Unix()
		err = s.db.UpdateProjectSecret(ctx, db.UpdateProjectSecretParams{
			VaultPath: secret.VaultPath,
			UpdatedBy: sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
			UpdatedAt: now,
			ID:        secret.ID,
		})
		if err != nil {
			slog.Error("failed to update secret record", "err", err)
			// Audit log for database failure
			s.auditLogger.Log(ctx, userInfo.AccountID, project.ID, audit.ProjectEntityType, audit.ProjectSecretUpdateFailed, map[string]any{
				"secret_id":   secret.PublicID,
				"secret_name": secret.Name,
				"vault_path":  secret.VaultPath,
				"error":       "database_update_failed",
				"error_msg":   err.Error(),
			})
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update secret"))
		}
	}

	secret, err = s.db.GetProjectSecretByPublicID(ctx, secretUUID.String())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Audit log for success
	s.auditLogger.Log(ctx, userInfo.AccountID, secret.ID, audit.ProjectEntityType, audit.ProjectSecretUpdateSuccess, map[string]any{
		"secret_id":   secret.PublicID,
		"secret_name": secret.Name,
		"vault_path":  secret.VaultPath,
	})

	return connect.NewResponse(&libopsv1.UpdateProjectSecretResponse{
		Secret: &libopsv1.ProjectSecret{
			SecretId:  secret.PublicID,
			ProjectId: projectUUID.String(),
			Name:      secret.Name,
			Status:    dbProjectStatusToProto(secret.Status),
		},
	}), nil
}

// DeleteProjectSecret deletes a project secret.
func (s *ProjectSecretService) DeleteProjectSecret(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteProjectSecretRequest],
) (*connect.Response[emptypb.Empty], error) {
	projectUUID, err := uuid.Parse(req.Msg.ProjectId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id"))
	}

	secretUUID, err := uuid.Parse(req.Msg.SecretId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid secret_id"))
	}

	// Get user info (authorization already done by scope interceptor)
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok || userInfo == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("not authenticated"))
	}

	project, err := s.db.GetProject(ctx, projectUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	secret, err := s.db.GetProjectSecretByPublicID(ctx, secretUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("secret not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	if secret.ProjectID != project.ID {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("secret does not belong to project"))
	}

	vaultClient, err := s.GetProjectVaultClient(ctx, project.OrganizationID)
	if err != nil {
		slog.Error("failed to get vault client", "err", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to access vault"))
	}

	err = vaultClient.DeleteSecret(ctx, secret.VaultPath)
	if err != nil {
		slog.Error("failed to delete secret from vault", "err", err)
		// Audit log for vault failure
		s.auditLogger.Log(ctx, userInfo.AccountID, project.ID, audit.ProjectEntityType, audit.ProjectSecretDeleteFailed, map[string]any{
			"secret_id":   secret.PublicID,
			"secret_name": secret.Name,
			"vault_path":  secret.VaultPath,
			"error":       "vault_delete_failed",
			"error_msg":   err.Error(),
		})
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete secret"))
	}

	now := time.Now().Unix()
	err = s.db.DeleteProjectSecret(ctx, db.DeleteProjectSecretParams{
		UpdatedBy: sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
		UpdatedAt: now,
		ID:        secret.ID,
	})
	if err != nil {
		slog.Error("failed to delete secret record", "err", err)
		// Audit log for database failure
		s.auditLogger.Log(ctx, userInfo.AccountID, project.ID, audit.ProjectEntityType, audit.ProjectSecretDeleteFailed, map[string]any{
			"secret_id":   secret.PublicID,
			"secret_name": secret.Name,
			"vault_path":  secret.VaultPath,
			"error":       "database_delete_failed",
			"error_msg":   err.Error(),
		})
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete secret"))
	}

	// Audit log for success
	s.auditLogger.Log(ctx, userInfo.AccountID, secret.ID, audit.ProjectEntityType, audit.ProjectSecretDeleteSuccess, map[string]any{
		"secret_id":   secret.PublicID,
		"secret_name": secret.Name,
		"vault_path":  secret.VaultPath,
	})

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// dbProjectStatusToProto converts database project secret status to proto status.
func dbProjectStatusToProto(status db.NullProjectSecretsStatus) commonv1.Status {
	if !status.Valid {
		return commonv1.Status_STATUS_UNSPECIFIED
	}

	switch status.ProjectSecretsStatus {
	case db.ProjectSecretsStatusActive:
		return commonv1.Status_STATUS_ACTIVE
	case db.ProjectSecretsStatusProvisioning:
		return commonv1.Status_STATUS_PROVISIONING
	case db.ProjectSecretsStatusFailed:
		return commonv1.Status_STATUS_FAILED
	case db.ProjectSecretsStatusSuspended:
		return commonv1.Status_STATUS_SUSPENDED
	case db.ProjectSecretsStatusDeleted:
		return commonv1.Status_STATUS_DELETED
	default:
		return commonv1.Status_STATUS_UNSPECIFIED
	}
}
