package project

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/billing"
	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/service"
	"github.com/libops/api/internal/validation"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// BillingManager defines the interface for billing operations.
type BillingManager interface {
	ValidateMachineType(ctx context.Context, machineType string) error
	ValidateDiskSize(ctx context.Context, diskSizeGB int) error
	AddProjectToSubscription(ctx context.Context, organizationID int64, projectName, machineType string, diskSizeGB int) (machineItemID string, err error)
	RemoveProjectFromSubscription(ctx context.Context, machineItemID string, diskSizeGB int, organizationID int64) error
	UpdateProjectMachine(ctx context.Context, oldMachineItemID, newMachineType, projectName string, organizationID int64) (newMachineItemID string, err error)
	UpdateProjectDiskSize(ctx context.Context, organizationID int64, oldDiskSizeGB, newDiskSizeGB int) error
}

// ProjectService implements the organization-facing project API.
type ProjectService struct {
	repo           *Repository
	billingManager BillingManager
}

// Compile-time check.
var _ libopsv1connect.ProjectServiceHandler = (*ProjectService)(nil)

// NewProjectServiceWithConfig creates a new project service with config-based billing
func NewProjectServiceWithConfig(querier db.Querier, disableBilling bool) *ProjectService {
	var billingMgr BillingManager
	if disableBilling {
		billingMgr = billing.NewNoOpBillingManager()
	} else {
		billingMgr = billing.NewStripeManager(querier)
	}

	return &ProjectService{
		repo:           NewRepository(querier),
		billingManager: billingMgr,
	}
}

// NewProjectServiceWithBilling creates a new project service with a custom billing manager (for testing).
func NewProjectServiceWithBilling(querier db.Querier, billingMgr BillingManager) *ProjectService {
	return &ProjectService{
		repo:           NewRepository(querier),
		billingManager: billingMgr,
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
		DiskSizeGb:        service.FromNullInt32(project.DiskSizeGb),
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

	// Validate machine type and disk size
	machineType := project.MachineType
	if machineType == "" {
		machineType = "e2-medium" // Default
	}

	diskSizeGB := project.DiskSizeGb
	if diskSizeGB == 0 {
		diskSizeGB = 20 // Default
	}

	if diskSizeGB < 10 || diskSizeGB > 2000 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("disk_size_gb must be between 10 and 2000"))
	}

	// Add project to Stripe subscription (adds machine + increases disk)
	machineItemID, err := s.billingManager.AddProjectToSubscription(
		ctx,
		organization.ID,
		project.ProjectName,
		machineType,
		int(diskSizeGB),
	)
	if err != nil {
		slog.Error("Failed to add project to Stripe subscription", "error", err, "project", project.ProjectName)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to setup billing for project: %w", err))
	}

	// Organizations can set most fields but not sensitive ones
	projectPublicID := uuid.New().String()
	params := db.CreateProjectParams{
		PublicID:                  projectPublicID,
		OrganizationID:            organization.ID,
		Name:                      project.ProjectName,
		GcpRegion:                 service.ToNullString(project.Region),
		GcpZone:                   service.ToNullString(project.Zone),
		MachineType:               sql.NullString{String: machineType, Valid: true},
		DiskSizeGb:                sql.NullInt32{Int32: diskSizeGB, Valid: true},
		StripeSubscriptionItemID:  sql.NullString{String: machineItemID, Valid: true},
		MonitoringEnabled:         sql.NullBool{Bool: false, Valid: true},
		MonitoringLogLevel:        sql.NullString{String: "INFO", Valid: true},
		MonitoringMetricsEnabled:  sql.NullBool{Bool: false, Valid: true},
		MonitoringHealthCheckPath: sql.NullString{String: "/", Valid: true},
		GcpProjectID:              sql.NullString{Valid: false}, // Set by orchestration
		GcpProjectNumber:          sql.NullString{Valid: false}, // Set by orchestration
		CreateBranchSites:         sql.NullBool{Bool: project.CreateBranchSites, Valid: true},
		Status:                    db.NullProjectsStatus{ProjectsStatus: db.ProjectsStatusProvisioning, Valid: true},
		CreatedBy:                 sql.NullInt64{Int64: accountID, Valid: true},
		UpdatedBy:                 sql.NullInt64{Int64: accountID, Valid: true},
	}

	err = s.repo.CreateProject(ctx, params)
	if err != nil {
		// Rollback: remove from Stripe if database creation fails
		slog.Error("Failed to create project in database, rolling back Stripe", "error", err)
		_ = s.billingManager.RemoveProjectFromSubscription(ctx, machineItemID, int(diskSizeGB), organization.ID)
		return nil, err
	}

	slog.Info("Project created with billing",
		"project", project.ProjectName,
		"organization_id", organization.ID,
		"machine_type", machineType,
		"disk_gb", diskSizeGB,
		"stripe_item_id", machineItemID)

	// Set the project ID in the response
	project.ProjectId = projectPublicID

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
	gcpRegion := existing.GcpRegion
	gcpZone := existing.GcpZone
	machineType := existing.MachineType
	diskSizeGb := existing.DiskSizeGb
	createBranchSites := existing.CreateBranchSites

	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.project_name") {
		name = project.ProjectName
	}
	// Region and zone cannot be updated after project creation
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.region") {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("region cannot be updated after project creation"))
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.zone") {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("zone cannot be updated after project creation"))
	}
	// Track changes for billing updates
	machineTypeChanged := false
	diskSizeChanged := false
	var newMachineItemID string

	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.machine_type") {
		newMachineType := project.MachineType
		if newMachineType != "" && newMachineType != existing.MachineType.String {
			// Validate new machine type
			if err := s.billingManager.ValidateMachineType(ctx, newMachineType); err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, err)
			}

			// Update machine type in Stripe
			if existing.StripeSubscriptionItemID.Valid && existing.StripeSubscriptionItemID.String != "" {
				newItemID, err := s.billingManager.UpdateProjectMachine(
					ctx,
					existing.StripeSubscriptionItemID.String,
					newMachineType,
					existing.Name,
					existing.OrganizationID,
				)
				if err != nil {
					slog.Error("Failed to update machine type in Stripe", "error", err)
					return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update billing: %w", err))
				}
				newMachineItemID = newItemID
				machineTypeChanged = true
			}

			machineType = sql.NullString{String: newMachineType, Valid: true}
		}
	}

	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.disk_size_gb") {
		newDiskSize := project.DiskSizeGb
		oldDiskSize := int32(20) // Default
		if existing.DiskSizeGb.Valid {
			oldDiskSize = existing.DiskSizeGb.Int32
		}

		if newDiskSize > 0 && newDiskSize != oldDiskSize {
			// Validate new disk size
			if err := s.billingManager.ValidateDiskSize(ctx, int(newDiskSize)); err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, err)
			}

			// Update disk size in Stripe
			err := s.billingManager.UpdateProjectDiskSize(
				ctx,
				existing.OrganizationID,
				int(oldDiskSize),
				int(newDiskSize),
			)
			if err != nil {
				slog.Error("Failed to update disk size in Stripe", "error", err)
				return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to update billing: %w", err))
			}
			diskSizeChanged = true
			diskSizeGb = sql.NullInt32{Int32: newDiskSize, Valid: true}
		}
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.create_branch_sites") {
		createBranchSites = sql.NullBool{Bool: project.CreateBranchSites, Valid: true}
	}

	// Update Stripe subscription item ID if machine type changed
	stripeSubItemID := existing.StripeSubscriptionItemID
	if machineTypeChanged && newMachineItemID != "" {
		stripeSubItemID = sql.NullString{String: newMachineItemID, Valid: true}
	}

	// Preserve all admin/orchestration fields
	params := db.UpdateProjectParams{
		Name:                      name,
		GcpRegion:                 gcpRegion,
		GcpZone:                   gcpZone,
		MachineType:               machineType,
		DiskSizeGb:                diskSizeGb,
		StripeSubscriptionItemID:  stripeSubItemID,
		MonitoringEnabled:         existing.MonitoringEnabled,
		MonitoringLogLevel:        existing.MonitoringLogLevel,
		MonitoringMetricsEnabled:  existing.MonitoringMetricsEnabled,
		MonitoringHealthCheckPath: existing.MonitoringHealthCheckPath,
		GcpProjectID:              existing.GcpProjectID,
		GcpProjectNumber:          existing.GcpProjectNumber,
		CreateBranchSites:         createBranchSites,
		Status:                    db.NullProjectsStatus{ProjectsStatus: db.ProjectsStatusActive, Valid: true},
		UpdatedBy:                 sql.NullInt64{Int64: accountID, Valid: true},
		PublicID:                  publicID.String(),
	}

	if machineTypeChanged || diskSizeChanged {
		slog.Info("Project configuration updated with billing changes",
			"project_id", publicID.String(),
			"machine_changed", machineTypeChanged,
			"disk_changed", diskSizeChanged)
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

	// Get project to retrieve billing info
	project, err := s.repo.GetProjectByPublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}

	// Remove project from Stripe subscription
	if project.StripeSubscriptionItemID.Valid && project.StripeSubscriptionItemID.String != "" {
		diskSize := 20 // Default
		if project.DiskSizeGb.Valid {
			diskSize = int(project.DiskSizeGb.Int32)
		}

		err = s.billingManager.RemoveProjectFromSubscription(
			ctx,
			project.StripeSubscriptionItemID.String,
			diskSize,
			project.OrganizationID,
		)
		if err != nil {
			// Log error but don't fail deletion - we don't want orphaned projects
			slog.Error("Failed to remove project from Stripe, continuing with deletion",
				"error", err,
				"project_id", projectID,
				"stripe_item_id", project.StripeSubscriptionItemID.String)
		} else {
			slog.Info("Removed project from Stripe subscription",
				"project", project.Name,
				"organization_id", project.OrganizationID,
				"stripe_item_id", project.StripeSubscriptionItemID.String)
		}
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
			DiskSizeGb:        service.FromNullInt32(project.DiskSizeGb),
			Promote:           commonv1.PromoteStrategy_PROMOTE_STRATEGY_GITHUB_TAG,
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
