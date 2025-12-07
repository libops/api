package auth

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// OIDCClient handles OIDC authentication flow with Vault.
type OIDCClient struct {
	config        *Config
	discoveryDoc  *DiscoveryDocument
	stateMutex    sync.RWMutex
	pendingStates map[string]*StateData
}

// StateData holds temporary state for OIDC flow.
type StateData struct {
	State        string
	Nonce        string
	RedirectPath string
	CreatedAt    time.Time
}

// DiscoveryDocument represents OIDC discovery metadata.
type DiscoveryDocument struct {
	Issuer                string `json:"issuer"`
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	JwksURI               string `json:"jwks_uri"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

// NewOIDCClient creates a new OIDC client.
func NewOIDCClient(config *Config) (*OIDCClient, error) {
	client := &OIDCClient{
		config:        config,
		pendingStates: make(map[string]*StateData),
	}

	if err := client.fetchDiscoveryDocument(); err != nil {
		return nil, fmt.Errorf("failed to fetch discovery document: %w", err)
	}

	go client.cleanupExpiredStates()

	return client, nil
}

// fetchDiscoveryDocument fetches OIDC discovery metadata from Vault.
func (c *OIDCClient) fetchDiscoveryDocument() error {
	// Format: {vault_addr}/v1/identity/oidc/provider/{provider}/.well-known/openid-configuration
	discoveryURL := fmt.Sprintf("%s/v1/identity/oidc/provider/%s/.well-known/openid-configuration",
		c.config.VaultAddr, c.config.VaultOIDCProvider)

	resp, err := http.Get(discoveryURL)
	if err != nil {
		return fmt.Errorf("failed to fetch discovery document: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("Failed to close response body", "err", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("discovery endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var doc DiscoveryDocument
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return fmt.Errorf("failed to decode discovery document: %w", err)
	}

	c.discoveryDoc = &doc
	return nil
}

// BuildAuthorizationURL creates the authorization URL to redirect user to Vault.
func (c *OIDCClient) BuildAuthorizationURL(provider string, redirectPath string) (string, error) {
	state, err := generateRandomString(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate state: %w", err)
	}

	nonce, err := generateRandomString(32)
	if err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	c.stateMutex.Lock()
	c.pendingStates[state] = &StateData{
		State:        state,
		Nonce:        nonce,
		RedirectPath: redirectPath,
		CreatedAt:    time.Now(),
	}
	c.stateMutex.Unlock()

	params := url.Values{}
	params.Set("client_id", c.config.ClientID)
	params.Set("response_type", "code")
	params.Set("redirect_uri", c.config.RedirectURL)
	params.Set("scope", strings.Join(c.config.Scopes, " "))
	params.Set("state", state)
	params.Set("nonce", nonce)

	// Add acr_values to hint which upstream provider to use (google, github, etc)
	if provider != "" {
		params.Set("acr_values", provider)
	}

	authURL := fmt.Sprintf("%s?%s", c.discoveryDoc.AuthorizationEndpoint, params.Encode())
	return authURL, nil
}

// ExchangeCode exchanges authorization code for tokens.
func (c *OIDCClient) ExchangeCode(ctx context.Context, code, state string) (*TokenResponse, error) {
	c.stateMutex.Lock()
	stateData, exists := c.pendingStates[state]
	if !exists {
		c.stateMutex.Unlock()
		return nil, fmt.Errorf("invalid state parameter")
	}
	delete(c.pendingStates, state)
	c.stateMutex.Unlock()

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", c.config.RedirectURL)
	data.Set("client_id", c.config.ClientID)
	data.Set("client_secret", c.config.ClientSecret)

	req, err := http.NewRequestWithContext(ctx, "POST", c.discoveryDoc.TokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("failed to create token request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to exchange code: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			slog.Error("Failed to close response body", "err", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("token endpoint returned status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	tokenResp.StateData = stateData
	return &tokenResp, nil
}

// TokenResponse represents the token response from Vault.
type TokenResponse struct {
	AccessToken string     `json:"access_token"`
	TokenType   string     `json:"token_type"`
	ExpiresIn   int        `json:"expires_in"`
	IDToken     string     `json:"id_token"`
	StateData   *StateData `json:"-"`
}

// cleanupExpiredStates periodically removes expired OIDC state data from the pending states map.
func (c *OIDCClient) cleanupExpiredStates() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		c.stateMutex.Lock()
		now := time.Now()
		for state, data := range c.pendingStates {
			// Remove states older than 10 minutes
			if now.Sub(data.CreatedAt) > 10*time.Minute {
				delete(c.pendingStates, state)
			}
		}
		c.stateMutex.Unlock()
	}
}

// generateRandomString generates a cryptographically secure random string of the specified length.
func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes)[:length], nil
}
