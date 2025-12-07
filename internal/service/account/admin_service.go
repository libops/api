// Package account provides shared business logic for account operations.
package account

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"connectrpc.com/connect"
	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/fieldmaskpb"

	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/events"
	"github.com/libops/api/internal/service"
	"github.com/libops/api/internal/validation"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// AdminAccountService implements the admin account service with full access.
type AdminAccountService struct {
	repo    *Repository
	emitter *events.Emitter
}

// Compile-time check.
var _ libopsv1connect.AdminAccountServiceHandler = (*AdminAccountService)(nil)

// NewAdminAccountService creates a new admin account service.
func NewAdminAccountService(querier db.Querier, emitter *events.Emitter) *AdminAccountService {
	return &AdminAccountService{
		repo:    NewRepository(querier),
		emitter: emitter,
	}
}

// GetAccount retrieves full account information by public ID.
func (s *AdminAccountService) GetAccount(
	ctx context.Context,
	req *connect.Request[libopsv1.GetAccountRequest],
) (*connect.Response[libopsv1.GetAccountResponse], error) {
	accountID := req.Msg.AccountId

	if err := validation.UUID(accountID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	publicID, err := uuid.Parse(accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid account_id format: %w", err))
	}

	account, err := s.repo.GetAccountByPublicID(ctx, publicID)
	if err != nil {
		return nil, service.HandleDatabaseError(err, "account")
	}

	protoAccount := &libopsv1.Account{
		AccountId:      account.PublicID,
		Email:          account.Email,
		Name:           fromNullString(account.Name),
		GithubUsername: fromNullStringPtr(account.GithubUsername),
	}

	return connect.NewResponse(&libopsv1.GetAccountResponse{
		Account: protoAccount,
	}), nil
}

// GetAccountByEmail retrieves full account information by email.
func (s *AdminAccountService) GetAccountByEmail(
	ctx context.Context,
	req *connect.Request[libopsv1.AdminGetAccountByEmailRequest],
) (*connect.Response[libopsv1.AdminGetAccountByEmailResponse], error) {
	email := req.Msg.Email

	if err := validation.Email(email); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	account, err := s.repo.GetAccountByEmail(ctx, email)
	if err != nil {
		return nil, service.HandleDatabaseError(err, "account")
	}

	protoAccount := &libopsv1.Account{
		AccountId:      account.PublicID,
		Email:          account.Email,
		Name:           fromNullString(account.Name),
		GithubUsername: fromNullStringPtr(account.GithubUsername),
	}

	return connect.NewResponse(&libopsv1.AdminGetAccountByEmailResponse{
		Account: protoAccount,
	}), nil
}

// CreateAccount creates a new account.
func (s *AdminAccountService) CreateAccount(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateAccountRequest],
) (*connect.Response[libopsv1.CreateAccountResponse], error) {
	if err := validation.Email(req.Msg.Email); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	if req.Msg.Name != "" {
		if err := validation.StringLength("name", req.Msg.Name, 1, 255); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	if req.Msg.GithubUsername != nil && *req.Msg.GithubUsername != "" {
		if err := validation.StringLength("github_username", *req.Msg.GithubUsername, 1, 39); err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
	}

	params := db.CreateAccountParams{
		Email:          req.Msg.Email,
		Name:           sql.NullString{String: req.Msg.Name, Valid: req.Msg.Name != ""},
		GithubUsername: sql.NullString{String: stringValue(req.Msg.GithubUsername, ""), Valid: req.Msg.GithubUsername != nil},
	}

	err := s.repo.CreateAccount(ctx, params)
	if err != nil {
		return nil, service.HandleDatabaseError(err, "account")
	}

	account, err := s.repo.GetAccountByEmail(ctx, req.Msg.Email)
	if err != nil {
		return nil, service.InternalError()
	}

	protoAccount := &libopsv1.Account{
		AccountId:      account.PublicID,
		Email:          account.Email,
		Name:           fromNullString(account.Name),
		GithubUsername: fromNullStringPtr(account.GithubUsername),
	}

	if s.emitter != nil {
		if err := s.emitter.SendProtoEventWithSubject(
			ctx,
			events.EventTypeAccountCreated,
			account.PublicID,
			protoAccount,
		); err != nil {
			slog.Error("failed to emit account created event", "error", err, "account_id", account.PublicID)
		}
	}

	return connect.NewResponse(&libopsv1.CreateAccountResponse{
		Account: protoAccount,
	}), nil
}

// UpdateAccount updates an existing account.
func (s *AdminAccountService) UpdateAccount(
	ctx context.Context,
	req *connect.Request[libopsv1.UpdateAccountRequest],
) (*connect.Response[libopsv1.UpdateAccountResponse], error) {
	accountID := req.Msg.AccountId

	if err := validation.UUID(accountID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	publicID, err := uuid.Parse(accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid account_id format: %w", err))
	}

	existing, err := s.repo.GetAccountByPublicID(ctx, publicID)
	if err != nil {
		return nil, service.HandleDatabaseError(err, "account")
	}

	name := existing.Name
	githubUsername := existing.GithubUsername

	if shouldUpdateField(req.Msg.UpdateMask, "name") && req.Msg.Name != nil {
		if *req.Msg.Name != "" {
			if err := validation.StringLength("name", *req.Msg.Name, 1, 255); err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, err)
			}
		}
		name = sql.NullString{String: *req.Msg.Name, Valid: true}
	}

	if shouldUpdateField(req.Msg.UpdateMask, "github_username") && req.Msg.GithubUsername != nil {
		if *req.Msg.GithubUsername != "" {
			if err := validation.StringLength("github_username", *req.Msg.GithubUsername, 1, 39); err != nil {
				return nil, connect.NewError(connect.CodeInvalidArgument, err)
			}
		}
		githubUsername = sql.NullString{String: *req.Msg.GithubUsername, Valid: true}
	}

	params := db.UpdateAccountParams{
		Name:           name,
		GithubUsername: githubUsername,
		PublicID:       publicID.String(),
	}

	err = s.repo.UpdateAccount(ctx, params)
	if err != nil {
		return nil, service.HandleDatabaseError(err, "account")
	}

	account, err := s.repo.GetAccountByPublicID(ctx, publicID)
	if err != nil {
		return nil, service.InternalError()
	}

	protoAccount := &libopsv1.Account{
		AccountId:      account.PublicID,
		Email:          account.Email,
		Name:           fromNullString(account.Name),
		GithubUsername: fromNullStringPtr(account.GithubUsername),
	}

	if s.emitter != nil {
		if err := s.emitter.SendProtoEventWithSubject(
			ctx,
			events.EventTypeAccountUpdated,
			account.PublicID,
			protoAccount,
		); err != nil {
			slog.Error("failed to emit account updated event", "error", err, "account_id", account.PublicID)
		}
	}

	return connect.NewResponse(&libopsv1.UpdateAccountResponse{
		Account: protoAccount,
	}), nil
}

// DeleteAccount deletes an account.
func (s *AdminAccountService) DeleteAccount(
	ctx context.Context,
	req *connect.Request[libopsv1.DeleteAccountRequest],
) (*connect.Response[emptypb.Empty], error) {
	accountID := req.Msg.AccountId

	if err := validation.UUID(accountID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	publicID, err := uuid.Parse(accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid account_id format: %w", err))
	}

	err = s.repo.DeleteAccount(ctx, publicID)
	if err != nil {
		return nil, service.HandleDatabaseError(err, "account")
	}

	if s.emitter != nil {
		deleteEvent := &libopsv1.DeleteAccountRequest{
			AccountId: accountID,
		}
		if err := s.emitter.SendProtoEventWithSubject(
			ctx,
			events.EventTypeAccountDeleted,
			accountID,
			deleteEvent,
		); err != nil {
			slog.Error("failed to emit account deleted event", "error", err, "account_id", accountID)
		}
	}

	return connect.NewResponse(&emptypb.Empty{}), nil
}

// ListAccounts lists all accounts with pagination.
func (s *AdminAccountService) ListAccounts(
	ctx context.Context,
	req *connect.Request[libopsv1.ListAccountsRequest],
) (*connect.Response[libopsv1.ListAccountsResponse], error) {
	pageSize := req.Msg.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset, err := parsePageToken(req.Msg.PageToken)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	params := db.ListAccountsParams{
		Limit:  pageSize,
		Offset: int32(offset),
	}

	accounts, err := s.repo.ListAccounts(ctx, params)
	if err != nil {
		return nil, service.HandleDatabaseError(err, "account")
	}

	protoAccounts := make([]*libopsv1.Account, 0, len(accounts))
	for _, account := range accounts {
		protoAccounts = append(protoAccounts, &libopsv1.Account{
			AccountId:      account.PublicID,
			Email:          account.Email,
			Name:           fromNullString(account.Name),
			GithubUsername: fromNullStringPtr(account.GithubUsername),
		})
	}

	nextPageToken := ""
	if len(accounts) == int(pageSize) {
		nextPageToken = generatePageToken(offset + int(pageSize))
	}

	return connect.NewResponse(&libopsv1.ListAccountsResponse{
		Accounts:      protoAccounts,
		NextPageToken: nextPageToken,
	}), nil
}

// ListAccountProjects lists projects accessible to an account.
func (s *AdminAccountService) ListAccountProjects(
	ctx context.Context,
	req *connect.Request[libopsv1.ListAccountProjectsRequest],
) (*connect.Response[libopsv1.ListAccountProjectsResponse], error) {
	accountID := req.Msg.AccountId

	if err := validation.UUID(accountID); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	publicID, err := uuid.Parse(accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid account_id format: %w", err))
	}

	account, err := s.repo.GetAccountByPublicID(ctx, publicID)
	if err != nil {
		return nil, service.HandleDatabaseError(err, "account")
	}

	pageSize := req.Msg.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset, err := parsePageToken(req.Msg.PageToken)
	if err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page_token: %w", err))
	}

	params := db.ListAccountOrganizationsParams{
		AccountID: account.ID,
		Limit:     pageSize,
		Offset:    int32(offset),
	}

	organizations, err := s.repo.ListAccountOrganizations(ctx, params)
	if err != nil {
		return nil, service.HandleDatabaseError(err, "account")
	}

	organizationIDs := make([]string, 0, len(organizations))
	for _, organization := range organizations {
		organizationIDs = append(organizationIDs, organization.PublicID)
	}

	nextPageToken := ""
	if len(organizations) == int(pageSize) {
		nextPageToken = generatePageToken(offset + int(pageSize))
	}

	return connect.NewResponse(&libopsv1.ListAccountProjectsResponse{
		OrganizationIds: organizationIDs,
		NextPageToken:   nextPageToken,
	}), nil
}

// ListAccountRepositories lists repositories accessible to an account.
func (s *AdminAccountService) ListAccountRepositories(
	ctx context.Context,
	req *connect.Request[libopsv1.ListAccountRepositoriesRequest],
) (*connect.Response[libopsv1.ListAccountRepositoriesResponse], error) {
	// TODO: Implement when we have repository tracking
	return connect.NewResponse(&libopsv1.ListAccountRepositoriesResponse{
		Repositories:  []*libopsv1.Repository{},
		NextPageToken: "",
	}), nil
}

// Helper functions

// shouldUpdateField checks if a field should be updated based on the field mask.
// If the field mask is nil or empty, all fields should be updated; otherwise, only fields present in the mask should be updated.
func shouldUpdateField(mask *fieldmaskpb.FieldMask, field string) bool {
	if mask == nil || len(mask.Paths) == 0 {
		return true
	}
	for _, path := range mask.Paths {
		if path == field {
			return true
		}
	}
	return false
}

// parsePageToken decodes a page token string to an offset integer.
// Returns 0 for empty tokens (first page).
func parsePageToken(token string) (int, error) {
	if token == "" {
		return 0, nil
	}
	var offset int
	_, err := fmt.Sscanf(token, "%d", &offset)
	return offset, err
}

// generatePageToken encodes an offset integer to a page token string.
func generatePageToken(offset int) string {
	if offset <= 0 {
		return ""
	}
	return fmt.Sprintf("%d", offset)
}
