package auth

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/url"
	"time"

	"github.com/markbates/goth"
	"github.com/markbates/goth/providers/github"
	"github.com/markbates/goth/providers/google"

	"github.com/libops/api/db"
)

// ProviderConfig holds configuration for an OAuth provider.
type ProviderConfig struct {
	ClientID     string
	ClientSecret string
	CallbackURL  string
}

// GothOAuthManager manages OAuth authentication using the Goth library.
type GothOAuthManager struct {
	stateManager *OAuthStateManager
	db           db.Querier
	providers    map[string]goth.Provider
}

// NewGothOAuthManager creates a new Goth OAuth manager.
func NewGothOAuthManager(googleCfg, githubCfg *ProviderConfig, querier db.Querier) (*GothOAuthManager, error) {
	manager := &GothOAuthManager{
		stateManager: NewOAuthStateManager(),
		db:           querier,
		providers:    make(map[string]goth.Provider),
	}

	// Initialize Google provider
	if googleCfg != nil && googleCfg.ClientID != "" {
		googleProvider := google.New(
			googleCfg.ClientID,
			googleCfg.ClientSecret,
			googleCfg.CallbackURL,
			"email", "profile",
		)
		manager.providers["google"] = googleProvider
		slog.Info("Google OAuth provider initialized")
	}

	// Initialize GitHub provider
	if githubCfg != nil && githubCfg.ClientID != "" {
		githubProvider := github.New(
			githubCfg.ClientID,
			githubCfg.ClientSecret,
			githubCfg.CallbackURL,
			"user:email",
		)
		manager.providers["github"] = githubProvider
		slog.Info("GitHub OAuth provider initialized")
	}

	if len(manager.providers) == 0 {
		return nil, fmt.Errorf("no OAuth providers configured")
	}

	return manager, nil
}

// BeginAuth starts the OAuth flow for a provider.
// Returns the authorization URL to redirect the user to.
func (m *GothOAuthManager) BeginAuth(provider, redirectPath string) (string, error) {
	gothProvider, exists := m.providers[provider]
	if !exists {
		return "", fmt.Errorf("unsupported OAuth provider: %s", provider)
	}

	// Create state for CSRF protection
	stateData, err := m.stateManager.CreateState(redirectPath)
	if err != nil {
		return "", fmt.Errorf("failed to create state: %w", err)
	}

	// Create a Goth session
	sess, err := gothProvider.BeginAuth(stateData.State)
	if err != nil {
		return "", fmt.Errorf("failed to begin auth: %w", err)
	}

	authURL, err := sess.GetAuthURL()
	if err != nil {
		return "", fmt.Errorf("failed to get auth URL: %w", err)
	}

	return authURL, nil
}

// CompleteAuth completes the OAuth flow and returns user information.
func (m *GothOAuthManager) CompleteAuth(ctx context.Context, provider, code, state string) (*goth.User, *StateData, error) {
	gothProvider, exists := m.providers[provider]
	if !exists {
		return nil, nil, fmt.Errorf("unsupported OAuth provider: %s", provider)
	}

	// Validate state
	stateData, err := m.stateManager.ValidateAndConsumeState(state)
	if err != nil {
		return nil, nil, fmt.Errorf("invalid state: %w", err)
	}

	// Create a new session and fetch user data
	sess, err := gothProvider.BeginAuth(state)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create session: %w", err)
	}

	// Authorize the session with the code
	params := url.Values{}
	params.Set("code", code)
	_, err = sess.Authorize(gothProvider, params)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to authorize: %w", err)
	}

	// Fetch user information
	user, err := gothProvider.FetchUser(sess)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to fetch user: %w", err)
	}

	return &user, stateData, nil
}

// LinkOrCreateAccount links OAuth user to existing account or creates new one.
func (m *GothOAuthManager) LinkOrCreateAccount(ctx context.Context, email, name, provider string) (*db.GetAccountByEmailRow, error) {
	// Check if account exists by email
	account, err := m.db.GetAccountByEmail(ctx, email)

	if err == sql.ErrNoRows {
		// Create new account
		return m.createOAuthAccount(ctx, email, name, provider)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to query account: %w", err)
	}

	// Account exists - check if verified
	if !account.Verified {
		return nil, fmt.Errorf("please verify your email address before linking OAuth account")
	}

	// Account exists and is verified - allow merge/link
	slog.Info("linking OAuth to existing account", "email", email, "provider", provider, "account_id", account.ID)
	return &account, nil
}

// createOAuthAccount creates a new account for OAuth user.
func (m *GothOAuthManager) createOAuthAccount(ctx context.Context, email, name, provider string) (*db.GetAccountByEmailRow, error) {
	now := sql.NullTime{Time: time.Now(), Valid: true}

	// Convert provider string to AuthMethod
	var authMethod db.AccountsAuthMethod
	switch provider {
	case "google":
		authMethod = db.AccountsAuthMethodGoogle
	case "github":
		authMethod = db.AccountsAuthMethodGithub
	default:
		authMethod = db.AccountsAuthMethodGoogle // Fallback
	}

	err := m.db.CreateAccount(ctx, db.CreateAccountParams{
		Email:          email,
		Name:           sql.NullString{String: name, Valid: name != ""},
		GithubUsername: sql.NullString{Valid: false},
		VaultEntityID:  sql.NullString{Valid: false}, // Will be populated by EnsureVaultEntity
		AuthMethod:     authMethod,
		Verified:       true, // OAuth providers verify email
		VerifiedAt:     now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	// Retrieve the created account
	account, err := m.db.GetAccountByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("failed to retrieve created account: %w", err)
	}

	slog.Info("created new OAuth account", "email", email, "provider", provider, "account_id", account.ID)
	return &account, nil
}
