package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all application configuration.
type Config struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	APIBaseURL string // Base URL for the API (e.g., https://api.libops.io)

	DatabaseURL string

	GCPProjectID  string
	EventsTopicID string

	AllowedOrigins []string

	// Vault Configuration
	VaultAddr         string
	VaultToken        string
	VaultOIDCProvider string

	// OIDC Configuration (API is the OIDC client to Vault)
	OIDCClientID     string
	OIDCClientSecret string
	OIDCRedirectURL  string
}

// Load loads configuration from environment variables and Vault secrets.
// Priority: 1) Environment variables, 2) Vault secrets at /vault/secrets
// Waits up to 120 seconds for required variables to appear in Vault.
func Load() (*Config, error) {
	loader := NewVaultLoader()

	databasePasswordFile := os.Getenv("MARIADB_PASSWORD_FILE")
	if databasePasswordFile == "" {
		return nil, fmt.Errorf("MARIADB_PASSWORD_FILE is required")
	}
	databasePassword, err := os.ReadFile(databasePasswordFile)
	if err != nil || string(databasePassword) == "" {
		return nil, fmt.Errorf("failed to read %s: %w", databasePasswordFile, err)
	}

	oidcClientId, err := loader.LoadEnv("OIDC_CLIENT_ID", true)
	if err != nil {
		return nil, fmt.Errorf("failed to load OIDC_CLIENT_ID: %w", err)
	}

	oidcClientSecret, err := loader.LoadEnv("OIDC_CLIENT_SECRET", true)
	if err != nil {
		return nil, fmt.Errorf("failed to load OIDC_CLIENT_SECRET: %w", err)
	}

	vaultToken, err := loader.LoadEnv("VAULT_TOKEN", true)
	if err != nil {
		return nil, fmt.Errorf("failed to load VAULT_TOKEN: %w", err)
	}

	baseUrl := loader.LoadEnvWithDefault("API_BASE_URL", "https://api.libops.io")

	cfg := &Config{
		Port:         loader.LoadEnvWithDefault("PORT", "8080"),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,

		APIBaseURL: baseUrl,

		DatabaseURL: fmt.Sprintf("libops:%s@tcp(mariadb:3306)/libops?parseTime=true", strings.TrimSpace(string(databasePassword))),

		GCPProjectID:  loader.LoadEnvWithDefault("GCP_PROJECT_ID", ""),
		EventsTopicID: loader.LoadEnvWithDefault("EVENTS_TOPIC_ID", ""),

		AllowedOrigins: parseAllowedOrigins(loader.LoadEnvWithDefault("ALLOWED_ORIGINS", baseUrl)),

		VaultAddr:         loader.LoadEnvWithDefault("VAULT_ADDR", "http://vault.libops.io"),
		VaultToken:        vaultToken,
		VaultOIDCProvider: loader.LoadEnvWithDefault("VAULT_OIDC_PROVIDER", "libops-api"),

		OIDCClientID:     oidcClientId,
		OIDCClientSecret: oidcClientSecret,
		OIDCRedirectURL:  loader.LoadEnvWithDefault("OIDC_REDIRECT_URL", fmt.Sprintf("%s/auth/callback", baseUrl)),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that required configuration is present.
func (cfg *Config) Validate() error {
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.OIDCClientSecret == "" {
		return fmt.Errorf("OIDC_CLIENT_SECRET is required")
	}
	if cfg.VaultToken == "" {
		return fmt.Errorf("VAULT_TOKEN is required")
	}
	return nil
}

// getEnv retrieves an environment variable with a fallback default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// parseAllowedOrigins parses ALLOWED_ORIGINS env var or returns secure defaults
// Format: comma-separated list of origins (e.g., "https://api.libops.io,https://dash.libops.io")
// Default: Production origins (api.libops.io and dash.libops.io).
func parseAllowedOrigins(originsEnv string) []string {
	if originsEnv != "" {
		origins := strings.Split(originsEnv, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
		return origins
	}

	return []string{
		"https://api.libops.io",
		"https://dash.libops.io",
		"http://localhost:8080",
	}
}
