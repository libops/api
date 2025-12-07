package auth

import (
	"fmt"
	"os"
	"strings"
)

// Config holds authentication configuration.
type Config struct {
	VaultAddr         string
	VaultToken        string
	VaultOIDCProvider string
	ClientID          string
	ClientSecret      string
	RedirectURL       string
	Scopes            []string
	AdminEmails       []string
}

// NewConfigFromEnv creates auth config from environment variables.
func NewConfigFromEnv() (*Config, error) {
	vaultAddr := getEnv("VAULT_ADDR", "https://vault.libops.io")
	vaultToken := os.Getenv("VAULT_TOKEN")
	provider := getEnv("VAULT_OIDC_PROVIDER", "libops-api")
	clientID := getEnv("OIDC_CLIENT_ID", "libops-api")
	clientSecret := os.Getenv("OIDC_CLIENT_SECRET")
	redirectURL := getEnv("OIDC_REDIRECT_URL", "http://localhost:8080/auth/callback")

	adminEmailsStr := os.Getenv("LIBOPS_ADMIN_EMAILS")
	var adminEmails []string
	if adminEmailsStr != "" {
		adminEmails = strings.Split(adminEmailsStr, ",")
		for i, email := range adminEmails {
			adminEmails[i] = strings.TrimSpace(email)
		}
	}

	if clientSecret == "" {
		return nil, fmt.Errorf("OIDC_CLIENT_SECRET environment variable is required")
	}

	return &Config{
		VaultAddr:         vaultAddr,
		VaultToken:        vaultToken,
		VaultOIDCProvider: provider,
		ClientID:          clientID,
		ClientSecret:      clientSecret,
		RedirectURL:       redirectURL,
		Scopes:            []string{"openid", "email", "profile"},
		AdminEmails:       adminEmails,
	}, nil
}

// getEnv retrieves the value of an environment variable or returns a default value if not set.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
