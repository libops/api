package auth

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/hashicorp/vault/api/auth/userpass"
	"golang.org/x/oauth2"

	"github.com/libops/api/internal/audit"
	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/vault"
)

// LibopsTokenRequest represents an OAuth 2.0 token request
// Supports multiple grant types following RFC 6749
type LibopsTokenRequest struct {
	GrantType string `json:"grant_type"` // "password", "google"

	// For grant_type=password (userpass)
	Username string `json:"username,omitempty"` // email
	Password string `json:"password,omitempty"`

	// For grant_type=google
	AccessToken string `json:"access_token,omitempty"` // Google access token
}

// LibopsTokenResponse represents an OAuth 2.0 token response
// This is the ONLY token response format - used everywhere
type LibopsTokenResponse struct {
	AccessToken string `json:"access_token"` // Vault OIDC token (used as Bearer token)
	IDToken     string `json:"id_token"`     // Vault ID token
	ExpiresIn   int    `json:"expires_in"`   // Seconds until expiration
	TokenType   string `json:"token_type"`   // Always "Bearer"
}

// LibopsTokenIssuer handles all token issuance with a single, clean interface
type LibopsTokenIssuer struct {
	vaultClient    *vault.Client
	db             db.Querier
	sessionManager *SessionManager
	vaultAddr      string
	provider       string
	adminEmails    []string
	auditLogger    *audit.Logger
}

// NewLibopsTokenIssuer creates a new token issuer
func NewLibopsTokenIssuer(vaultClient *vault.Client, querier db.Querier, sessionManager *SessionManager, vaultAddr, provider string, adminEmails []string, auditLogger *audit.Logger) *LibopsTokenIssuer {
	return &LibopsTokenIssuer{
		vaultClient:    vaultClient,
		db:             querier,
		sessionManager: sessionManager,
		vaultAddr:      vaultAddr,
		provider:       provider,
		adminEmails:    adminEmails,
		auditLogger:    auditLogger,
	}
}

// HandleToken is the token endpoint
// POST /auth/token
func (ti *LibopsTokenIssuer) HandleToken(w http.ResponseWriter, r *http.Request) {
	var req LibopsTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	var resp *LibopsTokenResponse
	var err error

	switch req.GrantType {
	case "password":
		resp, err = ti.handlePasswordGrant(r.Context(), req.Username, req.Password)
	case "google":
		resp, err = ti.handleGoogleGrant(r.Context(), req.AccessToken)
	default:
		http.Error(w, fmt.Sprintf("Unsupported grant_type: %s", req.GrantType), http.StatusBadRequest)
		return
	}

	if err != nil {
		slog.Error("Token grant failed", "grant_type", req.GrantType, "err", err)
		http.Error(w, err.Error(), http.StatusUnauthorized)
		return
	}

	// Set cookies for browser-based clients
	ti.sessionManager.SetSessionCookies(w, resp.AccessToken, resp.IDToken, resp.ExpiresIn)

	// Always return JSON response
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Error("Failed to encode response", "err", err)
	}
}

// handlePasswordGrant handles userpass authentication
func (ti *LibopsTokenIssuer) handlePasswordGrant(ctx context.Context, email, password string) (*LibopsTokenResponse, error) {
	if email == "" || password == "" {
		return nil, fmt.Errorf("username and password are required")
	}

	account, err := ti.db.GetAccountByEmail(ctx, email)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid credentials")
		}
		return nil, fmt.Errorf("internal error")
	}

	// Rate limiting check
	if account.FailedLoginAttempts >= 5 && account.LastFailedLoginAt.Valid && time.Since(account.LastFailedLoginAt.Time) < 15*time.Minute {
		return nil, fmt.Errorf("too many failed attempts, try again later")
	}

	if account.AuthMethod != "userpass" {
		return nil, fmt.Errorf("invalid auth method for this account")
	}

	if !account.Verified {
		return nil, fmt.Errorf("email not verified")
	}

	// Authenticate with Vault
	vaultUsername := strings.ReplaceAll(email, "@", "_")
	vaultClient := ti.vaultClient.GetAPIClient()
	originalToken := vaultClient.Token()
	defer vaultClient.SetToken(originalToken)

	userpassAuth, err := userpass.NewUserpassAuth(vaultUsername, &userpass.Password{FromString: password}, userpass.WithMountPath("userpass"))
	if err != nil {
		_ = ti.db.IncrementFailedLoginAttempts(ctx, account.ID)
		ti.auditLogger.Log(ctx, account.ID, account.ID, audit.AccountEntityType, audit.UserLoginFailure, map[string]any{"error": "invalid credentials"})
		return nil, fmt.Errorf("authentication failed")
	}

	secret, err := vaultClient.Auth().Login(ctx, userpassAuth)
	if err != nil {
		_ = ti.db.IncrementFailedLoginAttempts(ctx, account.ID)
		ti.auditLogger.Log(ctx, account.ID, account.ID, audit.AccountEntityType, audit.UserLoginFailure, map[string]any{"error": "invalid credentials"})
		return nil, fmt.Errorf("authentication failed")
	}

	_ = ti.db.ResetFailedLoginAttempts(ctx, account.ID)
	ti.auditLogger.Log(ctx, account.ID, account.ID, audit.AccountEntityType, audit.UserLoginSuccess, nil)

	// Get OIDC token from Vault
	userToken := secret.Auth.ClientToken
	scopes := GetAccountScopesForOAuth()
	scopeStrings := ScopesToStrings(scopes)

	oidcToken, ttl, err := ti.vaultClient.GetOIDCTokenWithAccountID(ctx, userToken, ti.provider, account.ID, scopeStrings)
	if err != nil {
		return nil, fmt.Errorf("failed to issue token: %w", err)
	}

	return &LibopsTokenResponse{
		AccessToken: oidcToken,
		IDToken:     oidcToken,
		ExpiresIn:   ttl,
		TokenType:   "Bearer",
	}, nil
}

// handleGoogleGrant handles Google OAuth token exchange
func (ti *LibopsTokenIssuer) handleGoogleGrant(ctx context.Context, accessToken string) (*LibopsTokenResponse, error) {
	if accessToken == "" {
		return nil, fmt.Errorf("access_token is required")
	}

	// Validate Google token
	userInfo, err := ti.validateGoogleToken(ctx, accessToken)
	if err != nil {
		return nil, fmt.Errorf("invalid Google token")
	}

	if !userInfo.EmailVerified {
		return nil, fmt.Errorf("email not verified")
	}

	account, err := ti.db.GetAccountByEmail(ctx, userInfo.Email)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("account not found, please register first")
		}
		return nil, fmt.Errorf("internal error")
	}

	if account.AuthMethod != "google" {
		return nil, fmt.Errorf("invalid auth method for this account")
	}

	// Issue OIDC token from Vault
	isAdmin := ti.isAdmin(userInfo.Email)
	tokenResp, err := ti.issueVaultOIDCToken(ctx, userInfo.Email, account.VaultEntityID.String, isAdmin, string(account.AuthMethod))
	if err != nil {
		return nil, err
	}

	return tokenResp, nil
}

// issueVaultOIDCToken issues an OIDC token from Vault
func (ti *LibopsTokenIssuer) issueVaultOIDCToken(ctx context.Context, email, entityID string, isAdmin bool, authMethod string) (*LibopsTokenResponse, error) {
	account, err := ti.db.GetAccountByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	// Validate entity in Vault
	entityInfo, err := ti.vaultClient.ValidateEntity(ctx, entityID)
	if err != nil {
		return nil, fmt.Errorf("entity validation failed: %w", err)
	}

	// Determine policies
	policies := vault.DeterminePolicies(isAdmin, authMethod)

	// Create entity token
	entityToken, err := ti.vaultClient.CreateEntityToken(ctx, entityInfo.ID, policies, "1h")
	if err != nil {
		return nil, fmt.Errorf("failed to create entity token: %w", err)
	}

	// Get OIDC token
	scopes := GetAccountScopesForOAuth()
	scopeStrings := ScopesToStrings(scopes)

	oidcToken, ttl, err := ti.vaultClient.GetOIDCTokenWithAccountID(ctx, entityToken, ti.provider, account.ID, scopeStrings)
	if err != nil {
		return nil, fmt.Errorf("failed to get OIDC token: %w", err)
	}

	return &LibopsTokenResponse{
		AccessToken: oidcToken,
		IDToken:     oidcToken,
		ExpiresIn:   ttl,
		TokenType:   "Bearer",
	}, nil
}

// validateGoogleToken validates a Google access token
func (ti *LibopsTokenIssuer) validateGoogleToken(ctx context.Context, accessToken string) (*GoogleUserInfo, error) {
	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{
		AccessToken: accessToken,
	})
	client := oauth2.NewClient(ctx, tokenSource)

	resp, err := client.Get("https://www.googleapis.com/oauth2/v2/userinfo")
	if err != nil {
		return nil, fmt.Errorf("failed to call Google userinfo: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google userinfo returned status %d", resp.StatusCode)
	}

	var userInfo GoogleUserInfo
	if err := json.NewDecoder(resp.Body).Decode(&userInfo); err != nil {
		return nil, fmt.Errorf("failed to decode userinfo: %w", err)
	}

	return &userInfo, nil
}

// isAdmin checks if email is an admin
func (ti *LibopsTokenIssuer) isAdmin(email string) bool {
	for _, adminEmail := range ti.adminEmails {
		if email == adminEmail {
			return true
		}
	}
	return false
}
