package organization

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

// OrganizationSettingService implements the OrganizationSettingService API.
type OrganizationSettingService struct {
	db db.Querier
}

// Compile-time check to ensure OrganizationSettingService implements the interface.
var _ libopsv1connect.OrganizationSettingServiceHandler = (*OrganizationSettingService)(nil)

// NewOrganizationSettingService creates a new OrganizationSettingService instance.
func NewOrganizationSettingService(querier db.Querier) *OrganizationSettingService {
	return &OrganizationSettingService{
		db: querier,
	}
}

// CreateOrganizationSetting creates a new organization setting.
func (s *OrganizationSettingService) CreateOrganizationSetting(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateOrganizationSettingRequest],
) (*connect.Response[libopsv1.CreateOrganizationSettingResponse], error) {
	organizationID := req.Msg.OrganizationId
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

	// Parse organization UUID
	orgUUID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id: %w", err))
	}

	// Get organization internal ID
	org, err := s.db.GetOrganization(ctx, orgUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get organization: %w", err))
	}

	// Create setting
	settingPublicID := uuid.New().String()
	err = s.db.CreateOrganizationSetting(ctx, db.CreateOrganizationSettingParams{
		PublicID:       settingPublicID,
		OrganizationID: org.ID,
		SettingKey:     key,
		SettingValue:   value,
		Editable:       sql.NullBool{Bool: editable, Valid: true},
		Description:    sql.NullString{String: description, Valid: description != ""},
		Status:         db.NullOrganizationSettingsStatus{OrganizationSettingsStatus: db.OrganizationSettingsStatusActive, Valid: true},
		CreatedBy:      sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
		UpdatedBy:      sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
	})
	if err != nil {
		slog.Error("Failed to create organization setting", "error", err, "org_id", organizationID, "key", key)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create setting: %w", err))
	}

	// Return created setting
	setting := &libopsv1.OrganizationSetting{
		SettingId:      settingPublicID,
		OrganizationId: organizationID,
		Key:            key,
		Value:          value,
		Editable:       editable,
		Description:    description,
		Status:         commonv1.Status_STATUS_ACTIVE,
	}

	return connect.NewResponse(&libopsv1.CreateOrganizationSettingResponse{
		Setting: setting,
	}), nil
}

// GetOrganizationSetting retrieves a single organization setting.
func (s *OrganizationSettingService) GetOrganizationSetting(
	ctx context.Context,
	req *connect.Request[libopsv1.GetOrganizationSettingRequest],
) (*connect.Response[libopsv1.GetOrganizationSettingResponse], error) {
	settingID := req.Msg.SettingId

	// Parse setting UUID
	settingUUID, err := uuid.Parse(settingID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid setting_id: %w", err))
	}

	// Get setting
	dbSetting, err := s.db.GetOrganizationSettingByPublicID(ctx, settingUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("setting not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get setting: %w", err))
	}

	// Convert to proto
	setting := &libopsv1.OrganizationSetting{
		SettingId:      dbSetting.PublicID,
		OrganizationId: fmt.Sprintf("%d", dbSetting.OrganizationID),
		Key:            dbSetting.SettingKey,
		Value:          dbSetting.SettingValue,
		Editable:       dbSetting.Editable.Bool,
		Description:    dbSetting.Description.String,
		Status:         convertSettingStatus(dbSetting.Status.OrganizationSettingsStatus),
	}

	return connect.NewResponse(&libopsv1.GetOrganizationSettingResponse{
		Setting: setting,
	}), nil
}

// ListOrganizationSettings lists all settings for an organization.
func (s *OrganizationSettingService) ListOrganizationSettings(
	ctx context.Context,
	req *connect.Request[libopsv1.ListOrganizationSettingsRequest],
) (*connect.Response[libopsv1.ListOrganizationSettingsResponse], error) {
	organizationID := req.Msg.OrganizationId

	// Parse organization UUID
	orgUUID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id: %w", err))
	}

	// Get organization internal ID
	org, err := s.db.GetOrganization(ctx, orgUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get organization: %w", err))
	}

	// List settings
	pageSize := req.Msg.PageSize
	if pageSize == 0 {
		pageSize = 100
	}

	dbSettings, err := s.db.ListOrganizationSettings(ctx, db.ListOrganizationSettingsParams{
		OrganizationID: org.ID,
		Limit:          pageSize,
		Offset:         0,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list settings: %w", err))
	}

	// Convert to proto
	settings := make([]*libopsv1.OrganizationSetting, 0, len(dbSettings))
	for _, dbSetting := range dbSettings {
		settings = append(settings, &libopsv1.OrganizationSetting{
			SettingId:      dbSetting.PublicID,
			OrganizationId: organizationID,
			Key:            dbSetting.SettingKey,
			Value:          dbSetting.SettingValue,
			Editable:       dbSetting.Editable.Bool,
			Description:    dbSetting.Description.String,
			Status:         convertSettingStatus(dbSetting.Status.OrganizationSettingsStatus),
		})
	}

	return connect.NewResponse(&libopsv1.ListOrganizationSettingsResponse{
		Settings:      settings,
		NextPageToken: "",
	}), nil
}

// UpdateOrganizationSetting updates an existing organization setting.
func (s *OrganizationSettingService) UpdateOrganizationSetting(
	ctx context.Context,
	req *connect.Request[libopsv1.UpdateOrganizationSettingRequest],
) (*connect.Response[libopsv1.UpdateOrganizationSettingResponse], error) {
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
	dbSetting, err := s.db.GetOrganizationSettingByPublicID(ctx, settingUUID.String())
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
	err = s.db.UpdateOrganizationSetting(ctx, db.UpdateOrganizationSettingParams{
		PublicID:     settingUUID.String(),
		SettingValue: *value,
		UpdatedBy:    sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
	})
	if err != nil {
		slog.Error("Failed to update organization setting", "error", err, "setting_id", settingID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update setting: %w", err))
	}

	// Return updated setting
	setting := &libopsv1.OrganizationSetting{
		SettingId:      settingID,
		OrganizationId: req.Msg.OrganizationId,
		Key:            dbSetting.SettingKey,
		Value:          *value,
		Editable:       dbSetting.Editable.Bool,
		Description:    dbSetting.Description.String,
		Status:         convertSettingStatus(dbSetting.Status.OrganizationSettingsStatus),
	}

	return connect.NewResponse(&libopsv1.UpdateOrganizationSettingResponse{
		Setting: setting,
	}), nil
}

// DeleteOrganizationSetting deletes an organization setting.
func (s *OrganizationSettingService) DeleteOrganizationSetting(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteOrganizationSettingRequest],
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
	err = s.db.DeleteOrganizationSetting(ctx, db.DeleteOrganizationSettingParams{
		PublicID:  settingUUID.String(),
		UpdatedBy: sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
	})
	if err != nil {
		slog.Error("Failed to delete organization setting", "error", err, "setting_id", settingID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete setting: %w", err))
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// convertSettingStatus converts database status to proto status.
func convertSettingStatus(status db.OrganizationSettingsStatus) commonv1.Status {
	switch status {
	case db.OrganizationSettingsStatusActive:
		return commonv1.Status_STATUS_ACTIVE
	case db.OrganizationSettingsStatusProvisioning:
		return commonv1.Status_STATUS_PROVISIONING
	case db.OrganizationSettingsStatusFailed:
		return commonv1.Status_STATUS_FAILED
	case db.OrganizationSettingsStatusSuspended:
		return commonv1.Status_STATUS_SUSPENDED
	case db.OrganizationSettingsStatusDeleted:
		return commonv1.Status_STATUS_DELETED
	default:
		return commonv1.Status_STATUS_UNSPECIFIED
	}
}
