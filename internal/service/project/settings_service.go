package project

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/libops/api/db"
	"github.com/libops/api/internal/auth"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// ProjectSettingService implements the ProjectSettingService API.
type ProjectSettingService struct {
	db db.Querier
}

// Compile-time check to ensure ProjectSettingService implements the interface.
var _ libopsv1connect.ProjectSettingServiceHandler = (*ProjectSettingService)(nil)

// NewProjectSettingService creates a new ProjectSettingService instance.
func NewProjectSettingService(querier db.Querier) *ProjectSettingService {
	return &ProjectSettingService{
		db: querier,
	}
}

// CreateProjectSetting creates a new project setting.
func (s *ProjectSettingService) CreateProjectSetting(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateProjectSettingRequest],
) (*connect.Response[libopsv1.CreateProjectSettingResponse], error) {
	projectID := req.Msg.ProjectId
	key := req.Msg.Key
	value := req.Msg.Value
	editable := req.Msg.Editable
	description := req.Msg.Description

	if key == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("setting key is required"))
	}

	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	// Parse project UUID
	projectUUID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id: %w", err))
	}

	// Get project internal ID
	project, err := s.db.GetProject(ctx, projectUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project: %w", err))
	}

	// Create setting
	settingPublicID := uuid.New().String()
	err = s.db.CreateProjectSetting(ctx, db.CreateProjectSettingParams{
		PublicID:     settingPublicID,
		ProjectID:    project.ID,
		SettingKey:   key,
		SettingValue: value,
		Editable:     sql.NullBool{Bool: editable, Valid: true},
		Description:  sql.NullString{String: description, Valid: description != ""},
		Status:       db.NullProjectSettingsStatus{ProjectSettingsStatus: db.ProjectSettingsStatusActive, Valid: true},
		CreatedBy:    sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
		UpdatedBy:    sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
	})
	if err != nil {
		slog.Error("Failed to create project setting", "error", err, "project_id", projectID, "key", key)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create setting: %w", err))
	}

	// Return created setting
	setting := &libopsv1.ProjectSetting{
		SettingId:   settingPublicID,
		ProjectId:   projectID,
		Key:         key,
		Value:       value,
		Editable:    editable,
		Description: description,
		Status:      commonv1.Status_STATUS_ACTIVE,
	}

	return connect.NewResponse(&libopsv1.CreateProjectSettingResponse{
		Setting: setting,
	}), nil
}

// GetProjectSetting retrieves a single project setting.
func (s *ProjectSettingService) GetProjectSetting(
	ctx context.Context,
	req *connect.Request[libopsv1.GetProjectSettingRequest],
) (*connect.Response[libopsv1.GetProjectSettingResponse], error) {
	settingID := req.Msg.SettingId

	// Parse setting UUID
	settingUUID, err := uuid.Parse(settingID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid setting_id: %w", err))
	}

	// Get setting
	dbSetting, err := s.db.GetProjectSettingByPublicID(ctx, settingUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("setting not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get setting: %w", err))
	}

	// Convert to proto
	setting := &libopsv1.ProjectSetting{
		SettingId:   dbSetting.PublicID,
		ProjectId:   fmt.Sprintf("%d", dbSetting.ProjectID),
		Key:         dbSetting.SettingKey,
		Value:       dbSetting.SettingValue,
		Editable:    dbSetting.Editable.Bool,
		Description: dbSetting.Description.String,
		Status:      convertProjectSettingStatus(dbSetting.Status.ProjectSettingsStatus),
	}

	return connect.NewResponse(&libopsv1.GetProjectSettingResponse{
		Setting: setting,
	}), nil
}

// ListProjectSettings lists all settings for a project.
func (s *ProjectSettingService) ListProjectSettings(
	ctx context.Context,
	req *connect.Request[libopsv1.ListProjectSettingsRequest],
) (*connect.Response[libopsv1.ListProjectSettingsResponse], error) {
	projectID := req.Msg.ProjectId

	// Parse project UUID
	projectUUID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id: %w", err))
	}

	// Get project internal ID
	project, err := s.db.GetProject(ctx, projectUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("project not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get project: %w", err))
	}

	// List settings
	pageSize := req.Msg.PageSize
	if pageSize == 0 {
		pageSize = 100
	}

	dbSettings, err := s.db.ListProjectSettings(ctx, db.ListProjectSettingsParams{
		ProjectID: project.ID,
		Limit:     pageSize,
		Offset:    0,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list settings: %w", err))
	}

	// Convert to proto
	settings := make([]*libopsv1.ProjectSetting, 0, len(dbSettings))
	for _, dbSetting := range dbSettings {
		settings = append(settings, &libopsv1.ProjectSetting{
			SettingId:   dbSetting.PublicID,
			ProjectId:   projectID,
			Key:         dbSetting.SettingKey,
			Value:       dbSetting.SettingValue,
			Editable:    dbSetting.Editable.Bool,
			Description: dbSetting.Description.String,
			Status:      convertProjectSettingStatus(dbSetting.Status.ProjectSettingsStatus),
		})
	}

	return connect.NewResponse(&libopsv1.ListProjectSettingsResponse{
		Settings:      settings,
		NextPageToken: "",
	}), nil
}

// UpdateProjectSetting updates an existing project setting.
func (s *ProjectSettingService) UpdateProjectSetting(
	ctx context.Context,
	req *connect.Request[libopsv1.UpdateProjectSettingRequest],
) (*connect.Response[libopsv1.UpdateProjectSettingResponse], error) {
	settingID := req.Msg.SettingId
	value := req.Msg.Value

	if value == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("value is required"))
	}

	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	// Parse setting UUID
	settingUUID, err := uuid.Parse(settingID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid setting_id: %w", err))
	}

	// Get existing setting
	dbSetting, err := s.db.GetProjectSettingByPublicID(ctx, settingUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("setting not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get setting: %w", err))
	}

	// Check if setting is editable
	if !dbSetting.Editable.Bool {
		return nil, connect.NewError(connect.CodePermissionDenied, fmt.Errorf("this setting cannot be modified"))
	}

	// Update setting
	err = s.db.UpdateProjectSetting(ctx, db.UpdateProjectSettingParams{
		PublicID:     settingUUID.String(),
		SettingValue: *value,
		UpdatedBy:    sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
	})
	if err != nil {
		slog.Error("Failed to update project setting", "error", err, "setting_id", settingID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update setting: %w", err))
	}

	// Return updated setting
	setting := &libopsv1.ProjectSetting{
		SettingId:   settingID,
		ProjectId:   req.Msg.ProjectId,
		Key:         dbSetting.SettingKey,
		Value:       *value,
		Editable:    dbSetting.Editable.Bool,
		Description: dbSetting.Description.String,
		Status:      convertProjectSettingStatus(dbSetting.Status.ProjectSettingsStatus),
	}

	return connect.NewResponse(&libopsv1.UpdateProjectSettingResponse{
		Setting: setting,
	}), nil
}

// DeleteProjectSetting deletes a project setting.
func (s *ProjectSettingService) DeleteProjectSetting(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteProjectSettingRequest],
) (*connect.Response[emptypb.Empty], error) {
	settingID := req.Msg.SettingId

	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	// Parse setting UUID
	settingUUID, err := uuid.Parse(settingID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid setting_id: %w", err))
	}

	// Delete setting (soft delete)
	err = s.db.DeleteProjectSetting(ctx, db.DeleteProjectSettingParams{
		PublicID:  settingUUID.String(),
		UpdatedBy: sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
	})
	if err != nil {
		slog.Error("Failed to delete project setting", "error", err, "setting_id", settingID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete setting: %w", err))
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// convertProjectSettingStatus converts database status to proto status.
func convertProjectSettingStatus(status db.ProjectSettingsStatus) commonv1.Status {
	switch status {
	case db.ProjectSettingsStatusActive:
		return commonv1.Status_STATUS_ACTIVE
	case db.ProjectSettingsStatusProvisioning:
		return commonv1.Status_STATUS_PROVISIONING
	case db.ProjectSettingsStatusFailed:
		return commonv1.Status_STATUS_FAILED
	case db.ProjectSettingsStatusSuspended:
		return commonv1.Status_STATUS_SUSPENDED
	case db.ProjectSettingsStatusDeleted:
		return commonv1.Status_STATUS_DELETED
	default:
		return commonv1.Status_STATUS_UNSPECIFIED
	}
}
