package site

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

// SiteMemberService implements the LibOps SiteMemberService API.
type SiteMemberService struct {
	db          db.Querier
	connManager *reconciler.ConnectionManager
}

// Compile-time check.
var _ libopsv1connect.SiteMemberServiceHandler = (*SiteMemberService)(nil)

// NewSiteMemberService creates a new SiteMemberService instance.
func NewSiteMemberService(querier db.Querier, connManager *reconciler.ConnectionManager) *SiteMemberService {
	return &SiteMemberService{
		db:          querier,
		connManager: connManager,
	}
}

// ListSiteMembers lists members of a site.
func (s *SiteMemberService) ListSiteMembers(
	ctx context.Context,
	req *connect.Request[libopsv1.ListSiteMembersRequest],
) (*connect.Response[libopsv1.ListSiteMembersResponse], error) {
	siteID := req.Msg.SiteId
	if err := validation.UUID(siteID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	site, err := service.GetSiteByPublicID(ctx, s.db, siteID)
	if err != nil {
		return nil, err
	}

	params := db.ListSiteMembersParams{
		SiteID: site.ID,
		Limit:  100,
		Offset: 0,
	}

	members, err := s.db.ListSiteMembers(ctx, params)
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
			Status:         service.DbSiteMemberStatusToProto(member.Status),
		})
	}

	return connect.NewResponse(&libopsv1.ListSiteMembersResponse{
		Members:       protoMembers,
		NextPageToken: "",
	}), nil
}

// CreateSiteMember adds a member to a site.
func (s *SiteMemberService) CreateSiteMember(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateSiteMemberRequest],
) (*connect.Response[libopsv1.CreateSiteMemberResponse], error) {
	siteID := req.Msg.SiteId
	accountID := req.Msg.AccountId

	if err := validation.UUID(siteID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.UUID(accountID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.RequiredString("role", req.Msg.Role); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	site, err := service.GetSiteByPublicID(ctx, s.db, siteID)
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
	status := db.SiteMembersStatusActive
	if req.Msg.Role == "owner" || req.Msg.Role == "developer" {
		status = db.SiteMembersStatusProvisioning
	}

	params := db.CreateSiteMemberParams{
		SiteID:    site.ID,
		AccountID: account.ID,
		Role:      db.SiteMembersRole(req.Msg.Role),
		CreatedBy: sql.NullInt64{Valid: false},
		UpdatedBy: sql.NullInt64{Valid: false},
	}

	err = s.db.CreateSiteMember(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Trigger reconciliation via WebSocket if owner/developer role
	if s.connManager != nil && (req.Msg.Role == "owner" || req.Msg.Role == "developer") {
		if err := s.connManager.TriggerReconciliation(site.ID, "ssh_keys"); err != nil {
			slog.Debug("site not connected, skipping reconciliation",
				"site_id", siteID,
				"error", err)
		} else {
			slog.Info("triggered ssh_keys reconciliation for site member addition",
				"site_id", siteID)
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

	return connect.NewResponse(&libopsv1.CreateSiteMemberResponse{
		Member: member,
	}), nil
}

// UpdateSiteMember updates a member's role.
func (s *SiteMemberService) UpdateSiteMember(
	ctx context.Context,
	req *connect.Request[libopsv1.UpdateSiteMemberRequest],
) (*connect.Response[libopsv1.UpdateSiteMemberResponse], error) {
	siteID := req.Msg.SiteId
	accountID := req.Msg.AccountId

	if err := validation.UUID(siteID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.UUID(accountID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.RequiredString("role", req.Msg.Role); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	site, err := service.GetSiteByPublicID(ctx, s.db, siteID)
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

	params := db.UpdateSiteMemberParams{
		SiteID:    site.ID,
		AccountID: account.ID,
		Role:      db.SiteMembersRole(req.Msg.Role),
	}

	err = s.db.UpdateSiteMember(ctx, params)
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

	return connect.NewResponse(&libopsv1.UpdateSiteMemberResponse{
		Member: member,
	}), nil
}

// DeleteSiteMember removes a member from a site.
func (s *SiteMemberService) DeleteSiteMember(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteSiteMemberRequest],
) (*connect.Response[emptypb.Empty], error) {
	siteID := req.Msg.SiteId
	accountID := req.Msg.AccountId

	if err := validation.UUID(siteID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if err := validation.UUID(accountID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	site, err := service.GetSiteByPublicID(ctx, s.db, siteID)
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
	existingMember, err := s.db.GetSiteMember(ctx, db.GetSiteMemberParams{
		SiteID:    site.ID,
		AccountID: account.ID,
	})
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("member not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	params := db.DeleteSiteMemberParams{
		SiteID:    site.ID,
		AccountID: account.ID,
	}

	err = s.db.DeleteSiteMember(ctx, params)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	// Trigger reconciliation via WebSocket if owner/developer role was removed
	memberRole := string(existingMember.Role)
	if s.connManager != nil && (memberRole == "owner" || memberRole == "developer") {
		if err := s.connManager.TriggerReconciliation(site.ID, "ssh_keys"); err == nil {
			slog.Info("triggered ssh_keys reconciliation for site member removal",
				"site_id", siteID)
		}
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}
