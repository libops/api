package project

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/libops/api/db"
	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/billing"
	"github.com/libops/api/internal/service"
	"github.com/libops/api/internal/validation"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	adminv1 "github.com/libops/api/proto/libops/v1/admin"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// AdminProjectService implements the admin-level project API.
type AdminProjectService struct {
	repo           *Repository
	billingManager BillingManager
}

// Compile-time check.
var _ libopsv1connect.AdminProjectServiceHandler = (*AdminProjectService)(nil)

// NewAdminProjectServiceWithConfig creates a new admin project service with config-based billing
func NewAdminProjectServiceWithConfig(querier db.Querier, disableBilling bool) *AdminProjectService {
	var billingMgr BillingManager
	if disableBilling {
		billingMgr = billing.NewNoOpBillingManager()
	} else {
		billingMgr = billing.NewStripeManager(querier)
	}

	return &AdminProjectService{
		repo:           NewRepository(querier),
		billingManager: billingMgr,
	}
}

// NewAdminProjectServiceWithBilling creates a new admin project service with a custom billing manager (for testing).
func NewAdminProjectServiceWithBilling(querier db.Querier, billingMgr BillingManager) *AdminProjectService {
	return &AdminProjectService{
		repo:           NewRepository(querier),
		billingManager: billingMgr,
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
			DiskSizeGb:        service.FromNullInt32(project.DiskSizeGb),
			Os:                service.FromNullString(project.Os),
			DiskType:          service.FromNullString(project.DiskType),
			Promote:           service.DbPromoteStrategyToProto(project.PromoteStrategy),
			Status:            DbProjectStatusToProto(project.Status),
		},
		BillingAccount:   project.GcpBillingAccount,
		GcpProjectId:     service.FromNullStringPtr(project.GcpProjectID),
		GcpProjectNumber: service.FromNullStringPtr(project.GcpProjectNumber),
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

	// Validate machine type and disk size
	machineType := project.Config.MachineType
	if machineType == "" {
		machineType = "e2-medium" // Default
	}

	diskSizeGB := project.Config.DiskSizeGb
	if diskSizeGB == 0 {
		diskSizeGB = 20 // Default
	}

	if diskSizeGB < 10 || diskSizeGB > 2000 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("disk_size_gb must be between 10 and 2000"))
	}

	// Check if this is the first project for the organization (onboarding flow)
	// During onboarding, the Stripe subscription is already created with machine+disk items
	// So we skip adding billing items for the first project to avoid duplicates
	allProjects, err := s.repo.db.ListProjects(ctx, db.ListProjectsParams{
		Limit:  100,
		Offset: 0,
	})
	if err != nil {
		slog.Error("Failed to check existing projects", "error", err)
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to check existing projects: %w", err))
	}

	// Count projects for this organization
	var orgProjectCount int
	for _, p := range allProjects {
		if p.OrganizationID == organization.ID {
			orgProjectCount++
		}
	}

	var machineItemID string
	isFirstProject := orgProjectCount == 0

	if !isFirstProject {
		// Add project to Stripe subscription (adds machine + increases disk)
		// Only for projects created after onboarding
		machineItemID, err = s.billingManager.AddProjectToSubscription(
			ctx,
			organization.ID,
			project.Config.ProjectName,
			machineType,
			int(diskSizeGB),
		)
		if err != nil {
			slog.Error("Failed to add project to Stripe subscription", "error", err, "project", project.Config.ProjectName)
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to setup billing for project: %w", err))
		}
	} else {
		slog.Info("Skipping billing setup for first project (onboarding)", "project", project.Config.ProjectName, "org_id", organization.ID)
		// Machine item ID will be empty for first project - it's already in the subscription from onboarding
		machineItemID = ""
	}

	projectPublicID := uuid.New().String()
	params := db.CreateProjectParams{
		PublicID:                  projectPublicID,
		OrganizationID:            organization.ID,
		Name:                      project.Config.ProjectName,
		GcpRegion:                 service.ToNullString(project.Config.Region),
		GcpZone:                   service.ToNullString(project.Config.Zone),
		MachineType:               sql.NullString{String: machineType, Valid: true},
		DiskSizeGb:                sql.NullInt32{Int32: diskSizeGB, Valid: true},
		StripeSubscriptionItemID:  sql.NullString{String: machineItemID, Valid: true},
		MonitoringEnabled:         sql.NullBool{Bool: false, Valid: true},
		MonitoringLogLevel:        sql.NullString{String: "INFO", Valid: true},
		MonitoringMetricsEnabled:  sql.NullBool{Bool: false, Valid: true},
		MonitoringHealthCheckPath: sql.NullString{String: "/", Valid: true},
		GcpProjectID:              service.ToNullString(service.PtrToString(project.GcpProjectId)),
		GcpProjectNumber:          service.ToNullString(service.PtrToString(project.GcpProjectNumber)),
		CreateBranchSites:         sql.NullBool{Bool: project.Config.CreateBranchSites, Valid: true},
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

	slog.Info("Project created with billing (admin)",
		"project", project.Config.ProjectName,
		"organization_id", organization.ID,
		"machine_type", machineType,
		"disk_gb", diskSizeGB,
		"stripe_item_id", machineItemID)

	// Set the project ID in the response
	project.Config.ProjectId = projectPublicID

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
	gcpProjectID := existing.GcpProjectID
	gcpProjectNumber := existing.GcpProjectNumber
	createBranchSites := existing.CreateBranchSites

	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.config.project_name") {
		name = project.Config.ProjectName
	}
	// Region and zone cannot be updated after project creation
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.config.region") {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("region cannot be updated after project creation"))
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.config.zone") {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("zone cannot be updated after project creation"))
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.config.machine_type") {
		machineType = service.ToNullString(project.Config.MachineType)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.config.disk_size_gb") {
		diskSizeGb = service.ToNullInt32(project.Config.DiskSizeGb)
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.config.create_branch_sites") {
		createBranchSites = sql.NullBool{Bool: project.Config.CreateBranchSites, Valid: true}
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.gcp_project_id") {
		gcpProjectID = service.ToNullString(service.PtrToString(project.GcpProjectId))
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "project.gcp_project_number") {
		gcpProjectNumber = service.ToNullString(service.PtrToString(project.GcpProjectNumber))
	}

	params := db.UpdateProjectParams{
		Name:                      name,
		GcpRegion:                 gcpRegion,
		GcpZone:                   gcpZone,
		MachineType:               machineType,
		DiskSizeGb:                diskSizeGb,
		MonitoringEnabled:         existing.MonitoringEnabled,
		MonitoringLogLevel:        existing.MonitoringLogLevel,
		MonitoringMetricsEnabled:  existing.MonitoringMetricsEnabled,
		MonitoringHealthCheckPath: existing.MonitoringHealthCheckPath,
		GcpProjectID:              gcpProjectID,
		GcpProjectNumber:          gcpProjectNumber,
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
			slog.Info("Removed project from Stripe subscription (admin)",
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
				DiskSizeGb:        service.FromNullInt32(project.DiskSizeGb),
				Os:                service.FromNullString(project.Os),
				DiskType:          service.FromNullString(project.DiskType),
				Promote:           commonv1.PromoteStrategy_PROMOTE_STRATEGY_GITHUB_TAG,
				Status:            DbProjectStatusToProto(project.Status),
			},
			BillingAccount:   "",
			GcpProjectId:     service.FromNullStringPtr(project.GcpProjectID),
			GcpProjectNumber: service.FromNullStringPtr(project.GcpProjectNumber),
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

// SshKeysResponse is the JSON response format for SSH keys.
type SshKeysResponse struct {
	SshKeys []string `json:"ssh_keys"`
}

// HandleProjectSshKeys returns SSH keys for all owners and developers of a project.
// This is a plain HTTP handler for VM reconciliation services.
func (s *AdminProjectService) HandleProjectSshKeys(w http.ResponseWriter, r *http.Request) {
	projectID := r.PathValue("projectId")
	if projectID == "" {
		http.Error(w, "project ID is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	keys, err := s.repo.db.ListSshKeysByProject(ctx, db.ListSshKeysByProjectParams{
		ProjectPublicID: projectID,
	})
	if err != nil {
		slog.Error("failed to fetch project SSH keys", "project_id", projectID, "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}

	response := SshKeysResponse{
		SshKeys: keys,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(response); err != nil {
		slog.Error("failed to encode response", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
		return
	}
}

// Helper functions
