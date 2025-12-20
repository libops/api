package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"connectrpc.com/connect"

	"github.com/libops/api/db"
	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/validation"
	libopsv1 "github.com/libops/api/proto/libops/v1"
	"github.com/libops/api/proto/libops/v1/libopsv1connect"
)

// AccountService implements the organization-facing account service.
type AccountService struct {
	repo          *Repository
	apiKeyManager *auth.APIKeyManager
}

// Compile-time check.
var _ libopsv1connect.AccountServiceHandler = (*AccountService)(nil)

// NewAccountService creates a new organization account service.
func NewAccountService(querier db.Querier, apiKeyManager *auth.APIKeyManager) *AccountService {
	return &AccountService{
		repo:          NewRepository(querier),
		apiKeyManager: apiKeyManager,
	}
}

// GetAccountByEmail retrieves limited account information by email (for Terraform provider)
// Only accessible to authenticated organizations with valid organization memberships.
func (s *AccountService) GetAccountByEmail(
	ctx context.Context,
	req *connect.Request[libopsv1.GetAccountByEmailRequest],
) (*connect.Response[libopsv1.GetAccountByEmailResponse], error) {
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok || userInfo == nil {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	// This ensures only organizations (not anonymous users) can look up accounts
	organizations, err := s.repo.db.ListAccountOrganizations(ctx, db.ListAccountOrganizationsParams{
		AccountID: userInfo.AccountID,
		Limit:     1,
		Offset:    0,
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to verify organization membership: %w", err))
	}
	if len(organizations) == 0 {
		return nil, connect.NewError(connect.CodePermissionDenied,
			fmt.Errorf("account lookup requires organization membership"))
	}

	email := req.Msg.Email
	if err := validation.Email(email); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	account, err := s.repo.GetAccountByEmail(ctx, email)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(
				connect.CodeNotFound,
				fmt.Errorf("account with email '%s' not found", email),
			)
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("database error: %w", err))
	}

	organizationAccount := &libopsv1.OrganizationAccount{
		AccountId: account.PublicID,
		Email:     account.Email,
		Name:      fromNullString(account.Name),
	}

	return connect.NewResponse(&libopsv1.GetAccountByEmailResponse{
		Account: organizationAccount,
	}), nil
}

// CreateApiKey creates a new API key for the authenticated user.
func (s *AccountService) CreateApiKey(
	ctx context.Context,
	req *connect.Request[libopsv1.CreateApiKeyRequest],
) (*connect.Response[libopsv1.CreateApiKeyResponse], error) {
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	// Security: Always create for authenticated user, never from request
	accountID := userInfo.AccountID

	// Validate name
	if req.Msg.Name == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name is required"))
	}
	if len(req.Msg.Name) > 255 {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("name must be 255 characters or less"))
	}

	// Validate scopes if provided
	if len(req.Msg.Scopes) > 0 {
		_, err := auth.ParseScopes(req.Msg.Scopes)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid scope: %w", err))
		}
	}

	// Get account UUID from database
	account, err := s.repo.db.GetAccountByID(ctx, accountID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get account: %w", err))
	}

	// Use APIKeyManager to create the key (handles both database and Vault storage)
	apiKey, keyMeta, err := s.apiKeyManager.CreateAPIKey(
		ctx,
		accountID,
		account.PublicID,
		req.Msg.Name,
		req.Msg.Description,
		req.Msg.Scopes,
		nil, // expiresAt
		accountID,
	)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to create API key: %w", err))
	}

	createdAt := int64(0)
	if keyMeta.CreatedAt.Valid {
		createdAt = keyMeta.CreatedAt.Time.Unix()
	}

	return connect.NewResponse(&libopsv1.CreateApiKeyResponse{
		ApiKeyId:    keyMeta.PublicID,
		ApiKey:      apiKey,
		Name:        req.Msg.Name,
		Description: req.Msg.Description,
		Scopes:      req.Msg.Scopes,
		CreatedAt:   createdAt,
	}), nil
}

// ListApiKeys lists all API keys for the authenticated user.
func (s *AccountService) ListApiKeys(
	ctx context.Context,
	req *connect.Request[libopsv1.ListApiKeysRequest],
) (*connect.Response[libopsv1.ListApiKeysResponse], error) {
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	// Security: Always list for authenticated user
	accountID := userInfo.AccountID

	// Parse pagination
	pageSize := req.Msg.PageSize
	if pageSize <= 0 {
		pageSize = 50
	}
	if pageSize > 100 {
		pageSize = 100
	}

	offset := int32(0)
	if req.Msg.PageToken != "" {
		parsedOffset, err := parsePageToken(req.Msg.PageToken)
		if err != nil {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("invalid page token: %w", err))
		}
		offset = int32(parsedOffset)
	}

	keys, err := s.repo.ListAPIKeysByAccount(ctx, accountID, pageSize, offset)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to list API keys: %w", err))
	}

	apiKeys := make([]*libopsv1.ApiKeyMetadata, len(keys))
	for i, key := range keys {
		createdAt := int64(0)
		if key.CreatedAt.Valid {
			createdAt = key.CreatedAt.Time.Unix()
		}

		lastUsedAt := int64(0)
		if key.LastUsedAt.Valid {
			lastUsedAt = key.LastUsedAt.Time.Unix()
		}

		apiKeys[i] = &libopsv1.ApiKeyMetadata{
			ApiKeyId:    key.PublicID,
			Name:        key.Name,
			Description: key.Description.String,
			Scopes:      unmarshalScopes(key.Scopes),
			Active:      key.Active,
			CreatedAt:   createdAt,
			LastUsedAt:  lastUsedAt,
		}
	}

	nextPageToken := ""
	if len(keys) == int(pageSize) {
		nextPageToken = generatePageToken(int(offset) + len(keys))
	}

	return connect.NewResponse(&libopsv1.ListApiKeysResponse{
		ApiKeys:       apiKeys,
		NextPageToken: nextPageToken,
	}), nil
}

// RevokeApiKey revokes an API key for the authenticated user.
func (s *AccountService) RevokeApiKey(
	ctx context.Context,
	req *connect.Request[libopsv1.RevokeApiKeyRequest],
) (*connect.Response[libopsv1.RevokeApiKeyResponse], error) {
	userInfo, ok := auth.GetUserFromContext(ctx)
	if !ok {
		return nil, connect.NewError(connect.CodeUnauthenticated, fmt.Errorf("authentication required"))
	}

	accountID := userInfo.AccountID

	if err := validation.UUID(req.Msg.ApiKeyId); err != nil {
		return nil, connect.NewError(connect.CodeInvalidArgument, err)
	}

	// First verify the key exists and belongs to the user
	key, err := s.repo.GetAPIKeyByUUID(ctx, req.Msg.ApiKeyId)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// Return 404 instead of 403 to avoid information leakage
			return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("API key not found"))
		}
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to get API key: %w", err))
	}

	// Security: Verify key belongs to authenticated user
	if key.AccountID != accountID {
		// Return 404 instead of 403 to avoid information leakage
		return nil, connect.NewError(connect.CodeNotFound, fmt.Errorf("API key not found"))
	}

	// Revoke the key (set active=false and delete from Vault)
	err = s.apiKeyManager.DeactivateAPIKey(ctx, req.Msg.ApiKeyId)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("failed to revoke API key: %w", err))
	}

	return connect.NewResponse(&libopsv1.RevokeApiKeyResponse{
		Success: true,
	}), nil
}
