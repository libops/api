package organization

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/libops/api/db"
	"github.com/libops/api/internal/reconciler"
	"github.com/libops/api/internal/service"
	"github.com/libops/api/internal/validation"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// MemberService implements the LibOps MemberService API.
type MemberService struct {
	db          db.Querier
	connManager *reconciler.ConnectionManager
}

// Compile-time check.
var _ libopsv1connect.MemberServiceHandler = (*MemberService)(nil)

// NewMemberService creates a new MemberService instance with DI.
func NewMemberService(querier db.Querier, connManager *reconciler.ConnectionManager) *MemberService {
	return &MemberService{
		db:          querier,
		connManager: connManager,
	}
}

// ListOrganizationMembers lists members of a organization.
func (s *MemberService) ListOrganizationMembers(
	ctx context.Context,
	req *connect.Request[libopsv1.ListOrganizationMembersRequest],
) (*connect.Response[libopsv1.ListOrganizationMembersResponse], error) {
	organizationID := req.Msg.OrganizationId
	if err := validation.UUID(organizationID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	publicID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id format: %w", err))
	}

	organization, err := s.db.GetOrganization(ctx, publicID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(
				connect.CodeNotFound,
				fmt.Errorf("organization with ID '%s' not found", organizationID),
			)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
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

	params := db.ListOrganizationMembersParams{
		OrganizationID: organization.ID,
		Limit:          pageSize,
		Offset:         int32(offset),
	}

	members, err := s.db.ListOrganizationMembers(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	protoMembers := make([]*libopsv1.MemberDetail, 0, len(members))
	for _, member := range members {
		protoMembers = append(protoMembers, &libopsv1.MemberDetail{
			AccountId:      member.AccountPublicID,
			Email:          member.Email,
			Name:           service.FromNullString(member.Name),
			Role:           string(member.Role),
			GithubUsername: service.FromNullStringPtr(member.GithubUsername),
			Status:         service.DbOrganizationMemberStatusToProto(member.Status),
		})
	}

	nextPageToken := ""
	if len(members) == int(pageSize) {
		nextPageToken = service.GeneratePageToken(offset + int(pageSize))
	}

	return connect.NewResponse(&libopsv1.ListOrganizationMembersResponse{
		Members:       protoMembers,
		NextPageToken: nextPageToken,
	}), nil
}

// CreateOrganizationMember creates a member for a organization.
func (s *MemberService) CreateOrganizationMember(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateOrganizationMemberRequest],
) (*connect.Response[libopsv1.CreateOrganizationMemberResponse], error) {
	organizationID := req.Msg.OrganizationId
	accountID := req.Msg.AccountId
	role := req.Msg.Role

	if err := validation.UUID(organizationID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.UUID(accountID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if !service.IsValidMemberRole(role) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid role: %s", role))
	}

	organizationPublicID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id format: %w", err))
	}

	accountPublicID, err := uuid.Parse(accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid account_id format: %w", err))
	}

	organization, err := s.db.GetOrganization(ctx, organizationPublicID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	account, err := s.db.GetAccount(ctx, accountPublicID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("account not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Determine initial status based on role
	// Owner/developer roles require reconciliation (SSH keys, secrets, firewall)
	// so they start in 'provisioning' state and will be set to 'active' after reconciliation
	status := db.OrganizationMembersStatusActive
	if role == "owner" || role == "developer" {
		status = db.OrganizationMembersStatusProvisioning
	}

	params := db.CreateOrganizationMemberParams{
		OrganizationID: organization.ID,
		AccountID:      account.ID,
		Role:           db.OrganizationMembersRole(role),
		Status:         db.NullOrganizationMembersStatus{OrganizationMembersStatus: status, Valid: true},
		CreatedBy:      sql.NullInt64{Valid: false},
		UpdatedBy:      sql.NullInt64{Valid: false},
	}

	err = s.db.CreateOrganizationMember(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Trigger reconciliation via WebSocket if owner/developer role
	if s.connManager != nil && (role == "owner" || role == "developer") {
		// Get all projects in this organization
		projects, err := s.db.ListOrganizationProjects(ctx, db.ListOrganizationProjectsParams{
			OrganizationID: organization.ID,
			Limit:          1000, // Max projects per org
			Offset:         0,
		})
		if err != nil {
			slog.Warn("failed to get organization projects for reconciliation",
				"organization_id", organizationID,
				"error", err)
		} else {
			// Get all sites across all projects
			for _, project := range projects {
				sites, err := s.db.ListProjectSites(ctx, db.ListProjectSitesParams{
					ProjectID: project.ID,
					Limit:     1000, // Max sites per project
					Offset:    0,
				})
				if err != nil {
					slog.Warn("failed to get project sites for reconciliation",
						"project_id", project.PublicID,
						"error", err)
					continue
				}

				// Trigger SSH key reconciliation for each connected site
				for _, site := range sites {
					if err := s.connManager.TriggerReconciliation(site.ID, "ssh_keys"); err != nil {
						slog.Debug("site not connected, skipping reconciliation",
							"site_id", site.PublicID,
							"error", err)
					} else {
						slog.Info("triggered ssh_keys reconciliation for org member addition",
							"site_id", site.PublicID,
							"organization_id", organizationID)
					}
				}
			}
		}
	}

	member := &libopsv1.MemberDetail{
		AccountId:      accountID,
		Email:          account.Email,
		Name:           service.FromNullString(account.Name),
		Role:           role,
		Status:         service.DbStatusToProto(string(status)),
		GithubUsername: service.FromNullStringPtr(account.GithubUsername),
	}

	return connect.NewResponse(&libopsv1.CreateOrganizationMemberResponse{
		Member: member,
	}), nil
}

// UpdateOrganizationMember updates a organization member's role.
func (s *MemberService) UpdateOrganizationMember(
	ctx context.Context,
	req *connect.Request[libopsv1.UpdateOrganizationMemberRequest],
) (*connect.Response[libopsv1.UpdateOrganizationMemberResponse], error) {
	organizationID := req.Msg.OrganizationId
	accountID := req.Msg.AccountId
	role := req.Msg.Role

	if err := validation.UUID(organizationID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.UUID(accountID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if !service.IsValidMemberRole(role) {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid role: %s", role))
	}

	organizationPublicID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id format: %w", err))
	}

	accountPublicID, err := uuid.Parse(accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid account_id format: %w", err))
	}

	organization, err := s.db.GetOrganization(ctx, organizationPublicID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	account, err := s.db.GetAccount(ctx, accountPublicID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("account not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	existingMember, err := s.db.GetOrganizationMember(ctx, db.GetOrganizationMemberParams{
		OrganizationID: organization.ID,
		AccountID:      account.ID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	memberRole := existingMember.Role
	if service.ShouldUpdateField(req.Msg.UpdateMask, "role") {
		memberRole = db.OrganizationMembersRole(role)
	}

	params := db.UpdateOrganizationMemberParams{
		Role:           memberRole,
		UpdatedBy:      sql.NullInt64{Valid: false},
		OrganizationID: organization.ID,
		AccountID:      account.ID,
	}

	err = s.db.UpdateOrganizationMember(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	member := &libopsv1.MemberDetail{
		AccountId:      accountID,
		Email:          account.Email,
		Name:           service.FromNullString(account.Name),
		Role:           role,
		GithubUsername: service.FromNullStringPtr(account.GithubUsername),
	}

	return connect.NewResponse(&libopsv1.UpdateOrganizationMemberResponse{
		Member: member,
	}), nil
}

// DeleteOrganizationMember deletes a member from a organization.
func (s *MemberService) DeleteOrganizationMember(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteOrganizationMemberRequest],
) (*connect.Response[emptypb.Empty], error) {
	organizationID := req.Msg.OrganizationId
	accountID := req.Msg.AccountId

	if err := validation.UUID(organizationID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.UUID(accountID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	organizationPublicID, err := uuid.Parse(organizationID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid organization_id format: %w", err))
	}

	accountPublicID, err := uuid.Parse(accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid account_id format: %w", err))
	}

	organization, err := s.db.GetOrganization(ctx, organizationPublicID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("organization not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	account, err := s.db.GetAccount(ctx, accountPublicID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("account not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Get member role before deletion for reconciliation check
	existingMember, err := s.db.GetOrganizationMember(ctx, db.GetOrganizationMemberParams{
		OrganizationID: organization.ID,
		AccountID:      account.ID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	params := db.DeleteOrganizationMemberParams{
		OrganizationID: organization.ID,
		AccountID:      account.ID,
	}

	err = s.db.DeleteOrganizationMember(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Trigger reconciliation via WebSocket if owner/developer role was removed
	memberRole := string(existingMember.Role)
	if s.connManager != nil && (memberRole == "owner" || memberRole == "developer") {
		// Get all projects in this organization
		projects, err := s.db.ListOrganizationProjects(ctx, db.ListOrganizationProjectsParams{
			OrganizationID: organization.ID,
			Limit:          1000,
			Offset:         0,
		})
		if err == nil {
			// Get all sites across all projects
			for _, project := range projects {
				sites, err := s.db.ListProjectSites(ctx, db.ListProjectSitesParams{
					ProjectID: project.ID,
					Limit:     1000,
					Offset:    0,
				})
				if err != nil {
					continue
				}

				// Trigger SSH key reconciliation for each connected site
				for _, site := range sites {
					if err := s.connManager.TriggerReconciliation(site.ID, "ssh_keys"); err == nil {
						slog.Info("triggered ssh_keys reconciliation for org member removal",
							"site_id", site.PublicID,
							"organization_id", organizationID)
					}
				}
			}
		}
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}
