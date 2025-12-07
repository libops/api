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
	adminv1 "github.com/libops/api/proto/libops/v1/admin"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// AdminProjectService implements the admin-level project API.
type AdminProjectService struct {
	repo *Repository
}

// Compile-time check.
var _ libopsv1connect.AdminProjectServiceHandler = (*AdminProjectService)(nil)

// NewAdminProjectService creates a new admin project service.
func NewAdminProjectService(querier db.Querier) *AdminProjectService {
	return &AdminProjectService{
		repo: NewRepository(querier),
	}
}

// GetProject retrieves project configuration (admin view - includes all fields).
func (s *AdminProjectService) GetProject(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminGetProjectRequest],
) (*connect.Response[libopsv1.AdminGetProjectResponse], error) {
	projectID := req.Msg.ProjectId

	if projectID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id is required"))
	}

	publicID, err := uuid.Parse(projectID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid project_id format: %w", err))
	}

	project, err := s.repo.GetProjectWithOrganizationByPublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}

	protoProject := &adminv1.AdminProjectConfig{
		Config: &commonv1.ProjectConfig{
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
		},
		BillingAccount:      project.GcpBillingAccount,
		GithubWebhookUrl:    service.FromNullStringPtr(project.GithubRepository),
		GithubWebhookSecret: nil, // Sensitive - don't return
		HostDockerConfig:    nil, // Sensitive - don't return
		GcpProjectId:        service.FromNullStringPtr(project.GcpProjectID),
		GcpProjectNumber:    service.FromNullStringPtr(project.GcpProjectNumber),
		GithubTeamId:        service.FromNullStringPtr(project.GithubTeamID),
	}

	return connect.NewResponse(&libopsv1.AdminGetProjectResponse{
		Project: protoProject,
	}), nil
}

// CreateProject creates a new project (admin - can set all fields).
func (s *AdminProjectService) CreateProject(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminCreateProjectRequest],
) (*connect.Response[libopsv1.AdminCreateProjectResponse], error) {
	organizationID := req.Msg.OrganizationId
	project := req.Msg.Project

	if organizationID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("organization_id is required"))
	}

	if project == nil || project.Config == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project configuration is required"))
	}

	if project.Config.ProjectName == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_name is required"))
	}

	if err := validation.StringLength("project_name", project.Config.ProjectName, 1, 255); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if project.GcpProjectId != nil && *project.GcpProjectId != "" {
		if err := validation.GCPProjectID(*project.GcpProjectId); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	if project.GcpProjectNumber != nil && *project.GcpProjectNumber != "" {
		if err := validation.StringLength("gcp_project_number", *project.GcpProjectNumber, 12, 12); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	if project.Config.GithubRepo != nil && *project.Config.GithubRepo != "" {
		if err := validation.GitHubRepoIsPublic(ctx, *project.Config.GithubRepo); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	// Validate zone matches region
	if project.Config.Region != "" && project.Config.Zone != "" {
		if err := validation.GCPZoneMatchesRegion(project.Config.Region, project.Config.Zone); err != nil {
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

	params := db.CreateProjectParams{
		OrganizationID:            organization.ID,
		Name:                      project.Config.ProjectName,
		GithubRepository:          toNullString(ptrToString(project.Config.GithubRepo)),
		GithubBranch:              sql.NullString{String: "main", Valid: true},
		ComposePath:               sql.NullString{String: "", Valid: true},
		GcpRegion:                 toNullString(project.Config.Region),
		GcpZone:                   toNullString(project.Config.Zone),
		MachineType:               toNullString(project.Config.MachineType),
		DiskSizeGb:                toNullInt32(project.Config.DiskSizeGb),
		ComposeFile:               sql.NullString{String: "docker-compose.yml", Valid: true},
		ApplicationType:           sql.NullString{String: "generic", Valid: true},
		MonitoringEnabled:         sql.NullBool{Bool: false, Valid: true},
		MonitoringLogLevel:        sql.NullString{String: "INFO", Valid: true},
		MonitoringMetricsEnabled:  sql.NullBool{Bool: false, Valid: true},
		MonitoringHealthCheckPath: sql.NullString{String: "/", Valid: true},
		GcpProjectID:              toNullString(ptrToString(project.GcpProjectId)),
		GcpProjectNumber:          toNullString(ptrToString(project.GcpProjectNumber)),
		GithubTeamID:              toNullString(ptrToString(project.GithubTeamId)),
		CreateBranchSites:         sql.NullBool{Bool: project.Config.CreateBranchSites, Valid: true},
		Status:                    db.NullProjectsStatus{ProjectsStatus: db.ProjectsStatusProvisioning, Valid: true},
		CreatedBy:                 sql.NullInt64{Int64: accountID, Valid: true},
		UpdatedBy:                 sql.NullInt64{Int64: accountID, Valid: true},
	}

	err = s.repo.CreateProject(ctx, params)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&libopsv1.AdminCreateProjectResponse{
		Project: project,
	}), nil
}

// UpdateProject updates project configuration (admin - can update all fields).
func (s *AdminProjectService) UpdateProject(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminUpdateProjectRequest],
) (*connect.Response[libopsv1.AdminUpdateProjectResponse], error) {
	projectID := req.Msg.ProjectId
	project := req.Msg.Project

	if projectID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id is required"))
	}

	if project == nil || project.Config == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project configuration is required"))
	}

	if project.Config.ProjectName != "" {
		if err := validation.StringLength("project_name", project.Config.ProjectName, 1, 255); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	if project.GcpProjectId != nil && *project.GcpProjectId != "" {
		if err := validation.GCPProjectID(*project.GcpProjectId); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	if project.GcpProjectNumber != nil && *project.GcpProjectNumber != "" {
		if err := validation.StringLength("gcp_project_number", *project.GcpProjectNumber, 12, 12); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	if project.Config.GithubRepo != nil && *project.Config.GithubRepo != "" {
		if err := validation.GitHubRepoIsPublic(ctx, *project.Config.GithubRepo); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
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
	gcpProjectID := existing.GcpProjectID
	gcpProjectNumber := existing.GcpProjectNumber
	githubTeamID := existing.GithubTeamID
	createBranchSites := existing.CreateBranchSites

	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.config.project_name") {
		name = project.Config.ProjectName
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.config.github_repo") {
		githubRepository = toNullString(ptrToString(project.Config.GithubRepo))
	}
	// Region and zone cannot be updated after project creation
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.config.region") {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("region cannot be updated after project creation"))
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.config.zone") {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("zone cannot be updated after project creation"))
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.config.machine_type") {
		machineType = toNullString(project.Config.MachineType)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.config.disk_size_gb") {
		diskSizeGb = toNullInt32(project.Config.DiskSizeGb)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.config.create_branch_sites") {
		createBranchSites = sql.NullBool{Bool: project.Config.CreateBranchSites, Valid: true}
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.gcp_project_id") {
		gcpProjectID = toNullString(ptrToString(project.GcpProjectId))
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.gcp_project_number") {
		gcpProjectNumber = toNullString(ptrToString(project.GcpProjectNumber))
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.github_team_id") {
		githubTeamID = toNullString(ptrToString(project.GithubTeamId))
	}

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
		GcpProjectID:              gcpProjectID,
		GcpProjectNumber:          gcpProjectNumber,
		GithubTeamID:              githubTeamID,
		CreateBranchSites:         createBranchSites,
		Status:                    db.NullProjectsStatus{ProjectsStatus: db.ProjectsStatusActive, Valid: true},
		UpdatedBy:                 sql.NullInt64{Int64: accountID, Valid: true},
		PublicID:                  publicID.String(),
	}

	err = s.repo.UpdateProject(ctx, params)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&libopsv1.AdminUpdateProjectResponse{
		Project: project,
	}), nil
}

// DeleteProject deletes a project (must have no sites).
func (s *AdminProjectService) DeleteProject(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminDeleteProjectRequest],
) (*connect.Response[emptypb.Empty], error) {
	projectID := req.Msg.ProjectId

	if projectID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("project_id is required"))
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

// ListProjects lists projects for a organization (admin view).
func (s *AdminProjectService) ListProjects(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminListProjectsRequest],
) (*connect.Response[libopsv1.AdminListProjectsResponse], error) {
	var organizationID string
	if req.Msg.OrganizationId != nil {
		organizationID = *req.Msg.OrganizationId
	}

	if organizationID == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("organization_id is required"))
	}

	organizationPublicID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id format: %w", err))
	}

	organization, err := s.repo.GetOrganizationByPublicID(ctx, organizationPublicID)
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

	params := db.ListOrganizationProjectsParams{
		OrganizationID: organization.ID,
		Limit:          pageSize,
		Offset:         int32(offset),
	}

	projects, err := s.repo.ListOrganizationProjects(ctx, params)
	if err != nil {
		return nil, err
	}

	protoProjects := make([]*adminv1.AdminProjectConfig, 0, len(projects))
	for _, project := range projects {
		protoProjects = append(protoProjects, &adminv1.AdminProjectConfig{
			Config: &commonv1.ProjectConfig{
				OrganizationId:    fmt.Sprintf("%d", project.OrganizationID),
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
			},
			BillingAccount:   "",
			GcpProjectId:     service.FromNullStringPtr(project.GcpProjectID),
			GcpProjectNumber: service.FromNullStringPtr(project.GcpProjectNumber),
			GithubTeamId:     service.FromNullStringPtr(project.GithubTeamID),
		})
	}

	nextPageToken := ""
	if len(projects) == int(pageSize) {
		nextPageToken = service.GeneratePageToken(offset + int(pageSize))
	}

	return connect.NewResponse(&libopsv1.AdminListProjectsResponse{
		Projects:      protoProjects,
		NextPageToken: nextPageToken,
	}), nil
}

// ListAllProjects lists all projects across all organizations (admin only).
func (s *AdminProjectService) ListAllProjects(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminListAllProjectsRequest],
) (*connect.Response[libopsv1.AdminListAllProjectsResponse], error) {
	// TODO: Implement cross-organization project listing
	return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("list all projects not yet implemented"))
}

// Helper functions
