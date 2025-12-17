package middleware

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lestrrat-go/httprc/v3"
	"github.com/lestrrat-go/httprc/v3/tracesink"
	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"

	"github.com/libops/api/internal/auth"
)

// ContextKey is the type for context keys.
type ContextKey string

const (
	// UserContextKey is the key for storing user info in context.
	UserContextKey ContextKey = "user"
)

// JWTValidator validates JWT tokens from Vault.
type JWTValidator struct {
	vaultAddr string
	provider  string
	jwksCache *jwk.Cache
	jwksURL   string
	apiKeyMgr *auth.APIKeyManager
}

// NewJWTValidator creates a new JWT validator with automatic JWKS caching.
func NewJWTValidator(vaultAddr, provider string) *JWTValidator {
	jwksURL := fmt.Sprintf("%s/v1/identity/oidc/provider/%s/.well-known/keys",
		vaultAddr, provider)

	return &JWTValidator{
		vaultAddr: vaultAddr,
		provider:  provider,
		jwksURL:   jwksURL,
	}
}

// Initialize sets up the JWKS cache with automatic refresh.
func (v *JWTValidator) Initialize(ctx context.Context) error {
	// Use the default logger which is now context-aware
	// This ensures JWKS refresh operations will include request_id when available
	cache, err := jwk.NewCache(
		ctx,
		httprc.NewClient(
			httprc.WithTraceSink(tracesink.NewSlog(slog.Default())),
		),
	)
	if err != nil {
		return fmt.Errorf("failed to create JWKS cache: %w", err)
	}

	err = cache.Register(
		ctx,
		v.jwksURL,
		jwk.WithMaxInterval(240*time.Hour),
		jwk.WithMinInterval(1*time.Hour),
	)
	if err != nil {
		return fmt.Errorf("failed to register JWKS URL: %w", err)
	}

	_, err = cache.Refresh(ctx, v.jwksURL)
	if err != nil {
		return fmt.Errorf("failed to perform initial JWKS fetch: %w", err)
	}

	v.jwksCache = cache
	return nil
}

// SetAPIKeyManager sets the API key manager for the validator
// This is called after initialization to avoid circular dependencies.
func (v *JWTValidator) SetAPIKeyManager(apiKeyMgr *auth.APIKeyManager) {
	v.apiKeyMgr = apiKeyMgr
}

// Middleware returns an HTTP middleware that validates JWT tokens or API keys
// For JWTs: Only validates signature and expiry (no database lookups)
// For API keys: Validates against Vault and database.
func (v *JWTValidator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for public endpoints
		if isPublicEndpoint(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		var tokenString string

		authHeader := r.Header.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				tokenString = parts[1]
			}
		}

		if tokenString == "" {
			cookie, err := r.Cookie("id_token")
			if err == nil && cookie.Value != "" {
				tokenString = cookie.Value
			}
		}

		if tokenString == "" {
			http.Error(w, "Missing authentication token", http.StatusUnauthorized)
			return
		}

		if strings.HasPrefix(tokenString, "libops_") {
			if v.apiKeyMgr == nil {
				http.Error(w, "API key authentication not configured", http.StatusInternalServerError)
				return
			}

			// Validate API key
			apiKeyInfo, err := v.apiKeyMgr.ValidateAPIKey(r.Context(), tokenString)
			if err != nil {
				http.Error(w, "Invalid API key", http.StatusUnauthorized)
				return
			}

			userInfo := &auth.UserInfo{
				EntityID:  apiKeyInfo.EntityID,
				Email:     apiKeyInfo.Email,
				Name:      apiKeyInfo.Name,
				AccountID: apiKeyInfo.AccountID,
				Scopes:    apiKeyInfo.Scopes,
				Metadata: map[string]string{
					"auth_type":    "api_key",
					"key_uuid":     apiKeyInfo.KeyUUID,
					"key_name":     apiKeyInfo.KeyName,
					"account_uuid": apiKeyInfo.AccountUUID,
				},
			}

			ctx := context.WithValue(r.Context(), auth.UserContextKey, userInfo)
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// Otherwise, validate as JWT
		userInfo, err := v.ValidateToken(r.Context(), tokenString)
		if err != nil {
			slog.Error("Invalid token", "err", err)
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), auth.UserContextKey, userInfo)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ValidateToken validates a JWT token from Vault and extracts user info.
func (v *JWTValidator) ValidateToken(ctx context.Context, tokenString string) (*auth.UserInfo, error) {
	if v.jwksCache == nil {
		return nil, fmt.Errorf("JWT validator not initialized")
	}

	keyset, err := v.jwksCache.Lookup(ctx, v.jwksURL)
	if err != nil {
		return nil, fmt.Errorf("failed to get cached keyset: %w", err)
	}

	token, err := jwt.Parse(
		[]byte(tokenString),
		jwt.WithKeySet(keyset),
		jwt.WithValidate(true),
		jwt.WithAcceptableSkew(30*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to parse and validate token: %w", err)
	}

	userInfo := &auth.UserInfo{
		Metadata: make(map[string]string),
	}

	sub, ok := token.Subject()
	if !ok || sub == "" {
		return nil, fmt.Errorf("missing sub claim")
	}
	userInfo.EntityID = sub

	var accountIDVal any
	if err := token.Get("account_id", &accountIDVal); err == nil {
		switch accountID := accountIDVal.(type) {
		case float64:
			userInfo.AccountID = int64(accountID)
		case int64:
			userInfo.AccountID = accountID
		case int:
			userInfo.AccountID = int64(accountID)
		case string:
			if id, err := strconv.ParseInt(accountID, 10, 64); err == nil {
				userInfo.AccountID = id
			}
		}
	}

	var email string
	if err := token.Get("email", &email); err == nil {
		userInfo.Email = email
	}

	var name string
	if err := token.Get("name", &name); err == nil {
		userInfo.Name = name
	}

	userInfo.Metadata["sub"] = sub
	if userInfo.Email != "" {
		userInfo.Metadata["email"] = userInfo.Email
	}
	if userInfo.Name != "" {
		userInfo.Metadata["name"] = userInfo.Name
	}

	var scopeStrs []string
	if err := token.Get("scopes", &scopeStrs); err == nil && len(scopeStrs) > 0 {
		scopes, err := auth.ParseScopes(scopeStrs)
		if err != nil {
			userInfo.Scopes = []auth.Scope{}
		} else {
			userInfo.Scopes = scopes
		}
	} else {
		userInfo.Scopes = auth.GetAccountScopesForOAuth()
	}

	return userInfo, nil
}

// isPublicEndpoint determines if an endpoint can be accessed without authentication.
func isPublicEndpoint(path string) bool {
	publicPrefixes := []string{
		"/static/",
		"/health",
		"/version",
		"/openapi",
		"/auth/token",
		"/auth/register/",
		"/auth/userpass/",
		"/auth/verify",
		"/auth/login",
		"/auth/callback",
		"/auth/callback/github",
		"/auth/callback/google",
		"/auth/logout",
		"/webhooks",
	}

	for _, p := range publicPrefixes {
		if strings.HasPrefix(path, p) {
			return true
		}
	}

	return path == "/" || path == "/login"
}
