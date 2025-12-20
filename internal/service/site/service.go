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
	"github.com/libops/api/internal/service"
	"github.com/libops/api/internal/validation"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// SiteService implements the organization-facing site API.
type SiteService struct {
	repo *Repository
}

// Compile-time check.
var _ libopsv1connect.SiteServiceHandler = (*SiteService)(nil)

// NewSiteService creates a new organization-facing site service.
func NewSiteService(querier db.Querier) *SiteService {
	return &SiteService{
		repo: NewRepository(querier),
	}
}

// ListSites lists sites for a organization/project.
func (s *SiteService) ListSites(
	ctx context.Context,
	req *connect.Request[libopsv1.ListSitesRequest],
) (*connect.Response[libopsv1.ListSitesResponse], error) {
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}
	accountID := userInfo.AccountID

	var filterOrgID sql.NullInt64
	if req.Msg.OrganizationId != nil && *req.Msg.OrganizationId != "" {
		if err := validation.UUID(*req.Msg.OrganizationId); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		organizationPublicID, err := uuid.Parse(*req.Msg.OrganizationId)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id format: %w", err))
		}
		org, err := s.repo.GetOrganizationByPublicID(ctx, organizationPublicID)
		if err != nil {
			slog.Error("Failed to get organization by public ID", "error", err, "organization_id", *req.Msg.OrganizationId)
			return nil, err
		}
		filterOrgID = sql.NullInt64{Int64: org.ID, Valid: true}
	}

	var filterProjectID sql.NullInt64
	if req.Msg.ProjectId != nil && *req.Msg.ProjectId != "" {
		if err := validation.UUID(*req.Msg.ProjectId); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		projectPublicID, err := uuid.Parse(*req.Msg.ProjectId)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
		}
		project, err := s.repo.GetProjectByPublicID(ctx, projectPublicID)
		if err != nil {
			slog.Error("Failed to get project by public ID", "error", err, "project_id", *req.Msg.ProjectId)
			return nil, err
		}
		filterProjectID = sql.NullInt64{Int64: project.ID, Valid: true}
	}

	pageSize := req.Msg.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset, err := service.ParsePageToken(req.Msg.PageToken)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	params := db.ListUserSitesParams{
		AccountID:            accountID,
		FilterOrganizationID: filterOrgID,
		FilterProjectID:      filterProjectID,
		Limit:                pageSize,
		Offset:               int32(offset),
	}

	sites, err := s.repo.ListUserSites(ctx, params)
	if err != nil {
		slog.Error("Failed to list user sites", "error", err, "account_id", accountID)
		return nil, err
	}

	protoSites := make([]*commonv1.SiteConfig, 0, len(sites))
	for _, site := range sites {
		protoSites = append(protoSites, &commonv1.SiteConfig{
			SiteId:         site.PublicID,
			OrganizationId: site.OrganizationPublicID,
			ProjectId:      site.ProjectPublicID,
			SiteName:       site.Name,
			GithubRef:      site.GithubRef,
			UpCmd:          service.FromJSONStringArray(site.UpCmd),
			InitCmd:        service.FromJSONStringArray(site.InitCmd),
			RolloutCmd:     service.FromJSONStringArray(site.RolloutCmd),
			OverlayVolumes: service.FromJSONStringArray(site.OverlayVolumes),
			Os:             service.FromNullString(site.Os),
			IsProduction:   site.IsProduction.Bool,
			Status:         DbSiteStatusToProto(site.Status),
		})
	}

	nextPageToken := ""
	if len(sites) == int(pageSize) {
		nextPageToken = service.GeneratePageToken(offset + int(pageSize))
	}

	return connect.NewResponse(&libopsv1.ListSitesResponse{
		Sites:         protoSites,
		NextPageToken: nextPageToken,
	}), nil
}

// GetSite retrieves site configuration (organization view).
func (s *SiteService) GetSite(
	ctx context.Context,
	req *connect.Request[libopsv1.GetSiteRequest],
) (*connect.Response[libopsv1.GetSiteResponse], error) {
	siteID := req.Msg.SiteId

	if err := validation.UUID(siteID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	siteUUID, err := uuid.Parse(siteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id format: %w", err))
	}

	site, err := s.repo.GetSiteByPublicID(ctx, siteUUID)
	if err != nil {
		slog.Error("Failed to get site by public ID", "error", err, "site_id", siteID)
		return nil, err
	}

	project, err := s.repo.GetProjectByID(ctx, site.ProjectID)
	if err != nil {
		slog.Error("Failed to get project by ID", "error", err, "project_id", site.ProjectID)
		return nil, err
	}

	org, err := s.repo.GetOrganizationByID(ctx, project.OrganizationID)
	if err != nil {
		slog.Error("Failed to get organization by ID", "error", err, "organization_id", project.OrganizationID)
		return nil, err
	}

	protoSite := &commonv1.SiteConfig{
		SiteId:         site.PublicID,
		OrganizationId: org.PublicID,
		ProjectId:      project.PublicID,
		SiteName:       site.Name,
		GithubRef:      site.GithubRef,
		UpCmd:          service.FromJSONStringArray(site.UpCmd),
		InitCmd:        service.FromJSONStringArray(site.InitCmd),
		RolloutCmd:     service.FromJSONStringArray(site.RolloutCmd),
		OverlayVolumes: service.FromJSONStringArray(site.OverlayVolumes),
		Os:             service.FromNullString(site.Os),
		IsProduction:   site.IsProduction.Bool,
		Status:         service.DbSiteStatusToProto(site.Status),
	}

	return connect.NewResponse(&libopsv1.GetSiteResponse{
		Site: protoSite,
	}), nil
}

// CreateSite creates a new site.
func (s *SiteService) CreateSite(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateSiteRequest],
) (*connect.Response[libopsv1.CreateSiteResponse], error) {
	projectID := req.Msg.ProjectId
	site := req.Msg.Site

	if err := validation.UUID(projectID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if site == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("site is required"))
	}

	slog.Info("CreateSite called", "project_id", projectID, "site_name", site.SiteName)

	if err := validation.SiteName(site.SiteName); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	projectPublicID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
	}

	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	accountID := userInfo.AccountID

	project, err := s.repo.GetProjectByPublicID(ctx, projectPublicID)
	if err != nil {
		slog.Error("Failed to get project by public ID", "error", err, "project_id", projectID)
		return nil, err
	}

	// Set defaults for new fields - inherit from project if not specified
	osImage := site.Os
	if osImage == "" {
		osImage = service.FromNullString(project.Os)
		if osImage == "" {
			osImage = "cos-125-19216-104-74" // Default
		}
	}

	// Organizations can create sites but GCP fields are set by orchestration
	params := db.CreateSiteParams{
		ProjectID:        project.ID,
		Name:             site.SiteName,
		GithubRepository: site.GithubRepository,
		GithubRef:        site.GithubRef,
		ComposePath:      service.ToNullString(site.ComposePath),
		ComposeFile:      service.ToNullString(site.ComposeFile),
		Port:             service.ToNullInt32(site.Port),
		ApplicationType:  service.ToNullString(site.ApplicationType),
		UpCmd:            service.ToJSON(site.UpCmd),
		InitCmd:          service.ToJSON(site.InitCmd),
		RolloutCmd:       service.ToJSON(site.RolloutCmd),
		OverlayVolumes:   service.ToJSON(site.OverlayVolumes),
		Os:               sql.NullString{String: osImage, Valid: true},
		IsProduction:     sql.NullBool{Bool: site.IsProduction, Valid: true},
		GcpExternalIp:    sql.NullString{Valid: false}, // Set by orchestration
		GithubTeamID:     sql.NullString{Valid: false}, // Set by orchestration or admin
		Status:           db.NullSitesStatus{SitesStatus: db.SitesStatusProvisioning, Valid: true},
		CreatedBy:        sql.NullInt64{Int64: accountID, Valid: true},
		UpdatedBy:        sql.NullInt64{Int64: accountID, Valid: true},
	}

	err = s.repo.CreateSite(ctx, params)
	if err != nil {
		slog.Error("Failed to create site in DB", "error", err, "params", params)
		return nil, err
	}

	// Fetch the newly created site to get all populated fields
	createdSite, err := s.repo.GetSiteByProjectAndName(ctx, project.ID, site.SiteName)
	if err != nil {
		slog.Error("Failed to get created site", "error", err, "site_name", site.SiteName)
		return nil, err
	}

	// Get organization public ID
	organization, err := s.repo.GetOrganizationByID(ctx, project.OrganizationID)
	if err != nil {
		slog.Error("Failed to get organization by ID", "error", err, "organization_id", project.OrganizationID)
		return nil, err
	}

	return connect.NewResponse(&libopsv1.CreateSiteResponse{
		Site: &commonv1.SiteConfig{
			SiteId:         createdSite.PublicID,
			OrganizationId: organization.PublicID,
			ProjectId:      project.PublicID,
			SiteName:       createdSite.Name,
			GithubRef:      createdSite.GithubRef,
			UpCmd:          service.FromJSONStringArray(createdSite.UpCmd),
			InitCmd:        service.FromJSONStringArray(createdSite.InitCmd),
			RolloutCmd:     service.FromJSONStringArray(createdSite.RolloutCmd),
			OverlayVolumes: service.FromJSONStringArray(createdSite.OverlayVolumes),
			Os:             service.FromNullString(createdSite.Os),
			IsProduction:   createdSite.IsProduction.Bool,
			Status:         service.DbSiteStatusToProto(createdSite.Status),
		},
	}), nil
}

// UpdateSite updates site configuration (organization-editable fields only).
func (s *SiteService) UpdateSite(
	ctx context.Context,
	req *connect.Request[libopsv1.UpdateSiteRequest],
) (*connect.Response[libopsv1.UpdateSiteResponse], error) {
	siteID := req.Msg.SiteId
	site := req.Msg.Site

	if err := validation.UUID(siteID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if site == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("site is required"))
	}

	siteUUID, err := uuid.Parse(siteID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid site_id format: %w", err))
	}

	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	accountID := userInfo.AccountID

	existing, err := s.repo.GetSiteByPublicID(ctx, siteUUID)
	if err != nil {
		slog.Error("Failed to get site by public ID for update", "error", err, "site_id", siteID)
		return nil, err
	}

	name := existing.Name
	githubRepository := existing.GithubRepository
	githubRef := existing.GithubRef
	composePath := existing.ComposePath
	composeFile := existing.ComposeFile
	port := existing.Port
	applicationType := existing.ApplicationType
	upCmd := existing.UpCmd
	initCmd := existing.InitCmd
	rolloutCmd := existing.RolloutCmd
	gcpExternalIp := existing.GcpExternalIp
	overlayVolumes := existing.OverlayVolumes
	osImage := existing.Os
	isProduction := existing.IsProduction

	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.site_name") {
		name = site.SiteName
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.github_ref") {
		githubRef = site.GithubRef
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.up_cmd") {
		upCmd = service.ToJSON(site.UpCmd)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.init_cmd") {
		initCmd = service.ToJSON(site.InitCmd)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.rollout_cmd") {
		rolloutCmd = service.ToJSON(site.RolloutCmd)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.overlay_volumes") {
		overlayVolumes = service.ToJSON(site.OverlayVolumes)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.os") {
		osImage = sql.NullString{String: site.Os, Valid: true}
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.is_production") {
		isProduction = sql.NullBool{Bool: site.IsProduction, Valid: true}
	}

	// Preserve all GCP fields
	params := db.UpdateSiteParams{
		Name:             name,
		GithubRepository: githubRepository,
		GithubRef:        githubRef,
		ComposePath:      composePath,
		ComposeFile:      composeFile,
		Port:             port,
		ApplicationType:  applicationType,
		UpCmd:            upCmd,
		InitCmd:          initCmd,
		RolloutCmd:       rolloutCmd,
		OverlayVolumes:   overlayVolumes,
		Os:               osImage,
		IsProduction:     isProduction,
		GcpExternalIp:    gcpExternalIp,
		GithubTeamID:     existing.GithubTeamID,
		Status:           existing.Status,
		UpdatedBy:        sql.NullInt64{Int64: accountID, Valid: true},
		PublicID:         siteUUID.String(),
	}

	err = s.repo.UpdateSite(ctx, params)
	if err != nil {
		slog.Error("Failed to update site in DB", "error", err, "site_id", siteID)
		return nil, err
	}

	return connect.NewResponse(&libopsv1.UpdateSiteResponse{
		Site: site,
	}), nil
}

// DeleteSite deletes a site.
func (s *SiteService) DeleteSite(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteSiteRequest],
) (*connect.Response[emptypb.Empty], error) {
	siteID := req.Msg.SiteId

	if err := validation.UUID(siteID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	err := s.repo.DeleteSite(ctx, siteID)
	if err != nil {
		slog.Error("Failed to delete site", "error", err, "site_id", siteID)
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}
