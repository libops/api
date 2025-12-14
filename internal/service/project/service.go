package project

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
	"github.com/libops/api/internal/validation"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// ProjectService implements the organization-facing project API.
type ProjectService struct {
	repo *Repository
}

// Compile-time check.
var _ libopsv1connect.ProjectServiceHandler = (*ProjectService)(nil)

// NewProjectService creates a new organization-facing project service.
func NewProjectService(querier db.Querier) *ProjectService {
	return &ProjectService{
		repo: NewRepository(querier),
	}
}

// GetProject retrieves project configuration (organization view).
func (s *ProjectService) GetProject(
	ctx context.Context,
	req *connect.Request[libopsv1.GetProjectRequest],
) (*connect.Response[libopsv1.GetProjectResponse], error) {
	projectID := req.Msg.ProjectId

	if err := validation.UUID(projectID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	publicID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
	}

	project, err := s.repo.GetProjectByPublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}

	protoProject := &commonv1.ProjectConfig{
		OrganizationId:    fmt.Sprintf("%d", project.OrganizationID),
		ProjectId:         project.PublicID,
		ProjectName:       project.Name,
		CreateBranchSites: project.CreateBranchSites.Bool,
		Region:            service.FromNullString(project.GcpRegion),
		Zone:              service.FromNullString(project.GcpZone),
		MachineType:       service.FromNullString(project.MachineType),
		DiskSizeGb:        fromNullInt32(project.DiskSizeGb),
		GithubRepo:        service.FromNullStringPtr(project.GithubRepository),
		Promote:           service.DbPromoteStrategyToProto(project.PromoteStrategy),
		Status:            DbProjectStatusToProto(project.Status),
	}

	return connect.NewResponse(&libopsv1.GetProjectResponse{
		Project: protoProject,
	}), nil
}

// CreateProject creates a new project (organization).
func (s *ProjectService) CreateProject(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateProjectRequest],
) (*connect.Response[libopsv1.CreateProjectResponse], error) {
	organizationID := req.Msg.OrganizationId
	project := req.Msg.Project

	if err := validation.UUID(organizationID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if project == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project is required"))
	}

	if err := validation.ProjectName(project.ProjectName); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// Validate zone matches region
	if project.Region != "" && project.Zone != "" {
		if err := validation.GCPZoneMatchesRegion(project.Region, project.Zone); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	// Validate GitHub repository if provided
	if project.GithubRepo != nil && *project.GithubRepo != "" {
		if err := validation.GitHubRepoIsPublic(ctx, *project.GithubRepo); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	organizationPublicID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id format: %w", err))
	}

	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	accountID := userInfo.AccountID

	organization, err := s.repo.GetOrganizationByPublicID(ctx, organizationPublicID)
	if err != nil {
		return nil, err
	}

	// Organizations can set most fields but not sensitive ones
	params := db.CreateProjectParams{
		OrganizationID:            organization.ID,
		Name:                      project.ProjectName,
		GithubRepository:          toNullString(ptrToString(project.GithubRepo)),
		GithubBranch:              sql.NullString{String: "main", Valid: true},
		ComposePath:               sql.NullString{String: "", Valid: true},
		GcpRegion:                 toNullString(project.Region),
		GcpZone:                   toNullString(project.Zone),
		MachineType:               toNullString(project.MachineType),
		DiskSizeGb:                toNullInt32(project.DiskSizeGb),
		ComposeFile:               sql.NullString{String: "docker-compose.yml", Valid: true},
		ApplicationType:           sql.NullString{String: "generic", Valid: true},
		MonitoringEnabled:         sql.NullBool{Bool: false, Valid: true},
		MonitoringLogLevel:        sql.NullString{String: "INFO", Valid: true},
		MonitoringMetricsEnabled:  sql.NullBool{Bool: false, Valid: true},
		MonitoringHealthCheckPath: sql.NullString{String: "/", Valid: true},
		GcpProjectID:              sql.NullString{Valid: false}, // Set by orchestration
		GcpProjectNumber:          sql.NullString{Valid: false}, // Set by orchestration
		GithubTeamID:              sql.NullString{Valid: false}, // Set by orchestration
		CreateBranchSites:         sql.NullBool{Bool: project.CreateBranchSites, Valid: true},
		Status:                    db.NullProjectsStatus{ProjectsStatus: db.ProjectsStatusProvisioning, Valid: true},
		CreatedBy:                 sql.NullInt64{Int64: accountID, Valid: true},
		UpdatedBy:                 sql.NullInt64{Int64: accountID, Valid: true},
	}

	err = s.repo.CreateProject(ctx, params)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&libopsv1.CreateProjectResponse{
		Project: project,
	}), nil
}

// UpdateProject updates project configuration (organization-editable fields only).
func (s *ProjectService) UpdateProject(
	ctx context.Context,
	req *connect.Request[libopsv1.UpdateProjectRequest],
) (*connect.Response[libopsv1.UpdateProjectResponse], error) {
	projectID := req.Msg.ProjectId
	project := req.Msg.Project

	if err := validation.UUID(projectID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if project == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project is required"))
	}

	publicID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
	}

	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	accountID := userInfo.AccountID

	existing, err := s.repo.GetProjectByPublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}

	name := existing.Name
	githubRepository := existing.GithubRepository
	gcpRegion := existing.GcpRegion
	gcpZone := existing.GcpZone
	machineType := existing.MachineType
	diskSizeGb := existing.DiskSizeGb
	createBranchSites := existing.CreateBranchSites

	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.project_name") {
		name = project.ProjectName
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.github_repo") {
		// Validate GitHub repository if being updated
		if project.GithubRepo != nil && *project.GithubRepo != "" {
			if err := validation.GitHubRepoIsPublic(ctx, *project.GithubRepo); err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, err)
			}
		}
		githubRepository = toNullString(ptrToString(project.GithubRepo))
	}
	// Region and zone cannot be updated after project creation
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.region") {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("region cannot be updated after project creation"))
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.zone") {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("zone cannot be updated after project creation"))
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.machine_type") {
		machineType = toNullString(project.MachineType)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.disk_size_gb") {
		diskSizeGb = toNullInt32(project.DiskSizeGb)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.create_branch_sites") {
		createBranchSites = sql.NullBool{Bool: project.CreateBranchSites, Valid: true}
	}

	// Preserve all admin/orchestration fields
	params := db.UpdateProjectParams{
		Name:                      name,
		GithubRepository:          githubRepository,
		GithubBranch:              existing.GithubBranch,
		ComposePath:               existing.ComposePath,
		GcpRegion:                 gcpRegion,
		GcpZone:                   gcpZone,
		MachineType:               machineType,
		DiskSizeGb:                diskSizeGb,
		ComposeFile:               existing.ComposeFile,
		ApplicationType:           existing.ApplicationType,
		MonitoringEnabled:         existing.MonitoringEnabled,
		MonitoringLogLevel:        existing.MonitoringLogLevel,
		MonitoringMetricsEnabled:  existing.MonitoringMetricsEnabled,
		MonitoringHealthCheckPath: existing.MonitoringHealthCheckPath,
		GcpProjectID:              existing.GcpProjectID,
		GcpProjectNumber:          existing.GcpProjectNumber,
		GithubTeamID:              existing.GithubTeamID,
		CreateBranchSites:         createBranchSites,
		Status:                    db.NullProjectsStatus{ProjectsStatus: db.ProjectsStatusActive, Valid: true},
		UpdatedBy:                 sql.NullInt64{Int64: accountID, Valid: true},
		PublicID:                  publicID.String(),
	}

	err = s.repo.UpdateProject(ctx, params)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&libopsv1.UpdateProjectResponse{
		Project: project,
	}), nil
}

// DeleteProject deletes a project (must have no sites).
func (s *ProjectService) DeleteProject(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteProjectRequest],
) (*connect.Response[emptypb.Empty], error) {
	projectID := req.Msg.ProjectId

	if err := validation.UUID(projectID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	publicID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
	}

	err = s.repo.DeleteProject(ctx, publicID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// ListProjects lists projects for a organization.
func (s *ProjectService) ListProjects(
	ctx context.Context,
	req *connect.Request[libopsv1.ListProjectsRequest],
) (*connect.Response[libopsv1.ListProjectsResponse], error) {
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
			return nil, err
		}
		filterOrgID = sql.NullInt64{Int64: org.ID, Valid: true}
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

	params := db.ListUserProjectsParams{
		AccountID:            accountID,
		FilterOrganizationID: filterOrgID,
		Limit:                pageSize,
		Offset:               int32(offset),
	}

	projects, err := s.repo.ListUserProjects(ctx, params)
	if err != nil {
		return nil, err
	}

	protoProjects := make([]*commonv1.ProjectConfig, 0, len(projects))
	for _, project := range projects {
		protoProjects = append(protoProjects, &commonv1.ProjectConfig{
			OrganizationId:    project.OrganizationPublicID,
			ProjectId:         project.PublicID,
			ProjectName:       project.Name,
			CreateBranchSites: project.CreateBranchSites.Bool,
			Region:            service.FromNullString(project.GcpRegion),
			Zone:              service.FromNullString(project.GcpZone),
			MachineType:       service.FromNullString(project.MachineType),
			DiskSizeGb:        fromNullInt32(project.DiskSizeGb),
			GithubRepo:        service.FromNullStringPtr(project.GithubRepository),
			Promote:           commonv1.PromoteStrategy_PROMOTE_STRATEGY_GITHUB_TAG,
			Status:            DbProjectStatusToProto(project.Status),
		})
	}

	nextPageToken := ""
	if len(projects) == int(pageSize) {
		nextPageToken = service.GeneratePageToken(offset + int(pageSize))
	}

	return connect.NewResponse(&libopsv1.ListProjectsResponse{
		Projects:      protoProjects,
		NextPageToken: nextPageToken,
	}), nil
}

// ListProjectSites lists sites for a project.
func (s *ProjectService) ListProjectSites(
	ctx context.Context,
	req *connect.Request[libopsv1.ListProjectSitesRequest],
) (*connect.Response[libopsv1.ListProjectSitesResponse], error) {
	projectID := req.Msg.ProjectId

	if err := validation.UUID(projectID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	publicID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
	}

	project, err := s.repo.GetProjectByPublicID(ctx, publicID)
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

	siteNames := make([]string, 0, len(sites))
	for _, site := range sites {
		siteNames = append(siteNames, site.Name)
	}

	nextPageToken := ""
	if len(sites) == int(pageSize) {
		nextPageToken = service.GeneratePageToken(offset + int(pageSize))
	}

	return connect.NewResponse(&libopsv1.ListProjectSitesResponse{
		SiteNames:     siteNames,
		NextPageToken: nextPageToken,
	}), nil
}
