package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/lestrrat-go/jwx/v3/jwk"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

// VaultJWTValidator validates JWTs issued by Vault.
type VaultJWTValidator struct {
	vaultAddr         string
	vaultOIDCProvider string
	apiKeyManager     *APIKeyManager
	jwksSet           jwk.Set
	issuer            string
}

// NewJWTValidator creates a new JWT validator.
func NewJWTValidator(vaultAddr, vaultOIDCProvider string) *VaultJWTValidator {
	return &VaultJWTValidator{
		vaultAddr:         vaultAddr,
		vaultOIDCProvider: vaultOIDCProvider,
	}
}

// Initialize fetches the OIDC discovery document and keys.
func (v *VaultJWTValidator) Initialize(ctx context.Context) error {
	slog.Info("Initializing JWT validator", "vault", v.vaultAddr, "provider", v.vaultOIDCProvider)

	// Fetch Discovery Document
	discoveryURL := fmt.Sprintf("%s/v1/identity/oidc/provider/%s/.well-known/openid-configuration",
		v.vaultAddr, v.vaultOIDCProvider)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(discoveryURL)
	if err != nil {
		return fmt.Errorf("failed to fetch discovery document: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("Failed to close response body", "err", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("discovery endpoint returned status %d", resp.StatusCode)
	}

	var doc struct {
		Issuer  string `json:"issuer"`
		JwksURI string `json:"jwks_uri"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("failed to decode discovery document: %w", err)
	}

	v.issuer = doc.Issuer

	jwksURL := doc.JwksURI
	// If vaultAddr is set, ensure we use its host for JWKS as well
	// This handles cases where Vault advertises 0.0.0.0 or localhost but is accessed via a service name
	if v.vaultAddr != "" {
		if vaultURL, err := url.Parse(v.vaultAddr); err == nil {
			if parsedJwks, err := url.Parse(jwksURL); err == nil {
				parsedJwks.Scheme = vaultURL.Scheme
				parsedJwks.Host = vaultURL.Host
				jwksURL = parsedJwks.String()
			}
		}
	}

	// Fetch JWKS
	// Use jwk.Fetch to get the keyset
	set, err := jwk.Fetch(ctx, jwksURL)
	if err != nil {
		return fmt.Errorf("failed to fetch JWKS: %w", err)
	}

	v.jwksSet = set
	slog.Info("JWT validator initialized", "issuer", v.issuer, "keys_count", set.Len())

	return nil
}

// SetAPIKeyManager sets the API key manager for hybrid authentication.
func (v *VaultJWTValidator) SetAPIKeyManager(manager *APIKeyManager) {
	v.apiKeyManager = manager
}

// Middleware validates the JWT token or API key in the request.
func (v *VaultJWTValidator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		authHeader := r.Header.Get("Authorization")
		tokenString := ""

		// 1. Check for API Key or Bearer Token in Header
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && strings.EqualFold(parts[0], "Bearer") {
				tokenString = parts[1]
			}
		}

		// 2. Check cookies if no header token
		if tokenString == "" {
			if cookie, err := r.Cookie("vault_token"); err == nil {
				tokenString = cookie.Value
			} else if cookie, err := r.Cookie("id_token"); err == nil {
				tokenString = cookie.Value
			}
		}

		// If still no token, proceed unauthenticated
		if tokenString == "" {
			next.ServeHTTP(w, r)
			return
		}

		// 3. Validate API Key (starts with libops_)
		if strings.HasPrefix(tokenString, "libops_") {
			if v.apiKeyManager != nil {
				apiKeyInfo, err := v.apiKeyManager.ValidateAPIKey(ctx, tokenString)
				if err != nil {
					// Invalid API key - log and proceed unauthenticated
					slog.Warn("Invalid API key", "err", err)
				} else {
					// Map APIKeyInfo to UserInfo
					userInfo := &UserInfo{
						EntityID:  apiKeyInfo.EntityID,
						Email:     apiKeyInfo.Email,
						Name:      apiKeyInfo.Name,
						AccountID: apiKeyInfo.AccountID,
						Scopes:    apiKeyInfo.Scopes,
					}
					ctx = context.WithValue(ctx, UserContextKey, userInfo)
				}
			}
			next.ServeHTTP(w, r.WithContext(ctx))
			return
		}

		// 4. Validate JWT
		userInfo, err := v.ValidateToken(ctx, tokenString)
		if err != nil {
			// Token present but invalid.
			// To avoid redirect loops, we just log it and don't set the user in context.
			slog.Debug("Invalid JWT token", "err", err)
		} else {
			ctx = context.WithValue(ctx, UserContextKey, userInfo)
		}

		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// ValidateToken validates a raw JWT token string.
func (v *VaultJWTValidator) ValidateToken(ctx context.Context, tokenString string) (*UserInfo, error) {
	if v.jwksSet == nil {
		return nil, fmt.Errorf("validator not initialized")
	}

	// Parse and validate token
	token, err := jwt.Parse([]byte(tokenString), jwt.WithKeySet(v.jwksSet), jwt.WithValidate(true))
	if err != nil {
		return nil, fmt.Errorf("failed to verify token: %w", err)
	}

	// Validate claims
	if v.issuer != "" {
		iss, ok := token.Issuer()
		if !ok {
			return nil, fmt.Errorf("token missing issuer")
		}
		if iss != v.issuer {
			// Fallback: Allow the default Vault OIDC issuer path
			// This handles cases where the provider uses the default key/issuer
			// v.issuer is like ".../v1/identity/oidc/provider/libops-api"
			// We want to accept ".../v1/identity/oidc"
			suffix := "/provider/" + v.vaultOIDCProvider
			defaultIssuer := strings.TrimSuffix(v.issuer, suffix)

			if iss != defaultIssuer {
				return nil, fmt.Errorf("invalid issuer: expected %s (or %s), got %s", v.issuer, defaultIssuer, iss)
			}
		}
	}

	// Extract claims
	entityID, ok := token.Subject()
	if !ok || entityID == "" {
		return nil, fmt.Errorf("token missing subject (entity_id)")
	}

	var email string
	err = token.Get("email", &email)
	if err != nil {
		return nil, fmt.Errorf("unable to get email from jwt: %w", err)
	}
	var name string
	_ = token.Get("name", &name)
	if err != nil {
		return nil, fmt.Errorf("unable to get name from jwt: %w", err)
	}
	// Try to get Account ID if available in custom claims
	var accountID int64
	var rawAccountID any
	if err := token.Get("account_id", &rawAccountID); err == nil {
		switch v := rawAccountID.(type) {
		case float64:
			accountID = int64(v)
		case int64:
			accountID = v
		case string:
			_, err = fmt.Sscanf(v, "%d", &accountID)
			if err != nil {
				return nil, fmt.Errorf("unable to get account_id from jwt: %w", err)
			}
		}
	}

	return &UserInfo{
		EntityID:  entityID,
		Email:     email,
		Name:      name,
		AccountID: accountID,
	}, nil
}
