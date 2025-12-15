// Package auth provides userpass authentication functionality with Vault and email verification.
package auth

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/hashicorp/vault/api/auth/userpass"

	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/validation"
	"github.com/libops/api/internal/vault"
)

// UserpassClient handles username/password authentication with Vault.
type UserpassClient struct {
	vaultClient     *vault.Client
	vaultMountPoint string
	db              db.Querier
	emailVerifier   *EmailVerifier
}

// NewUserpassClient creates a new userpass authentication client.
func NewUserpassClient(vaultClient *vault.Client, mountPoint string, querier db.Querier, emailVerifier *EmailVerifier) *UserpassClient {
	return &UserpassClient{
		vaultClient:     vaultClient,
		vaultMountPoint: mountPoint,
		db:              querier,
		emailVerifier:   emailVerifier,
	}
}

// Register creates a new user account with email verification.
func (c *UserpassClient) Register(ctx context.Context, email, password string) (*EmailVerificationToken, error) {
	_, err := c.db.GetAccountByEmail(ctx, email)
	if err == nil {
		slog.Info("registration attempted for existing account", "email", email)
		return nil, fmt.Errorf("registration failed")
	}
	if err != sql.ErrNoRows {
		slog.Error("failed to check existing account", "err", err)
		return nil, fmt.Errorf("internal server error")
	}

	// 1. Create Vault User
	vaultUsername := strings.ReplaceAll(email, "@", "_")
	userPath := fmt.Sprintf("auth/%s/users/%s", c.vaultMountPoint, vaultUsername)
	data := map[string]any{
		"password": password,
		"policies": []string{"default", "libops-user"},
	}

	_, err = c.vaultClient.GetAPIClient().Logical().Write(userPath, data)
	if err != nil {
		slog.Error("failed to create vault user", "err", err)
		return nil, fmt.Errorf("internal server error")
	}

	// 2. Create Account (Unverified, no Entity ID yet)
	err = c.db.CreateAccount(ctx, db.CreateAccountParams{
		Email:          email,
		Name:           sql.NullString{Valid: false},
		GithubUsername: sql.NullString{Valid: false},
		VaultEntityID:  sql.NullString{Valid: false},
		AuthMethod:     "userpass",
		Verified:       false,
		VerifiedAt:     sql.NullTime{Valid: false},
	})
	if err != nil {
		slog.Error("failed to create account", "err", err)
		return nil, fmt.Errorf("internal server error")
	}

	// 3. Get Account to retrieve IDs for metadata
	account, err := c.db.GetAccountByEmail(ctx, email)
	if err != nil {
		slog.Error("failed to get created account", "err", err)
		return nil, fmt.Errorf("internal server error")
	}

	// 4. Create Entity with metadata
	// The account_id metadata is critical for OIDC token generation
	accountUUID := strings.ReplaceAll(strings.ToLower(account.PublicID), "-", "")
	entityMetadata := map[string]string{
		"email":        email,
		"account_id":   fmt.Sprintf("%d", account.ID),
		"account_uuid": accountUUID,
	}

	entityID, err := c.vaultClient.CreateEntity(ctx, email, entityMetadata, []string{"default", "libops-user"})
	if err != nil {
		slog.Error("failed to create entity", "err", err)
		return nil, fmt.Errorf("internal server error")
	}

	// 5. Create Alias
	mountAccessor, err := c.vaultClient.GetAuthMountAccessor(ctx, "userpass")
	if err != nil {
		slog.Error("failed to get userpass mount accessor", "err", err)
		return nil, fmt.Errorf("internal server error")
	}

	if err := c.vaultClient.CreateEntityAlias(ctx, entityID, mountAccessor, vaultUsername); err != nil {
		slog.Error("failed to create entity alias", "err", err)
		return nil, fmt.Errorf("internal server error")
	}

	// 6. Update Account with Entity ID
	err = c.db.UpdateAccount(ctx, db.UpdateAccountParams{
		Email:          account.Email,
		Name:           account.Name,
		GithubUsername: account.GithubUsername,
		VaultEntityID:  sql.NullString{String: entityID, Valid: true},
		AuthMethod:     account.AuthMethod,
		Verified:       account.Verified,
		VerifiedAt:     account.VerifiedAt,
		PublicID:       account.PublicID,
	})
	if err != nil {
		slog.Error("failed to update account with entity ID", "err", err)
		return nil, fmt.Errorf("internal server error")
	}

	// 7. Create Token
	token, err := c.emailVerifier.CreateVerificationToken(ctx, email, "")
	if err != nil {
		slog.Error("failed to create verification token", "err", err)
		return nil, fmt.Errorf("internal server error")
	}

	return token, nil
}

// VerifyEmail verifies the email and activates the account.
func (c *UserpassClient) VerifyEmail(ctx context.Context, email, tokenString string) error {
	if err := c.emailVerifier.VerifyToken(ctx, email, tokenString); err != nil {
		slog.Error("invalid verification token", "err", err)
		return fmt.Errorf("invalid verification token")
	}

	// Fetch existing account to update it
	account, err := c.db.GetAccountByEmail(ctx, email)
	if err != nil {
		slog.Error("failed to get account for verification", "err", err)
		return fmt.Errorf("internal server error")
	}

	// Update account verification status
	now := sql.NullTime{Time: time.Now(), Valid: true}
	err = c.db.UpdateAccount(ctx, db.UpdateAccountParams{
		Email:          account.Email,
		Name:           account.Name,
		GithubUsername: account.GithubUsername,
		VaultEntityID:  account.VaultEntityID,
		AuthMethod:     account.AuthMethod,
		Verified:       true,
		VerifiedAt:     now,
		PublicID:       account.PublicID,
	})
	if err != nil {
		slog.Error("failed to update account verification status", "err", err)
		return fmt.Errorf("internal server error")
	}

	if err := c.emailVerifier.DeleteToken(ctx, email); err != nil {
		// Log but don't fail
		slog.Warn("failed to delete verification token", "err", err)
	}

	return nil
}

// Login authenticates a user with username and password.
func (c *UserpassClient) Login(ctx context.Context, email, password string) (*VaultTokenResponse, error) {
	// Sanitize email for Vault username (replace @ with _)
	vaultUsername := strings.ReplaceAll(email, "@", "_")
	userpassAuth, err := userpass.NewUserpassAuth(vaultUsername, &userpass.Password{FromString: password}, userpass.WithMountPath(c.vaultMountPoint))
	if err != nil {
		slog.Error("failed to create userpass auth", "err", err)
		return nil, fmt.Errorf("internal server error")
	}

	// Clone the client to avoid modifying the shared client's token
	clonedClient, err := c.vaultClient.Clone()
	if err != nil {
		slog.Error("failed to clone vault client", "err", err)
		return nil, fmt.Errorf("internal server error")
	}

	authInfo, err := clonedClient.GetAPIClient().Auth().Login(ctx, userpassAuth)
	if err != nil {
		slog.Error("login failed", "err", err)
		return nil, fmt.Errorf("login failed")
	}

	if authInfo == nil || authInfo.Auth == nil || authInfo.Auth.ClientToken == "" {
		slog.Error("login returned no token")
		return nil, fmt.Errorf("internal server error")
	}

	entityID := ""
	if authInfo.Auth.EntityID != "" {
		entityID = authInfo.Auth.EntityID
	}

	// TODO: Update account with entity ID if not set
	// account, err := c.db.GetAccountByEmail(ctx, email)
	// if err != nil {
	// 	slog.Error("failed to get account", "err", err)
	// 	return nil, fmt.Errorf("internal server error")
	// }
	// if !account.VaultEntityID.Valid && entityID != "" {
	// 	// Update the account with entity ID
	// }

	return &VaultTokenResponse{
		VaultToken:    authInfo.Auth.ClientToken,
		EntityID:      entityID,
		LeaseDuration: authInfo.Auth.LeaseDuration,
		Renewable:     authInfo.Auth.Renewable,
	}, nil
}

// VaultTokenResponse represents a Vault authentication response.
type VaultTokenResponse struct {
	VaultToken    string
	EntityID      string
	LeaseDuration int
	Renewable     bool
}

// HandleRegister handles userpass registration requests.
func (c *UserpassClient) HandleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		slog.Warn("Error parsing form data", "err", err)
		http.Redirect(w, r, "/login?register=true&error=Invalid form data", http.StatusSeeOther)
		return
	}

	email := r.FormValue("email")
	password := r.FormValue("password")

	if email == "" || password == "" {
		http.Redirect(w, r, "/login?register=true&error=Email and password are required", http.StatusSeeOther)
		return
	}

	if err := validatePasswordComplexity(password); err != nil {
		slog.Info("Password complexity validation failed", "email", email, "err", err)
		http.Redirect(w, r, fmt.Sprintf("/login?register=true&error=%s", url.QueryEscape(err.Error())), http.StatusSeeOther)
		return
	}

	if err := validation.Email(email); err != nil {
		slog.Warn("Invalid email format", "email", email, "err", err)
		http.Redirect(w, r, fmt.Sprintf("/login?register=true&error=%s", url.QueryEscape(err.Error())), http.StatusSeeOther)
		return
	}

	token, err := c.Register(r.Context(), email, password)
	if err != nil {
		slog.Error("Registration failed", "err", err)
		http.Redirect(w, r, "/login?register=true&error=Registration failed", http.StatusSeeOther)
		return
	}

	if err := c.emailVerifier.SendVerificationEmail(email, token.Token); err != nil {
		slog.Error("Failed to send verification email", "err", err)
		http.Redirect(w, r, "/login?register=true&error=Internal Server Error", http.StatusSeeOther)
		return
	}

	http.Redirect(w, r, "/login?message=Registration successful. Please check your email to verify your account.", http.StatusSeeOther)
}

// HandleVerifyEmail handles email verification requests.
func (c *UserpassClient) HandleVerifyEmail(w http.ResponseWriter, r *http.Request) {
	email := r.URL.Query().Get("email")
	token := r.URL.Query().Get("token")

	if email == "" || token == "" {
		http.Error(w, "Email and token are required", http.StatusBadRequest)
		return
	}

	if err := c.VerifyEmail(r.Context(), email, token); err != nil {
		slog.Error("Verification failed", "err", err)
		http.Error(w, "Verification failed", http.StatusBadRequest)
		return
	}

	// Redirect to login page with verification success message
	// User can now log in with their credentials
	http.Redirect(w, r, "/login?verified=true", http.StatusSeeOther)
}

// HandleResendVerification resends the verification email for unverified accounts.
func (c *UserpassClient) HandleResendVerification(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if err := r.ParseForm(); err != nil {
		http.Error(w, "Error parsing form data", http.StatusBadRequest)
		return
	}

	email := r.FormValue("email")
	if email == "" {
		http.Error(w, "Email is required", http.StatusBadRequest)
		return
	}

	if err := validation.Email(email); err != nil {
		slog.Warn("Invalid email format", "email", email, "err", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	ctx := r.Context()

	token, err := c.db.GetEmailVerificationTokenByEmail(ctx, email)

	// For security, always return the same success message
	// This prevents attackers from enumerating valid email addresses
	successMessage := `{"message": "If an unverified account exists for this email, a verification link has been sent."}`

	if err == sql.ErrNoRows {
		account, err := c.db.GetAccountByEmail(ctx, email)
		if err == nil && account.Verified {
			// Account already verified - don't reveal this
			slog.Info("Resend attempted for already verified account", "email", email)
		} else if err == nil && !account.Verified {
			// This is an edge case - account may be in invalid state
			slog.Warn("Unverified account found without pending token", "email", email)
		} else {
			slog.Info("Resend attempted for non-existent email", "email", email)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprint(w, successMessage); err != nil {
			slog.Error("Failed to write response", "err", err)
		}
		return
	}

	if err != nil {
		slog.Error("Failed to check verification token", "err", err)
		// Even on error, return success message for security
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprint(w, successMessage); err != nil {
			slog.Error("Failed to write response", "err", err)
		}
		return
	}

	if err := c.emailVerifier.SendVerificationEmail(email, token.Token); err != nil {
		slog.Error("Failed to send verification email", "err", err, "email", email)
		// Still return success to prevent email enumeration
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if _, err := fmt.Fprint(w, successMessage); err != nil {
			slog.Error("Failed to write response", "err", err)
		}
		return
	}

	slog.Info("Verification email resent", "email", email)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := fmt.Fprint(w, successMessage); err != nil {
		slog.Error("Failed to write response", "err", err)
	}
}

// validatePasswordComplexity checks if a password meets the complexity requirements.
func validatePasswordComplexity(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}
	if !regexp.MustCompile(`[A-Z]`).MatchString(password) {
		return fmt.Errorf("password must contain uppercase letter")
	}
	if !regexp.MustCompile(`[a-z]`).MatchString(password) {
		return fmt.Errorf("password must contain lowercase letter")
	}
	if !regexp.MustCompile(`[0-9]`).MatchString(password) {
		return fmt.Errorf("password must contain number")
	}
	if !regexp.MustCompile(`[^a-zA-Z0-9]`).MatchString(password) {
		return fmt.Errorf("password must contain special character")
	}
	return nil
}
