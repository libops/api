package site

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

// SiteSettingService implements the SiteSettingService API.
type SiteSettingService struct {
	db db.Querier
}

// Compile-time check to ensure SiteSettingService implements the interface.
var _ libopsv1connect.SiteSettingServiceHandler = (*SiteSettingService)(nil)

// NewSiteSettingService creates a new SiteSettingService instance.
func NewSiteSettingService(querier db.Querier) *SiteSettingService {
	return &SiteSettingService{
		db: querier,
	}
}

// CreateSiteSetting creates a new site setting.
func (s *SiteSettingService) CreateSiteSetting(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateSiteSettingRequest],
) (*connect.Response[libopsv1.CreateSiteSettingResponse], error) {
	siteID := req.Msg.SiteId
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

	// Parse site UUID
	siteUUID, err := uuid.Parse(siteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id: %w", err))
	}

	// Get site internal ID
	site, err := s.db.GetSite(ctx, siteUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get site: %w", err))
	}

	// Create setting
	settingPublicID := uuid.New().String()
	err = s.db.CreateSiteSetting(ctx, db.CreateSiteSettingParams{
		PublicID:     settingPublicID,
		SiteID:       site.ID,
		SettingKey:   key,
		SettingValue: value,
		Editable:     sql.NullBool{Bool: editable, Valid: true},
		Description:  sql.NullString{String: description, Valid: description != ""},
		Status:       db.NullSiteSettingsStatus{SiteSettingsStatus: db.SiteSettingsStatusActive, Valid: true},
		CreatedBy:    sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
		UpdatedBy:    sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
	})
	if err != nil {
		slog.Error("Failed to create site setting", "error", err, "site_id", siteID, "key", key)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create setting: %w", err))
	}

	// Return created setting
	setting := &libopsv1.SiteSetting{
		SettingId:   settingPublicID,
		SiteId:      siteID,
		Key:         key,
		Value:       value,
		Editable:    editable,
		Description: description,
		Status:      commonv1.Status_STATUS_ACTIVE,
	}

	return connect.NewResponse(&libopsv1.CreateSiteSettingResponse{
		Setting: setting,
	}), nil
}

// GetSiteSetting retrieves a single site setting.
func (s *SiteSettingService) GetSiteSetting(
	ctx context.Context,
	req *connect.Request[libopsv1.GetSiteSettingRequest],
) (*connect.Response[libopsv1.GetSiteSettingResponse], error) {
	settingID := req.Msg.SettingId

	// Parse setting UUID
	settingUUID, err := uuid.Parse(settingID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid setting_id: %w", err))
	}

	// Get setting
	dbSetting, err := s.db.GetSiteSettingByPublicID(ctx, settingUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("setting not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get setting: %w", err))
	}

	// Convert to proto
	setting := &libopsv1.SiteSetting{
		SettingId:   dbSetting.PublicID,
		SiteId:      fmt.Sprintf("%d", dbSetting.SiteID),
		Key:         dbSetting.SettingKey,
		Value:       dbSetting.SettingValue,
		Editable:    dbSetting.Editable.Bool,
		Description: dbSetting.Description.String,
		Status:      convertSiteSettingStatus(dbSetting.Status.SiteSettingsStatus),
	}

	return connect.NewResponse(&libopsv1.GetSiteSettingResponse{
		Setting: setting,
	}), nil
}

// ListSiteSettings lists all settings for a site.
func (s *SiteSettingService) ListSiteSettings(
	ctx context.Context,
	req *connect.Request[libopsv1.ListSiteSettingsRequest],
) (*connect.Response[libopsv1.ListSiteSettingsResponse], error) {
	siteID := req.Msg.SiteId

	// Parse site UUID
	siteUUID, err := uuid.Parse(siteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id: %w", err))
	}

	// Get site internal ID
	site, err := s.db.GetSite(ctx, siteUUID.String())
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("site not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get site: %w", err))
	}

	// List settings
	pageSize := req.Msg.PageSize
	if pageSize == 0 {
		pageSize = 100
	}

	dbSettings, err := s.db.ListSiteSettings(ctx, db.ListSiteSettingsParams{
		SiteID: site.ID,
		Limit:  pageSize,
		Offset: 0,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list settings: %w", err))
	}

	// Convert to proto
	settings := make([]*libopsv1.SiteSetting, 0, len(dbSettings))
	for _, dbSetting := range dbSettings {
		settings = append(settings, &libopsv1.SiteSetting{
			SettingId:   dbSetting.PublicID,
			SiteId:      siteID,
			Key:         dbSetting.SettingKey,
			Value:       dbSetting.SettingValue,
			Editable:    dbSetting.Editable.Bool,
			Description: dbSetting.Description.String,
			Status:      convertSiteSettingStatus(dbSetting.Status.SiteSettingsStatus),
		})
	}

	return connect.NewResponse(&libopsv1.ListSiteSettingsResponse{
		Settings:      settings,
		NextPageToken: "",
	}), nil
}

// UpdateSiteSetting updates an existing site setting.
func (s *SiteSettingService) UpdateSiteSetting(
	ctx context.Context,
	req *connect.Request[libopsv1.UpdateSiteSettingRequest],
) (*connect.Response[libopsv1.UpdateSiteSettingResponse], error) {
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
	dbSetting, err := s.db.GetSiteSettingByPublicID(ctx, settingUUID.String())
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
	err = s.db.UpdateSiteSetting(ctx, db.UpdateSiteSettingParams{
		PublicID:     settingUUID.String(),
		SettingValue: *value,
		UpdatedBy:    sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
	})
	if err != nil {
		slog.Error("Failed to update site setting", "error", err, "setting_id", settingID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update setting: %w", err))
	}

	// Return updated setting
	setting := &libopsv1.SiteSetting{
		SettingId:   settingID,
		SiteId:      req.Msg.SiteId,
		Key:         dbSetting.SettingKey,
		Value:       *value,
		Editable:    dbSetting.Editable.Bool,
		Description: dbSetting.Description.String,
		Status:      convertSiteSettingStatus(dbSetting.Status.SiteSettingsStatus),
	}

	return connect.NewResponse(&libopsv1.UpdateSiteSettingResponse{
		Setting: setting,
	}), nil
}

// DeleteSiteSetting deletes a site setting.
func (s *SiteSettingService) DeleteSiteSetting(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteSiteSettingRequest],
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
	err = s.db.DeleteSiteSetting(ctx, db.DeleteSiteSettingParams{
		PublicID:  settingUUID.String(),
		UpdatedBy: sql.NullInt64{Int64: userInfo.AccountID, Valid: true},
	})
	if err != nil {
		slog.Error("Failed to delete site setting", "error", err, "setting_id", settingID)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to delete setting: %w", err))
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// convertSiteSettingStatus converts database status to proto status.
func convertSiteSettingStatus(status db.SiteSettingsStatus) commonv1.Status {
	switch status {
	case db.SiteSettingsStatusActive:
		return commonv1.Status_STATUS_ACTIVE
	case db.SiteSettingsStatusProvisioning:
		return commonv1.Status_STATUS_PROVISIONING
	case db.SiteSettingsStatusFailed:
		return commonv1.Status_STATUS_FAILED
	case db.SiteSettingsStatusSuspended:
		return commonv1.Status_STATUS_SUSPENDED
	case db.SiteSettingsStatusDeleted:
		return commonv1.Status_STATUS_DELETED
	default:
		return commonv1.Status_STATUS_UNSPECIFIED
	}
}
