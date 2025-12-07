package site

import (
	"context"
	"database/sql"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/service"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	adminv1 "github.com/libops/api/proto/libops/v1/admin"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// AdminSiteService implements the admin-level site API.
type AdminSiteService struct {
	repo *Repository
}

// Compile-time check.
var _ libopsv1connect.AdminSiteServiceHandler = (*AdminSiteService)(nil)

// NewAdminSiteService creates a new admin site service.
func NewAdminSiteService(querier db.Querier) *AdminSiteService {
	return &AdminSiteService{
		repo: NewRepository(querier),
	}
}

// ListSites lists sites for a project (admin view).
func (s *AdminSiteService) ListSites(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminListSitesRequest],
) (*connect.Response[libopsv1.AdminListSitesResponse], error) {
	var projectID string
	if req.Msg.ProjectId != nil {
		projectID = *req.Msg.ProjectId
	}

	if projectID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id is required"))
	}

	projectPublicID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
	}

	project, err := s.repo.GetProjectByPublicID(ctx, projectPublicID)
	if err != nil {
		return nil, err
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

	params := db.ListProjectSitesParams{
		ProjectID: project.ID,
		Limit:     pageSize,
		Offset:    int32(offset),
	}

	sites, err := s.repo.ListProjectSites(ctx, params)
	if err != nil {
		return nil, err
	}

	var organizationID string
	if req.Msg.OrganizationId != nil {
		organizationID = *req.Msg.OrganizationId
	}

	protoSites := make([]*adminv1.AdminSiteConfig, 0, len(sites))
	for _, site := range sites {
		protoSites = append(protoSites, &adminv1.AdminSiteConfig{
			Config: &commonv1.SiteConfig{
				SiteId:         site.PublicID,
				OrganizationId: organizationID,
				ProjectId:      projectID,
				SiteName:       site.Name,
				GithubRef:      site.GithubRef,
				Status:         DbSiteStatusToProto(site.Status),
			},
			GcpInstanceName: nil,
			GcpExternalIp:   service.FromNullStringPtr(site.GcpExternalIp),
			GcpInternalIp:   nil,
		})
	}

	nextPageToken := ""
	if len(sites) == int(pageSize) {
		nextPageToken = service.GeneratePageToken(offset + int(pageSize))
	}

	return connect.NewResponse(&libopsv1.AdminListSitesResponse{
		Sites:         protoSites,
		NextPageToken: nextPageToken,
	}), nil
}

// GetSite retrieves site configuration (admin view - includes internal GCP details).
func (s *AdminSiteService) GetSite(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminGetSiteRequest],
) (*connect.Response[libopsv1.AdminGetSiteResponse], error) {
	projectID := req.Msg.ProjectId
	siteName := req.Msg.SiteName

	if projectID == "" || siteName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id and site_name are required"))
	}

	projectPublicID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
	}

	project, err := s.repo.GetProjectByPublicID(ctx, projectPublicID)
	if err != nil {
		return nil, err
	}

	site, err := s.repo.GetSiteByProjectAndName(ctx, project.ID, siteName)
	if err != nil {
		return nil, err
	}

	protoSite := &adminv1.AdminSiteConfig{
		Config: &commonv1.SiteConfig{
			SiteId:         site.PublicID,
			OrganizationId: req.Msg.OrganizationId,
			ProjectId:      projectID,
			SiteName:       site.Name,
			GithubRef:      site.GithubRef,
			Status:         DbSiteStatusToProto(site.Status),
		},
		GcpInstanceName: nil,
		GcpExternalIp:   service.FromNullStringPtr(site.GcpExternalIp),
		GcpInternalIp:   nil,
	}

	return connect.NewResponse(&libopsv1.AdminGetSiteResponse{
		Site: protoSite,
	}), nil
}

// CreateSite creates a new site (admin - can set all fields).
func (s *AdminSiteService) CreateSite(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminCreateSiteRequest],
) (*connect.Response[libopsv1.AdminCreateSiteResponse], error) {
	projectID := req.Msg.ProjectId
	site := req.Msg.Site

	if projectID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id is required"))
	}

	if site == nil || site.Config == nil || site.Config.SiteName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("site with site_name is required"))
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
		return nil, err
	}

	params := db.CreateSiteParams{
		ProjectID:     project.ID,
		Name:          site.Config.SiteName,
		GithubRef:     site.Config.GithubRef,
		GcpExternalIp: toNullString(ptrToString(site.GcpExternalIp)),
		Status:        db.NullSitesStatus{SitesStatus: db.SitesStatusProvisioning, Valid: true},
		CreatedBy:     sql.NullInt64{Int64: accountID, Valid: true},
		UpdatedBy:     sql.NullInt64{Int64: accountID, Valid: true},
	}

	err = s.repo.CreateSite(ctx, params)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&libopsv1.AdminCreateSiteResponse{
		Site: site,
	}), nil
}

// UpdateSite updates site configuration (admin - can update all fields).
func (s *AdminSiteService) UpdateSite(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminUpdateSiteRequest],
) (*connect.Response[libopsv1.AdminUpdateSiteResponse], error) {
	projectID := req.Msg.ProjectId
	siteName := req.Msg.SiteName
	site := req.Msg.Site

	if projectID == "" || siteName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id and site_name are required"))
	}

	if site == nil || site.Config == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("site is required"))
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
		return nil, err
	}

	existing, err := s.repo.GetSiteByProjectAndName(ctx, project.ID, siteName)
	if err != nil {
		return nil, err
	}

	name := existing.Name
	githubRef := existing.GithubRef
	gcpExternalIp := existing.GcpExternalIp

	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.config.site_name") {
		name = site.Config.SiteName
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.config.github_ref") {
		githubRef = site.Config.GithubRef
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "site.gcp_external_ip") {
		gcpExternalIp = toNullString(ptrToString(site.GcpExternalIp))
	}

	siteUUID, err := uuid.Parse(existing.PublicID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("invalid site public_id: %w", err))
	}

	params := db.UpdateSiteParams{
		Name:          name,
		GithubRef:     githubRef,
		GcpExternalIp: gcpExternalIp,
		Status:        db.NullSitesStatus{SitesStatus: db.SitesStatusActive, Valid: true},
		UpdatedBy:     sql.NullInt64{Int64: accountID, Valid: true},
		PublicID:      siteUUID.String(),
	}

	err = s.repo.UpdateSite(ctx, params)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&libopsv1.AdminUpdateSiteResponse{
		Site: site,
	}), nil
}

// DeleteSite deletes a site.
func (s *AdminSiteService) DeleteSite(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminDeleteSiteRequest],
) (*connect.Response[emptypb.Empty], error) {
	projectID := req.Msg.ProjectId
	siteName := req.Msg.SiteName

	if projectID == "" || siteName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id and site_name are required"))
	}

	projectPublicID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
	}

	project, err := s.repo.GetProjectByPublicID(ctx, projectPublicID)
	if err != nil {
		return nil, err
	}

	site, err := s.repo.GetSiteByProjectAndName(ctx, project.ID, siteName)
	if err != nil {
		return nil, err
	}

	err = s.repo.DeleteSite(ctx, site.PublicID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// ListAllSites lists all sites across all organizations/projects (admin only).
func (s *AdminSiteService) ListAllSites(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminListAllSitesRequest],
) (*connect.Response[libopsv1.AdminListAllSitesResponse], error) {
	// TODO: Implement cross-project site listing
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("list all sites not yet implemented"))
}
