// Package organization provides services related to organization management, including admin-level operations.
package organization

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

// AdminOrganizationService implements the admin-level organization API.
type AdminOrganizationService struct {
	repo *Repository
}

// Compile-time check.
var _ libopsv1connect.AdminOrganizationServiceHandler = (*AdminOrganizationService)(nil)

// NewAdminOrganizationService creates a new admin organization service.
func NewAdminOrganizationService(querier db.Querier) *AdminOrganizationService {
	return &AdminOrganizationService{
		repo: NewRepository(querier),
	}
}

// GetOrganization retrieves organization configuration (admin view - includes all fields).
func (s *AdminOrganizationService) GetOrganization(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminGetOrganizationRequest],
) (*connect.Response[libopsv1.AdminGetOrganizationResponse], error) {
	organizationID := req.Msg.OrganizationId
	if err := validation.UUID(organizationID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	publicID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id format: %w", err))
	}
	organization, err := s.repo.GetOrganizationByPublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}

	folder := &adminv1.AdminFolderConfig{
		Config: &commonv1.FolderConfig{
			OrganizationId:   organization.PublicID,
			OrganizationName: organization.Name,
			Status:           DbOrganizationStatusToProto(organization.Status),
		},
		GcpParent:   organization.GcpParent,
		GcpFolderId: service.FromNullStringPtr(organization.GcpFolderID),
	}

	return connect.NewResponse(&libopsv1.AdminGetOrganizationResponse{
		Folder: folder,
	}), nil
}

// CreateOrganization creates a new organization (admin - can set all fields).
func (s *AdminOrganizationService) CreateOrganization(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminCreateOrganizationRequest],
) (*connect.Response[libopsv1.AdminCreateOrganizationResponse], error) {
	folder := req.Msg.Folder

	if folder == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("folder configuration is required"))
	}

	if folder.Config == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("folder configuration is required"))
	}

	if err := validation.OrganizationName(folder.Config.OrganizationName); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	accountID := userInfo.AccountID

	// Generate a new UUID for the organization
	newID := uuid.New().String()

	// Admin can set all fields including GCP fields
	params := db.CreateOrganizationParams{
		PublicID:          newID,
		Name:              folder.Config.OrganizationName,
		GcpOrgID:          folder.GcpParent,
		GcpBillingAccount: "",
		GcpParent:         folder.GcpParent,
		GcpFolderID:       toNullString(ptrToString(folder.GcpFolderId)),
		Status:            db.NullOrganizationsStatus{OrganizationsStatus: db.OrganizationsStatusProvisioning, Valid: true},
		CreatedBy:         sql.NullInt64{Int64: accountID, Valid: true},
		UpdatedBy:         sql.NullInt64{Int64: accountID, Valid: true},
	}

	err := s.repo.CreateOrganization(ctx, params)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&libopsv1.AdminCreateOrganizationResponse{
		OrganizationId: newID,
		Folder:         folder,
	}), nil
}

// UpdateOrganization updates organization metadata (admin - can update all fields).
func (s *AdminOrganizationService) UpdateOrganization(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminUpdateOrganizationRequest],
) (*connect.Response[libopsv1.AdminUpdateOrganizationResponse], error) {
	organizationID := req.Msg.OrganizationId
	if err := validation.UUID(organizationID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	folder := req.Msg.Folder
	if folder == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("folder configuration is required"))
	}

	if folder.Config != nil && folder.Config.OrganizationName != "" {
		if err := validation.StringLength("organization_name", folder.Config.OrganizationName, 1, 255); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	publicID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id format: %w", err))
	}

	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	accountID := userInfo.AccountID

	existing, err := s.repo.GetOrganizationByPublicID(ctx, publicID)
	if err != nil {
		return nil, err
	}

	// Apply field mask - admin can update all fields
	name := existing.Name
	gcpOrgID := existing.GcpOrgID
	gcpBillingAccount := existing.GcpBillingAccount
	gcpParent := existing.GcpParent
	gcpFolderID := existing.GcpFolderID

	if service.ShouldUpdateField(req.Msg.UpdateMask, "folder.config.organization_name") {
		name = folder.Config.OrganizationName
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "folder.gcp_parent") {
		gcpOrgID = folder.GcpParent
		gcpParent = folder.GcpParent
	}
	if service.ShouldUpdateField(req.Msg.UpdateMask, "folder.gcp_folder_id") {
		gcpFolderID = toNullString(ptrToString(folder.GcpFolderId))
	}

	params := db.UpdateOrganizationParams{
		Name:              name,
		GcpOrgID:          gcpOrgID,
		GcpBillingAccount: gcpBillingAccount,
		GcpParent:         gcpParent,
		GcpFolderID:       gcpFolderID,
		Status:            db.NullOrganizationsStatus{OrganizationsStatus: db.OrganizationsStatusActive, Valid: true},
		UpdatedBy:         sql.NullInt64{Int64: accountID, Valid: true},
		PublicID:          publicID.String(),
	}

	err = s.repo.UpdateOrganization(ctx, params)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&libopsv1.AdminUpdateOrganizationResponse{
		Folder: folder,
	}), nil
}

// DeleteOrganization deletes a organization (must have no projects).
func (s *AdminOrganizationService) DeleteOrganization(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminDeleteOrganizationRequest],
) (*connect.Response[emptypb.Empty], error) {
	organizationID := req.Msg.OrganizationId
	if err := validation.UUID(organizationID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	publicID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id format: %w", err))
	}
	err = s.repo.DeleteOrganization(ctx, publicID)
	if err != nil {
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// ListOrganizations lists all organizations (admin view).
func (s *AdminOrganizationService) ListOrganizations(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminListOrganizationsRequest],
) (*connect.Response[libopsv1.AdminListOrganizationsResponse], error) {
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

	params := db.ListOrganizationsParams{
		Limit:  pageSize,
		Offset: int32(offset),
	}

	organizations, err := s.repo.ListOrganizations(ctx, params)
	if err != nil {
		return nil, err
	}

	protoOrganizations := make([]*adminv1.AdminFolderConfig, 0, len(organizations))
	for _, organization := range organizations {
		protoOrganizations = append(protoOrganizations, &adminv1.AdminFolderConfig{
			Config: &commonv1.FolderConfig{
				OrganizationId:   organization.PublicID,
				OrganizationName: organization.Name,
				Status:           DbOrganizationStatusToProto(organization.Status),
			},
			GcpParent:   organization.GcpParent,
			GcpFolderId: service.FromNullStringPtr(organization.GcpFolderID),
		})
	}

	nextPageToken := ""
	if len(organizations) == int(pageSize) {
		nextPageToken = service.GeneratePageToken(offset + int(pageSize))
	}

	return connect.NewResponse(&libopsv1.AdminListOrganizationsResponse{
		Organizations: protoOrganizations,
		NextPageToken: nextPageToken,
	}), nil
}

// ListOrganizationProjects lists projects for a organization (admin view).
func (s *AdminOrganizationService) ListOrganizationProjects(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminListOrganizationProjectsRequest],
) (*connect.Response[libopsv1.AdminListOrganizationProjectsResponse], error) {
	organizationID := req.Msg.OrganizationId
	if err := validation.UUID(organizationID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	publicID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id format: %w", err))
	}

	organization, err := s.repo.GetOrganizationByPublicID(ctx, publicID)
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

	projectIDs := make([]string, 0, len(projects))
	for _, project := range projects {
		projectIDs = append(projectIDs, project.PublicID)
	}

	nextPageToken := ""
	if len(projects) == int(pageSize) {
		nextPageToken = service.GeneratePageToken(offset + int(pageSize))
	}

	return connect.NewResponse(&libopsv1.AdminListOrganizationProjectsResponse{
		ProjectIds:    projectIDs,
		NextPageToken: nextPageToken,
	}), nil
}
