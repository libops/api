package project

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

// ProjectMemberService implements the LibOps ProjectMemberService API.
type ProjectMemberService struct {
	db          db.Querier
	connManager *reconciler.ConnectionManager
}

// Compile-time check.
var _ libopsv1connect.ProjectMemberServiceHandler = (*ProjectMemberService)(nil)

// NewProjectMemberService creates a new ProjectMemberService instance.
func NewProjectMemberService(querier db.Querier, connManager *reconciler.ConnectionManager) *ProjectMemberService {
	return &ProjectMemberService{
		db:          querier,
		connManager: connManager,
	}
}

// ListProjectMembers lists members of a project.
func (s *ProjectMemberService) ListProjectMembers(
	ctx context.Context,
	req *connect.Request[libopsv1.ListProjectMembersRequest],
) (*connect.Response[libopsv1.ListProjectMembersResponse], error) {
	projectID := req.Msg.ProjectId

	if err := validation.UUID(projectID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	project, err := service.GetProjectByPublicID(ctx, s.db, projectID)
	if err != nil {
		return nil, err
	}

	params := db.ListProjectMembersParams{
		ProjectID: project.ID,
		Limit:     100, // Default limit
		Offset:    0,
	}

	members, err := s.db.ListProjectMembers(ctx, params)
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
			Status:         service.DbProjectMemberStatusToProto(member.Status),
		})
	}

	return connect.NewResponse(&libopsv1.ListProjectMembersResponse{
		Members:       protoMembers,
		NextPageToken: "",
	}), nil
}

// CreateProjectMember adds a member to a project.
func (s *ProjectMemberService) CreateProjectMember(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateProjectMemberRequest],
) (*connect.Response[libopsv1.CreateProjectMemberResponse], error) {
	projectID := req.Msg.ProjectId
	accountID := req.Msg.AccountId

	if err := validation.UUID(projectID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.UUID(accountID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.RequiredString("role", req.Msg.Role); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	project, err := service.GetProjectByPublicID(ctx, s.db, projectID)
	if err != nil {
		return nil, err
	}

	accountUUID, err := uuid.Parse(accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid account_id format: %w", err))
	}

	account, err := s.db.GetAccount(ctx, accountUUID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("account not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Determine initial status based on role
	// Owner/developer roles require reconciliation (SSH keys, secrets, firewall)
	status := db.ProjectMembersStatusActive
	if req.Msg.Role == "owner" || req.Msg.Role == "developer" {
		status = db.ProjectMembersStatusProvisioning
	}

	params := db.CreateProjectMemberParams{
		ProjectID: project.ID,
		AccountID: account.ID,
		Role:      db.ProjectMembersRole(req.Msg.Role),
		CreatedBy: sql.NullInt64{Valid: false},
		UpdatedBy: sql.NullInt64{Valid: false},
	}

	err = s.db.CreateProjectMember(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Trigger reconciliation via WebSocket if owner/developer role
	if s.connManager != nil && (req.Msg.Role == "owner" || req.Msg.Role == "developer") {
		// Get all sites in this project
		sites, err := s.db.ListProjectSites(ctx, db.ListProjectSitesParams{
			ProjectID: project.ID,
			Limit:     1000, // Max sites per project
			Offset:    0,
		})
		if err != nil {
			slog.Warn("failed to get project sites for reconciliation",
				"project_id", projectID,
				"error", err)
		} else {
			// Trigger SSH key reconciliation for each connected site
			for _, site := range sites {
				if err := s.connManager.TriggerReconciliation(site.ID, "ssh_keys"); err != nil {
					slog.Debug("site not connected, skipping reconciliation",
						"site_id", site.PublicID,
						"error", err)
				} else {
					slog.Info("triggered ssh_keys reconciliation for project member addition",
						"site_id", site.PublicID,
						"project_id", projectID)
				}
			}
		}
	}

	member := &libopsv1.MemberDetail{
		AccountId:      accountID,
		Email:          account.Email,
		Name:           service.FromNullString(account.Name),
		Role:           req.Msg.Role,
		Status:         service.DbStatusToProto(string(status)),
		GithubUsername: service.FromNullStringPtr(account.GithubUsername),
	}

	return connect.NewResponse(&libopsv1.CreateProjectMemberResponse{
		Member: member,
	}), nil
}

// UpdateProjectMember updates a member's role.
func (s *ProjectMemberService) UpdateProjectMember(
	ctx context.Context,
	req *connect.Request[libopsv1.UpdateProjectMemberRequest],
) (*connect.Response[libopsv1.UpdateProjectMemberResponse], error) {
	projectID := req.Msg.ProjectId
	accountID := req.Msg.AccountId

	if err := validation.UUID(projectID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.UUID(accountID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.RequiredString("role", req.Msg.Role); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	project, err := service.GetProjectByPublicID(ctx, s.db, projectID)
	if err != nil {
		return nil, err
	}

	accountUUID, err := uuid.Parse(accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid account_id format: %w", err))
	}

	account, err := s.db.GetAccount(ctx, accountUUID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("account not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	params := db.UpdateProjectMemberParams{
		ProjectID: project.ID,
		AccountID: account.ID,
		Role:      db.ProjectMembersRole(req.Msg.Role),
	}

	err = s.db.UpdateProjectMember(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	member := &libopsv1.MemberDetail{
		AccountId:      accountID,
		Email:          account.Email,
		Name:           service.FromNullString(account.Name),
		Role:           req.Msg.Role,
		GithubUsername: service.FromNullStringPtr(account.GithubUsername),
	}

	return connect.NewResponse(&libopsv1.UpdateProjectMemberResponse{
		Member: member,
	}), nil
}

// DeleteProjectMember removes a member from a project.
func (s *ProjectMemberService) DeleteProjectMember(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteProjectMemberRequest],
) (*connect.Response[emptypb.Empty], error) {
	projectID := req.Msg.ProjectId
	accountID := req.Msg.AccountId

	if err := validation.UUID(projectID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.UUID(accountID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	project, err := service.GetProjectByPublicID(ctx, s.db, projectID)
	if err != nil {
		return nil, err
	}

	accountUUID, err := uuid.Parse(accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid account_id format: %w", err))
	}

	account, err := s.db.GetAccount(ctx, accountUUID.String())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("account not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Get member role before deletion for reconciliation check
	existingMember, err := s.db.GetProjectMember(ctx, db.GetProjectMemberParams{
		ProjectID: project.ID,
		AccountID: account.ID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	params := db.DeleteProjectMemberParams{
		ProjectID: project.ID,
		AccountID: account.ID,
	}

	err = s.db.DeleteProjectMember(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Trigger reconciliation via WebSocket if owner/developer role was removed
	memberRole := string(existingMember.Role)
	if s.connManager != nil && (memberRole == "owner" || memberRole == "developer") {
		// Get all sites in this project
		sites, err := s.db.ListProjectSites(ctx, db.ListProjectSitesParams{
			ProjectID: project.ID,
			Limit:     1000,
			Offset:    0,
		})
		if err == nil {
			// Trigger SSH key reconciliation for each connected site
			for _, site := range sites {
				if err := s.connManager.TriggerReconciliation(site.ID, "ssh_keys"); err == nil {
					slog.Info("triggered ssh_keys reconciliation for project member removal",
						"site_id", site.PublicID,
						"project_id", projectID)
				}
			}
		}
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}
