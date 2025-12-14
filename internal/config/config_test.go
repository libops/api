package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestLoad_OidcClientFromVault tests loading OIDC_CLIENT_SECRET from vault
func TestLoad_OidcClientFromVault(t *testing.T) {
	// Ensure OIDC_CLIENT_SECRET is not in environment
	_ = os.Unsetenv("OIDC_CLIENT_SECRET")
	dbPath := "/tmp/foo"
	_ = os.Setenv("MARIADB_PASSWORD_FILE", dbPath)
	if err := os.WriteFile(dbPath, []byte("bar"), 0600); err != nil {
		t.Errorf("failed to write MARIADB_PASSWORD_FILE: %v", err)
	}

	// Set up vault secrets directory
	tmpDir := t.TempDir()
	_ = os.Setenv("VAULT_SECRETS_DIR", tmpDir)
	defer func() { _ = os.Unsetenv("VAULT_SECRETS_DIR") }()

	expectedOidcId := "foo"
	expectedOIDCSecret := "vault-oidc-secret"
	expectedVaultToken := "vault-token-value"

	go func() {
		time.Sleep(5 * time.Second)
		oidcPath := filepath.Join(tmpDir, "OIDC_CLIENT_SECRET")
		if err := os.WriteFile(oidcPath, []byte(expectedOIDCSecret), 0600); err != nil {
			t.Errorf("failed to write OIDC_CLIENT_SECRET: %v", err)
		}
		oidcPath = filepath.Join(tmpDir, "OIDC_CLIENT_ID")
		if err := os.WriteFile(oidcPath, []byte(expectedOidcId), 0600); err != nil {
			t.Errorf("failed to write OIDC_CLIENT_ID: %v", err)
		}
		vaultPath := filepath.Join(tmpDir, "VAULT_TOKEN")
		if err := os.WriteFile(vaultPath, []byte(expectedVaultToken), 0600); err != nil {
			t.Errorf("failed to write VAULT_TOKEN: %v", err)
		}
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.OIDCClientID != expectedOidcId {
		t.Errorf("expected OIDC_CLIENT_ID %q, got %q", expectedOidcId, cfg.OIDCClientSecret)
	}
	if cfg.OIDCClientSecret != expectedOIDCSecret {
		t.Errorf("expected OIDC_CLIENT_SECRET %q, got %q", expectedOIDCSecret, cfg.OIDCClientSecret)
	}
	if cfg.VaultToken != expectedVaultToken {
		t.Errorf("expected VAULT_TOKEN %q, got %q", expectedVaultToken, cfg.VaultToken)
	}

	_ = os.Remove(dbPath)
}

// TestValidate tests the Validate method of the Config struct.
func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *Config
		wantErr bool
	}{
		{
			name: "valid config",
			config: &Config{
				DatabaseURL:      "user:pass@tcp(localhost:3306)/dbname",
				OIDCClientSecret: "test-secret",
				VaultToken:       "test-token",
				Port:             "8080",
			},
			wantErr: false,
		},
		{
			name: "missing database URL",
			config: &Config{
				OIDCClientSecret: "test-secret",
				VaultToken:       "test-token",
				Port:             "8080",
			},
			wantErr: true,
		},
		{
			name: "missing OIDC client secret",
			config: &Config{
				DatabaseURL: "user:pass@tcp(localhost:3306)/dbname",
				VaultToken:  "test-token",
				Port:        "8080",
			},
			wantErr: true,
		},
		{
			name: "missing Vault token",
			config: &Config{
				DatabaseURL:      "user:pass@tcp(localhost:3306)/dbname",
				OIDCClientSecret: "test-secret",
				Port:             "8080",
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

// TestGetEnv tests the getEnv helper function for retrieving environment variables.
func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		defaultValue string
		envValue     string
		want         string
	}{
		{
			name:         "environment variable set",
			key:          "TEST_VAR",
			defaultValue: "default",
			envValue:     "custom",
			want:         "custom",
		},
		{
			name:         "environment variable not set",
			key:          "TEST_VAR",
			defaultValue: "default",
			envValue:     "",
			want:         "default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				_ = os.Setenv(tt.key, tt.envValue)
				defer func() { _ = os.Unsetenv(tt.key) }()
			}

			got := getEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Errorf("getEnv() = %v, want %v", got, tt.want)
			}
		})
	}
}
