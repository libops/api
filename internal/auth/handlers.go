package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/vault"
)

// Handler provides HTTP handlers for authentication.
type Handler struct {
	oidcClient     *OIDCClient
	userpassClient *UserpassClient
	validator      JWTValidator
	sessionManager *SessionManager
	db             db.Querier
	vaultClient    *vault.Client
	provider       string
}

// NewHandler creates a new auth handler.
func NewHandler(oidcClient *OIDCClient, userpassClient *UserpassClient, validator JWTValidator, sessionManager *SessionManager, querier db.Querier, vaultClient *vault.Client, provider string) *Handler {
	return &Handler{
		oidcClient:     oidcClient,
		userpassClient: userpassClient,
		validator:      validator,
		sessionManager: sessionManager,
		db:             querier,
		vaultClient:    vaultClient,
		provider:       provider,
	}
}

// HandleHome shows the login page.
func (h *Handler) HandleHome(w http.ResponseWriter, r *http.Request) {
	// Check if user is already authenticated
	userInfo, authenticated := GetUserFromContext(r.Context())

	if authenticated && userInfo != nil {
		// User is already authenticated
		redirectURI := r.URL.Query().Get("redirect_uri")
		state := r.URL.Query().Get("state")

		if redirectURI != "" {
			// CLI request - redirect to CLI callback
			redirectURL := fmt.Sprintf("%s?state=%s", redirectURI, state)
			http.Redirect(w, r, redirectURL, http.StatusFound)
			return
		}

		// Web request - redirect to dashboard
		http.Redirect(w, r, "/dashboard", http.StatusFound)
		return
	}

	// Not authenticated, show login page
	HandleLoginPage(w, r)
}

// HandleLogin initiates the OIDC login flow.
func (h *Handler) HandleLogin(w http.ResponseWriter, r *http.Request) {
	provider := r.URL.Query().Get("provider")

	redirectPath := r.URL.Query().Get("redirect")
	if redirectPath == "" {
		redirectPath = "/"
	}

	authURL, err := h.oidcClient.BuildAuthorizationURL(provider, redirectPath)
	if err != nil {
		slog.Error("Failed to build authorization URL", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleCallback handles the OIDC callback from Vault.
func (h *Handler) HandleCallback(w http.ResponseWriter, r *http.Request) {
	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		http.Error(w, "Missing code or state parameter", http.StatusBadRequest)
		return
	}

	tokenResp, err := h.oidcClient.ExchangeCode(r.Context(), code, state)
	if err != nil {
		slog.Error("Failed to exchange code", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	userInfo, err := h.validator.ValidateToken(r.Context(), tokenResp.IDToken)
	if err != nil {
		slog.Error("Failed to validate token", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	account, err := h.linkOrCreateAccount(r.Context(), userInfo)
	if err != nil {
		slog.Error("Failed to link account", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	h.sessionManager.SetSessionCookies(w, tokenResp.AccessToken, tokenResp.IDToken, tokenResp.ExpiresIn)

	// Check if this is a CLI callback (redirect path starts with /cli-callback)
	if strings.HasPrefix(tokenResp.StateData.RedirectPath, "/cli-callback") {
		// Extract redirect_uri and state from the redirect path
		redirectURL, err := url.Parse(tokenResp.StateData.RedirectPath)
		if err != nil {
			slog.Error("Failed to parse redirect path", "err", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		redirectURI := redirectURL.Query().Get("redirect_uri")
		state := redirectURL.Query().Get("state")

		if redirectURI != "" {
			// For CLI: redirect and rely on cookie extraction
			cliRedirectURL := fmt.Sprintf("%s?state=%s", redirectURI, state)
			http.Redirect(w, r, cliRedirectURL, http.StatusFound)
			return
		}
	}

	// Check if this is an API request (JSON Accept header)
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		name := ""
		if account.Name.Valid {
			name = account.Name.String
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"account": map[string]string{
				"id":    account.PublicID,
				"email": account.Email,
				"name":  name,
			},
			"redirect": tokenResp.StateData.RedirectPath,
		}); err != nil {
			slog.Error("Failed to encode response", "err", err)
		}
		return
	}

	// For browser requests without redirect (normal web login), redirect to dashboard
	if tokenResp.StateData.RedirectPath == "/" || tokenResp.StateData.RedirectPath == "" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	// For other cases, redirect to the specified path
	http.Redirect(w, r, tokenResp.StateData.RedirectPath, http.StatusSeeOther)
}

// HandleLogout logs out the user.
func (h *Handler) HandleLogout(w http.ResponseWriter, r *http.Request) {
	h.sessionManager.ClearSessionCookies(w)

	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]bool{"success": true}); err != nil {
			slog.Error("Failed to encode response", "err", err)
		}
	} else {
		http.Redirect(w, r, "/login?message=Logged out successfully", http.StatusSeeOther)
	}
}

// HandleMe returns current user info.
func (h *Handler) HandleMe(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Not authenticated", http.StatusUnauthorized)
		return
	}

	account, err := h.db.GetAccountByVaultEntityID(r.Context(), sql.NullString{String: userInfo.EntityID, Valid: true})
	if err != nil {
		slog.Error("Failed to get account", "err", err, "entity_id", userInfo.EntityID)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}
	githubUsername := ""
	if account.GithubUsername.Valid {
		githubUsername = account.GithubUsername.String
	}
	if err := json.NewEncoder(w).Encode(map[string]any{
		"id":              account.PublicID,
		"email":           account.Email,
		"name":            name,
		"github_username": githubUsername,
		"vault_entity_id": userInfo.EntityID,
	}); err != nil {
		slog.Error("Failed to encode response", "err", err)
	}
}

// linkOrCreateAccount links a Vault entity to a local account or creates a new one.
func (h *Handler) linkOrCreateAccount(ctx context.Context, userInfo *UserInfo) (*db.GetAccountByVaultEntityIDRow, error) {
	account, err := h.db.GetAccountByVaultEntityID(ctx, sql.NullString{String: userInfo.EntityID, Valid: true})
	if err == nil {
		return &account, nil
	}

	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query account: %w", err)
	}

	existingAccount, err := h.db.GetAccountByEmail(ctx, userInfo.Email)
	if err == nil {
		// Account exists with this email but different entity ID
		publicID, err := uuid.Parse(existingAccount.PublicID)
		if err != nil {
			return nil, fmt.Errorf("failed to parse public ID: %w", err)
		}

		err = h.db.UpdateAccount(ctx, db.UpdateAccountParams{
			Email:          existingAccount.Email,
			Name:           existingAccount.Name,
			GithubUsername: existingAccount.GithubUsername,
			VaultEntityID:  sql.NullString{String: userInfo.EntityID, Valid: true},
			AuthMethod:     existingAccount.AuthMethod,
			Verified:       existingAccount.Verified,
			VerifiedAt:     existingAccount.VerifiedAt,
			PublicID:       publicID.String(),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to link vault entity: %w", err)
		}

		account, err = h.db.GetAccountByVaultEntityID(ctx, sql.NullString{String: userInfo.EntityID, Valid: true})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch linked account: %w", err)
		}
		return &account, nil
	}

	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("failed to query account by email: %w", err)
	}

	now := sql.NullTime{Time: time.Now(), Valid: true}
	err = h.db.CreateAccount(ctx, db.CreateAccountParams{
		Email:          userInfo.Email,
		Name:           sql.NullString{String: userInfo.Name, Valid: userInfo.Name != ""},
		GithubUsername: sql.NullString{Valid: false},
		VaultEntityID:  sql.NullString{String: userInfo.EntityID, Valid: true},
		AuthMethod:     db.AccountsAuthMethodGoogle,
		Verified:       true,
		VerifiedAt:     now,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create account: %w", err)
	}

	account, err = h.db.GetAccountByVaultEntityID(ctx, sql.NullString{String: userInfo.EntityID, Valid: true})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch created account: %w", err)
	}

	return &account, nil
}

// HandleVerifyEmail delegates to userpassClient for email verification.
func (h *Handler) HandleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	if h.userpassClient == nil {
		http.Error(w, "Userpass authentication not configured", http.StatusInternalServerError)
		return
	}
	h.userpassClient.HandleVerifyEmail(w, r)
}

// HandleGoogleLogin initiates the OIDC login flow specifically for Google.
func (h *Handler) HandleGoogleLogin(w http.ResponseWriter, r *http.Request) {
	// Check for CLI redirect parameters
	redirectURI := r.URL.Query().Get("redirect_uri")
	cliState := r.URL.Query().Get("state")

	// If this is a CLI request, encode the redirect info in the redirect path
	// The callback will extract this and redirect to the CLI
	var redirectPath string
	if redirectURI != "" {
		// Encode CLI redirect info in the redirect path
		redirectPath = fmt.Sprintf("/cli-callback?redirect_uri=%s&state=%s", redirectURI, cliState)
	} else {
		// Standard web flow
		redirectPath = r.URL.Query().Get("redirect")
		if redirectPath == "" {
			redirectPath = "/"
		}
	}

	authURL, err := h.oidcClient.BuildAuthorizationURL("google", redirectPath)
	if err != nil {
		slog.Error("Failed to build authorization URL", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleUserpassLogin delegates to userpassClient for userpass login.
func (h *Handler) HandleUserpassLogin(w http.ResponseWriter, r *http.Request) {
	if h.userpassClient == nil {
		http.Error(w, "Userpass authentication not configured", http.StatusInternalServerError)
		return
	}

	// Check for CLI redirect parameters
	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form data", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")
	redirectURI := r.FormValue("redirect_uri")
	state := r.FormValue("state")

	if email == "" || password == "" {
		http.Error(w, "Email and password are required", http.StatusBadRequest)
		return
	}

	// Get OIDC token for userpass authentication
	oidcToken, expiresIn, err := h.getUserpassOIDCToken(r.Context(), email, password)
	if err != nil {
		http.Error(w, "Login failed. Check username/password.", http.StatusUnauthorized)
		return
	}

	// Set auth cookies
	h.sessionManager.SetSessionCookies(w, oidcToken, oidcToken, expiresIn)

	// If this is a CLI request, redirect to CLI callback
	if redirectURI != "" {
		redirectURL := fmt.Sprintf("%s?state=%s", redirectURI, state)
		http.Redirect(w, r, redirectURL, http.StatusFound)
		return
	}

	// Web request - redirect to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// getUserpassOIDCToken authenticates with userpass and returns an OIDC token.
func (h *Handler) getUserpassOIDCToken(ctx context.Context, email, password string) (string, int, error) {
	// Authenticate with userpass to get Vault token
	vaultTokenResp, err := h.userpassClient.Login(ctx, email, password)
	if err != nil {
		return "", 0, err
	}

	// Get account info
	account, err := h.db.GetAccountByEmail(ctx, email)
	if err != nil {
		slog.Error("Failed to get account", "err", err)
		return "", 0, fmt.Errorf("internal server error")
	}

	// Use the user's Vault token to request an OIDC token
	// This uses the authenticated user's token, not root privileges
	scopes := GetAccountScopesForOAuth()
	scopeStrings := ScopesToStrings(scopes)

	oidcToken, ttl, err := h.vaultClient.GetOIDCTokenWithAccountID(ctx, vaultTokenResp.VaultToken, h.provider, account.ID, scopeStrings)
	if err != nil {
		slog.Error("Failed to get OIDC token", "err", err)
		return "", 0, fmt.Errorf("failed to get OIDC token")
	}

	// Validate the OIDC token to extract the entity ID
	userInfo, err := h.validator.ValidateToken(ctx, oidcToken)
	if err != nil {
		slog.Error("Failed to validate OIDC token", "err", err)
		return "", 0, fmt.Errorf("failed to validate OIDC token")
	}

	// Link or create account with the Vault entity ID
	// This ensures the account is properly linked to the Vault entity
	_, err = h.linkOrCreateAccount(ctx, userInfo)
	if err != nil {
		slog.Error("Failed to link account with entity", "err", err)
		return "", 0, fmt.Errorf("failed to link account")
	}

	return oidcToken, ttl, nil
}

// HandleDashboard displays the user dashboard with their organizations.
func (h *Handler) HandleDashboard(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := GetUserFromContext(r.Context())
	if !ok {
		slog.Warn("Dashboard accessed without authentication")
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	slog.Info("Dashboard access", "entity_id", userInfo.EntityID, "email", userInfo.Email)

	// Get account info
	account, err := h.db.GetAccountByVaultEntityID(r.Context(), sql.NullString{String: userInfo.EntityID, Valid: true})
	if err != nil {
		if err == sql.ErrNoRows {
			slog.Error("Account not found for entity", "entity_id", userInfo.EntityID, "email", userInfo.Email)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		slog.Error("Failed to get account", "entity_id", userInfo.EntityID, "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Get organizations the user is a member of
	orgs, err := h.db.ListAccountOrganizations(r.Context(), db.ListAccountOrganizationsParams{
		AccountID: account.ID,
		Limit:     100,
		Offset:    0,
	})
	if err != nil {
		slog.Error("Failed to get organizations", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Build organization list
	var organizations []Organization
	for _, org := range orgs {
		organizations = append(organizations, Organization{
			ID:          org.PublicID,
			Name:        org.Name,
			Description: "", // Description not included in this query
			Role:        string(org.Role),
		})
	}

	// Render dashboard
	name := ""
	if account.Name.Valid {
		name = account.Name.String
	}

	RenderDashboardPage(w, DashboardPageData{
		Email:         account.Email,
		Name:          name,
		Organizations: organizations,
	})
}
