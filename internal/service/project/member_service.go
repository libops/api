package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"

	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/service"
	"github.com/libops/api/internal/validation"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// ProjectMemberService implements the LibOps ProjectMemberService API.
type ProjectMemberService struct {
	db db.Querier
}

// Compile-time check.
var _ libopsv1connect.ProjectMemberServiceHandler = (*ProjectMemberService)(nil)

// NewProjectMemberService creates a new ProjectMemberService instance.
func NewProjectMemberService(querier db.Querier) *ProjectMemberService {
	return &ProjectMemberService{
		db: querier,
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

	params := db.CreateProjectMemberParams{
		ProjectID: project.ID,
		AccountID: account.ID,
		Role:      db.ProjectMembersRole(req.Msg.Role),
	}

	err = s.db.CreateProjectMember(ctx, params)
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

	params := db.DeleteProjectMemberParams{
		ProjectID: project.ID,
		AccountID: account.ID,
	}

	err = s.db.DeleteProjectMember(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}
