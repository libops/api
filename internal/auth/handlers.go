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

	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/vault"
)

// Handler provides HTTP handlers for authentication.
type Handler struct {
	userpassClient *UserpassClient
	validator      JWTValidator
	sessionManager *SessionManager
	db             db.Querier
	vaultClient    *vault.Client
	provider       string
	gothManager    *GothOAuthManager
	tokenIssuer    *LibopsTokenIssuer
}

// NewHandler creates a new auth handler.
func NewHandler(userpassClient *UserpassClient, validator JWTValidator, sessionManager *SessionManager, querier db.Querier, vaultClient *vault.Client, provider string, gothManager *GothOAuthManager, tokenIssuer *LibopsTokenIssuer) *Handler {
	return &Handler{
		userpassClient: userpassClient,
		validator:      validator,
		sessionManager: sessionManager,
		db:             querier,
		vaultClient:    vaultClient,
		provider:       provider,
		gothManager:    gothManager,
		tokenIssuer:    tokenIssuer,
	}
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

// HandleVerifyEmail delegates to userpassClient for email verification.
func (h *Handler) HandleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	if h.userpassClient == nil {
		http.Error(w, "Userpass authentication not configured", http.StatusInternalServerError)
		return
	}
	h.userpassClient.HandleVerifyEmail(w, r)
}

// HandleUserpassLogin delegates to userpassClient for userpass login.
func (h *Handler) HandleUserpassLogin(w http.ResponseWriter, r *http.Request) {
	if h.userpassClient == nil {
		http.Error(w, "Userpass authentication not configured", http.StatusInternalServerError)
		return
	}

	// Check for CLI redirect parameters
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/login?error=Invalid form data", http.StatusSeeOther)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")
	redirectURI := r.FormValue("redirect_uri")
	state := r.FormValue("state")

	if email == "" || password == "" {
		http.Redirect(w, r, "/login?error=Email and password are required", http.StatusSeeOther)
		return
	}

	// Get OIDC token for userpass authentication
	oidcToken, expiresIn, err := h.getUserpassOIDCToken(r.Context(), email, password)
	if err != nil {
		if strings.Contains(err.Error(), "email not verified") {
			http.Redirect(w, r, "/login?error=Please verify your email address", http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/login?error=Invalid credentials", http.StatusSeeOther)
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

	if !account.Verified {
		return "", 0, fmt.Errorf("email not verified")
	}

	// Update account with vault entity ID if not already set
	if vaultTokenResp.EntityID != "" && (!account.VaultEntityID.Valid || account.VaultEntityID.String == "") {
		// Update the entity metadata to include account_uuid for ACL templating
		accountUUID := strings.ReplaceAll(strings.ToLower(account.PublicID), "-", "")
		entityMetadata := map[string]string{
			"account_id":   fmt.Sprintf("%d", account.ID),
			"email":        account.Email,
			"account_uuid": accountUUID,
		}
		if updateErr := h.vaultClient.UpdateEntity(ctx, vaultTokenResp.EntityID, entityMetadata); updateErr != nil {
			slog.Warn("Failed to update entity metadata", "err", updateErr, "entity_id", vaultTokenResp.EntityID)
		}

		err = h.db.UpdateAccount(ctx, db.UpdateAccountParams{
			Email:          account.Email,
			Name:           account.Name,
			GithubUsername: account.GithubUsername,
			VaultEntityID:  sql.NullString{String: vaultTokenResp.EntityID, Valid: true},
			AuthMethod:     account.AuthMethod,
			Verified:       account.Verified,
			VerifiedAt:     account.VerifiedAt,
			PublicID:       account.PublicID,
		})
		if err != nil {
			slog.Warn("Failed to update account with vault entity ID", "err", err, "account_id", account.ID)
			// Don't fail the login, just log the warning
		} else {
			slog.Info("updated userpass account with vault entity ID and metadata", "account_id", account.ID, "entity_id", vaultTokenResp.EntityID, "account_uuid", accountUUID)
		}
	}

	// Ensure entity has a token alias for OIDC token creation
	if vaultTokenResp.EntityID != "" {
		if err := h.vaultClient.EnsureTokenAlias(ctx, vaultTokenResp.EntityID, email); err != nil {
			slog.Warn("Failed to ensure token alias for userpass user", "err", err, "entity_id", vaultTokenResp.EntityID)
			// Don't fail the login, just log warning
		}
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

	return oidcToken, ttl, nil
}

// HandleGoogleLoginV2 initiates OAuth flow via Goth for Google.
func (h *Handler) HandleGoogleLoginV2(w http.ResponseWriter, r *http.Request) {
	if h.gothManager == nil {
		http.Error(w, "OAuth not configured", http.StatusInternalServerError)
		return
	}

	// Extract redirect parameters
	redirectURI := r.URL.Query().Get("redirect_uri")
	cliState := r.URL.Query().Get("state")

	var redirectPath string
	if redirectURI != "" {
		// CLI request
		redirectPath = fmt.Sprintf("/cli-callback?redirect_uri=%s&state=%s", redirectURI, cliState)
	} else {
		// Web request
		redirectPath = r.URL.Query().Get("redirect")
		if redirectPath == "" {
			redirectPath = "/"
		}
	}

	authURL, err := h.gothManager.BeginAuth("google", redirectPath)
	if err != nil {
		slog.Error("Failed to begin Google OAuth", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleGitHubLogin initiates OAuth flow via Goth for GitHub.
func (h *Handler) HandleGitHubLogin(w http.ResponseWriter, r *http.Request) {
	if h.gothManager == nil {
		http.Error(w, "OAuth not configured", http.StatusInternalServerError)
		return
	}

	// Extract redirect parameters
	redirectURI := r.URL.Query().Get("redirect_uri")
	cliState := r.URL.Query().Get("state")

	var redirectPath string
	if redirectURI != "" {
		// CLI request
		redirectPath = fmt.Sprintf("/cli-callback?redirect_uri=%s&state=%s", redirectURI, cliState)
	} else {
		// Web request
		redirectPath = r.URL.Query().Get("redirect")
		if redirectPath == "" {
			redirectPath = "/"
		}
	}

	authURL, err := h.gothManager.BeginAuth("github", redirectPath)
	if err != nil {
		slog.Error("Failed to begin GitHub OAuth", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, authURL, http.StatusFound)
}

// HandleOAuthCallback handles OAuth callback from Google/GitHub via Goth.
func (h *Handler) HandleOAuthCallback(w http.ResponseWriter, r *http.Request) {
	if h.gothManager == nil || h.tokenIssuer == nil {
		http.Error(w, "OAuth not configured", http.StatusInternalServerError)
		return
	}

	// Determine provider from URL path
	provider := ""
	if strings.Contains(r.URL.Path, "/google") {
		provider = "google"
	} else if strings.Contains(r.URL.Path, "/github") {
		provider = "github"
	} else {
		http.Error(w, "Invalid OAuth provider", http.StatusBadRequest)
		return
	}

	code := r.URL.Query().Get("code")
	state := r.URL.Query().Get("state")

	if code == "" || state == "" {
		http.Error(w, "Missing code or state parameter", http.StatusBadRequest)
		return
	}

	// Complete OAuth flow
	gothUser, stateData, err := h.gothManager.CompleteAuth(r.Context(), provider, code, state)
	if err != nil {
		slog.Error("Failed to complete OAuth", "err", err, "provider", provider)
		http.Error(w, "Authentication failed", http.StatusUnauthorized)
		return
	}

	// Link or create account
	account, err := h.gothManager.LinkOrCreateAccount(r.Context(), gothUser.Email, gothUser.Name, provider)
	if err != nil {
		if strings.Contains(err.Error(), "verify your email") {
			http.Error(w, "Please verify your email address before linking OAuth account", http.StatusForbidden)
			return
		}
		slog.Error("Failed to link/create account", "err", err, "provider", provider, "email", gothUser.Email)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// Ensure account has Vault entity
	entityID, err := h.ensureVaultEntity(r.Context(), account)
	if err != nil {
		slog.Error("Failed to ensure Vault entity", "err", err, "account_id", account.ID)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	slog.Debug("Vault entity ensured", "entity_id", entityID, "account_id", account.ID)

	// Create entity token
	policies := vault.DeterminePolicies(string(account.AuthMethod))
	entityToken, actualEntityID, err := h.vaultClient.CreateEntityToken(r.Context(), entityID, policies, "1h")
	if err != nil {
		slog.Error("Failed to create entity token", "err", err, "entity_id", entityID)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// If the token was created with a different entity (because alias points elsewhere),
	// update that entity's metadata instead
	if actualEntityID != entityID {
		slog.Warn("Alias returned different entity, updating actual entity metadata",
			"stored_entity", entityID, "actual_entity", actualEntityID, "account_id", account.ID)

		accountUUID := strings.ReplaceAll(strings.ToLower(account.PublicID), "-", "")
		metadata := map[string]string{
			"account_id":   fmt.Sprintf("%d", account.ID),
			"email":        account.Email,
			"account_uuid": accountUUID,
		}

		err = h.vaultClient.UpdateEntity(r.Context(), actualEntityID, metadata)
		if err != nil {
			slog.Error("Failed to update actual entity metadata", "err", err, "entity_id", actualEntityID)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}
		slog.Info("Updated actual entity metadata", "entity_id", actualEntityID, "account_id", account.ID)

		// Update the database to store the correct entity ID
		err = h.db.UpdateAccount(r.Context(), db.UpdateAccountParams{
			Email:          account.Email,
			Name:           account.Name,
			GithubUsername: account.GithubUsername,
			VaultEntityID:  sql.NullString{String: actualEntityID, Valid: true},
			AuthMethod:     account.AuthMethod,
			Verified:       account.Verified,
			VerifiedAt:     account.VerifiedAt,
			PublicID:       account.PublicID,
		})
		if err != nil {
			slog.Warn("Failed to update account with correct entity ID", "err", err, "account_id", account.ID)
		}
	}

	slog.Debug("Entity token created", "entity_id", actualEntityID, "account_id", account.ID)

	// Issue OIDC token using the entity token
	scopes := GetAccountScopesForOAuth()
	scopeStrings := ScopesToStrings(scopes)
	oidcToken, ttl, err := h.vaultClient.GetOIDCTokenWithAccountID(r.Context(), entityToken, h.provider, account.ID, scopeStrings)
	if err != nil {
		slog.Error("Failed to get OIDC token", "err", err, "account_id", account.ID)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	slog.Debug("OIDC token issued", "account_id", account.ID, "ttl", ttl)

	// Validate the OIDC token to verify account_id is correct
	if tokenInfo, err := h.validator.ValidateToken(r.Context(), oidcToken); err == nil {
		slog.Info("OIDC token validated after issuance",
			"account_id_in_token", tokenInfo.AccountID,
			"expected_account_id", account.ID,
			"entity_id", tokenInfo.EntityID,
			"email", tokenInfo.Email)
		if tokenInfo.AccountID != account.ID {
			slog.Error("OIDC token has wrong account_id",
				"token_account_id", tokenInfo.AccountID,
				"expected_account_id", account.ID,
				"entity_id", entityID)
		}
	} else {
		slog.Warn("Failed to validate OIDC token after issuance", "err", err)
	}

	// Set session cookies
	h.sessionManager.SetSessionCookies(w, oidcToken, oidcToken, ttl)

	// Check if this is a CLI callback
	if strings.HasPrefix(stateData.RedirectPath, "/cli-callback") {
		redirectURL, err := url.Parse(stateData.RedirectPath)
		if err != nil {
			slog.Error("Failed to parse redirect path", "err", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		redirectURI := redirectURL.Query().Get("redirect_uri")
		cliState := redirectURL.Query().Get("state")

		if redirectURI != "" {
			cliRedirectURL := fmt.Sprintf("%s?state=%s", redirectURI, cliState)
			http.Redirect(w, r, cliRedirectURL, http.StatusFound)
			return
		}
	}

	// Check if this is an API request
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		name := ""
		if account.Name.Valid {
			name = account.Name.String
		}
		if err := json.NewEncoder(w).Encode(map[string]any{
			"success": true,
			"account": map[string]string{
				"id":    account.PublicID, // Already a string in GetAccountByEmailRow
				"email": account.Email,
				"name":  name,
			},
			"redirect": stateData.RedirectPath,
		}); err != nil {
			slog.Error("Failed to encode response", "err", err)
		}
		return
	}

	// Browser request - redirect
	if stateData.RedirectPath == "/" || stateData.RedirectPath == "" {
		http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, stateData.RedirectPath, http.StatusSeeOther)
}

// ensureVaultEntity ensures an account has a Vault entity and returns the entity ID.
func (h *Handler) ensureVaultEntity(ctx context.Context, account *db.GetAccountByEmailRow) (string, error) {
	// Convert account.PublicID (UUID) to lowercase no-dashes format for Vault ACL templating
	accountUUID := strings.ReplaceAll(strings.ToLower(account.PublicID), "-", "")

	metadata := map[string]string{
		"account_id":   fmt.Sprintf("%d", account.ID),
		"email":        account.Email,
		"account_uuid": accountUUID,
	}

	// If account already has entity ID, check if metadata needs updating
	if account.VaultEntityID.Valid && account.VaultEntityID.String != "" {
		// Read current entity to check if metadata needs updating
		entityInfo, err := h.vaultClient.ValidateEntity(ctx, account.VaultEntityID.String)
		if err != nil {
			return "", fmt.Errorf("failed to validate entity: %w", err)
		}

		// Check if metadata needs updating
		needsUpdate := false
		if entityInfo.Metadata["account_id"] != fmt.Sprintf("%d", account.ID) {
			needsUpdate = true
		}
		if entityInfo.Metadata["email"] != account.Email {
			needsUpdate = true
		}
		if entityInfo.Metadata["account_uuid"] != accountUUID {
			needsUpdate = true
		}

		if needsUpdate {
			slog.Debug("Updating existing Vault entity metadata", "entity_id", account.VaultEntityID.String, "account_id", account.ID, "metadata", metadata)
			err := h.vaultClient.UpdateEntity(ctx, account.VaultEntityID.String, metadata)
			if err != nil {
				return "", fmt.Errorf("failed to update Vault entity metadata: %w", err)
			}
			slog.Info("Updated Vault entity metadata", "entity_id", account.VaultEntityID.String, "account_id", account.ID)
		} else {
			slog.Debug("Entity metadata already up to date", "entity_id", account.VaultEntityID.String, "account_id", account.ID)
		}

		// Ensure entity has token alias
		err = h.vaultClient.EnsureTokenAlias(ctx, account.VaultEntityID.String, account.Email)
		if err != nil {
			return "", fmt.Errorf("failed to ensure token alias: %w", err)
		}

		return account.VaultEntityID.String, nil
	}

	// Create new Vault entity
	policies := vault.DeterminePolicies(string(account.AuthMethod))

	entityID, err := h.vaultClient.CreateEntity(ctx, account.Email, metadata, policies)
	if err != nil {
		return "", fmt.Errorf("failed to create Vault entity: %w", err)
	}

	// Update account with entity ID
	err = h.db.UpdateAccount(ctx, db.UpdateAccountParams{
		Email:          account.Email,
		Name:           account.Name,
		GithubUsername: account.GithubUsername,
		VaultEntityID:  sql.NullString{String: entityID, Valid: true},
		AuthMethod:     account.AuthMethod,
		Verified:       account.Verified,
		VerifiedAt:     account.VerifiedAt,
		PublicID:       account.PublicID, // Already a string in GetAccountByEmailRow
	})
	if err != nil {
		return "", fmt.Errorf("failed to update account with entity ID: %w", err)
	}

	slog.Info("created Vault entity for account", "account_id", account.ID, "entity_id", entityID, "email", account.Email)

	// Ensure entity has a token alias
	err = h.vaultClient.EnsureTokenAlias(ctx, entityID, account.Email)
	if err != nil {
		return "", fmt.Errorf("failed to ensure token alias: %w", err)
	}

	return entityID, nil
}
