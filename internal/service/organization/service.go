package organization

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/libops/api/db"
	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/config"
	"github.com/libops/api/internal/service"
	"github.com/libops/api/internal/validation"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	commonv1 "github.com/libops/api/proto/libops/v1/common"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// OrganizationService implements the organization-facing organization API.
type OrganizationService struct {
	repo   *Repository
	config *config.Config
}

// Compile-time check.
var _ libopsv1connect.OrganizationServiceHandler = (*OrganizationService)(nil)

// NewOrganizationService creates a new organization-facing organization service.
func NewOrganizationService(querier db.Querier, cfg *config.Config) *OrganizationService {
	return &OrganizationService{
		repo:   NewRepository(querier),
		config: cfg,
	}
}

// GetOrganization retrieves organization configuration (organization view - limited fields).
func (s *OrganizationService) GetOrganization(
	ctx context.Context,
	req *connect.Request[libopsv1.GetOrganizationRequest],
) (*connect.Response[libopsv1.GetOrganizationResponse], error) {
	organizationID := req.Msg.OrganizationId

	if err := validation.UUID(organizationID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	publicID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id format: %w", err))
	}

	// Membership check is handled automatically by scope interceptor via proto annotation
	organization, err := s.repo.GetOrganizationByPublicID(ctx, publicID)
	if err != nil {
		slog.Error("Failed to get organization by public ID", "error", err, "organization_id", organizationID)
		return nil, err
	}

	folder := &commonv1.FolderConfig{
		OrganizationId:   organization.PublicID,
		OrganizationName: organization.Name,
		Status:           service.DbOrganizationStatusToProto(organization.Status),
	}

	return connect.NewResponse(&libopsv1.GetOrganizationResponse{
		Folder: folder,
	}), nil
}

// CreateOrganization creates a new organization.
func (s *OrganizationService) CreateOrganization(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateOrganizationRequest],
) (*connect.Response[libopsv1.CreateOrganizationResponse], error) {
	folder := req.Msg.Folder

	if folder == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("folder configuration is required"))
	}

	if err := validation.OrganizationName(folder.OrganizationName); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	accountID := userInfo.AccountID

	// Validate organization limit
	if err := s.repo.ValidateOrganizationLimit(ctx, accountID); err != nil {
		return nil, err
	}

	newID := uuid.New().String()

	// Use the shared repository method that creates org, adds owner, and creates relationship
	_, err := s.repo.CreateOrganizationWithOwner(
		ctx,
		newID,
		folder.OrganizationName,
		s.config.GcpOrgID,
		s.config.GcpBillingAccount,
		s.config.GcpParent,
		accountID,
		s.config.RootOrganizationID,
	)
	if err != nil {
		slog.Error("Failed to create organization", "error", err, "organization_name", folder.OrganizationName, "account_id", accountID)
		return nil, err
	}

	return connect.NewResponse(&libopsv1.CreateOrganizationResponse{
		OrganizationId: newID,
		Folder:         folder,
	}), nil
}

// UpdateOrganization updates organization metadata (organization-editable fields only).
func (s *OrganizationService) UpdateOrganization(
	ctx context.Context,
	req *connect.Request[libopsv1.UpdateOrganizationRequest],
) (*connect.Response[libopsv1.UpdateOrganizationResponse], error) {
	organizationID := req.Msg.OrganizationId
	if err := validation.UUID(organizationID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}
	folder := req.Msg.Folder
	if folder == nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("folder configuration is required"))
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
		slog.Error("Failed to get organization by public ID for update", "error", err, "organization_id", organizationID)
		return nil, err
	}

	// Apply field mask - organizations can only update name
	name := existing.Name
	if service.ShouldUpdateField(req.Msg.UpdateMask, "folder.organization_name") {
		name = folder.OrganizationName
	}

	// Preserve all admin fields
	params := db.UpdateOrganizationParams{
		Name:              name,
		GcpOrgID:          existing.GcpOrgID,
		GcpBillingAccount: existing.GcpBillingAccount,
		GcpParent:         existing.GcpParent,
		GcpFolderID:       existing.GcpFolderID,
		Status:            existing.Status,
		UpdatedBy:         sql.NullInt64{Int64: accountID, Valid: true},
		PublicID:          publicID.String(),
	}

	err = s.repo.UpdateOrganization(ctx, params)
	if err != nil {
		slog.Error("Failed to update organization in DB", "error", err, "organization_id", organizationID)
		return nil, err
	}

	return connect.NewResponse(&libopsv1.UpdateOrganizationResponse{
		Folder: folder,
	}), nil
}

// DeleteOrganization deletes a organization (must have no projects).
func (s *OrganizationService) DeleteOrganization(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteOrganizationRequest],
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
		slog.Error("Failed to delete organization", "error", err, "organization_id", organizationID)
		return nil, err
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// ListOrganizations lists all organizations with pagination.
func (s *OrganizationService) ListOrganizations(
	ctx context.Context,
	req *connect.Request[libopsv1.ListOrganizationsRequest],
) (*connect.Response[libopsv1.ListOrganizationsResponse], error) {
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	pagination, err := service.ParsePagination(req.Msg.PageSize, req.Msg.PageToken)
	if err != nil {
		return nil, err
	}

	organizations, err := s.repo.ListOrganizations(ctx, db.ListOrganizationsParams{
		AccountID: userInfo.AccountID,
		Limit:     pagination.Limit,
		Offset:    pagination.Offset,
	})
	if err != nil {
		slog.Error("Failed to list organizations", "error", err, "account_id", userInfo.AccountID)
		return nil, err
	}

	protoOrganizations := make([]*commonv1.FolderConfig, 0, len(organizations))
	for _, organization := range organizations {
		protoOrganizations = append(protoOrganizations, &commonv1.FolderConfig{
			OrganizationId:   organization.PublicID,
			OrganizationName: organization.Name,
			Status:           service.DbOrganizationStatusToProto(organization.Status),
		})
	}

	paginationResult := service.MakePaginationResult(len(organizations), pagination)

	return connect.NewResponse(&libopsv1.ListOrganizationsResponse{
		Organizations: protoOrganizations,
		NextPageToken: paginationResult.NextPageToken,
	}), nil
}

// ListOrganizationProjects lists projects for a organization.
func (s *OrganizationService) ListOrganizationProjects(
	ctx context.Context,
	req *connect.Request[libopsv1.ListOrganizationProjectsRequest],
) (*connect.Response[libopsv1.ListOrganizationProjectsResponse], error) {
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
		slog.Error("Failed to get organization by public ID for project listing", "error", err, "organization_id", organizationID)
		return nil, err
	}

	pagination, err := service.ParsePagination(req.Msg.PageSize, req.Msg.PageToken)
	if err != nil {
		return nil, err
	}

	projects, err := s.repo.ListOrganizationProjects(ctx, db.ListOrganizationProjectsParams{
		OrganizationID: organization.ID,
		Limit:          pagination.Limit,
		Offset:         pagination.Offset,
	})
	if err != nil {
		slog.Error("Failed to list organization projects", "error", err, "organization_id", organization.ID)
		return nil, err
	}

	projectIDs := make([]string, 0, len(projects))
	for _, project := range projects {
		projectIDs = append(projectIDs, project.PublicID)
	}

	paginationResult := service.MakePaginationResult(len(projects), pagination)

	return connect.NewResponse(&libopsv1.ListOrganizationProjectsResponse{
		ProjectIds:    projectIDs,
		NextPageToken: paginationResult.NextPageToken,
	}), nil
}

// Helper functions

// ShouldUpdateField checks if a field should be updated based on the field mask.
// If the field mask is nil or empty, all fields should be updated; otherwise, only fields present in the mask should be updated.
func ShouldUpdateField(mask *fieldmaskpb.FieldMask, field string) bool {
	if mask == nil {
		return true
	}
	for _, path := range mask.Paths {
		if path == field {
			return true
		}
	}
	return false
}
