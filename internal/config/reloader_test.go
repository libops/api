package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestReloader_ConfigUpdate tests that the Reloader detects and applies config changes
func TestReloader_ConfigUpdate(t *testing.T) {
	// Ensure OIDC_CLIENT_SECRET is not in environment
	_ = os.Unsetenv("OIDC_CLIENT_SECRET")
	dbPath := "/tmp/foo-reloader-test"
	_ = os.Setenv("MARIADB_PASSWORD_FILE", dbPath)
	if err := os.WriteFile(dbPath, []byte("bar"), 0600); err != nil {
		t.Fatalf("failed to write MARIADB_PASSWORD_FILE: %v", err)
	}
	defer func() { _ = os.Remove(dbPath) }()

	// Set up vault secrets directory
	tmpDir := t.TempDir()
	_ = os.Setenv("VAULT_SECRETS_DIR", tmpDir)
	defer func() { _ = os.Unsetenv("VAULT_SECRETS_DIR") }()

	expectedOidcId := "foo"
	expectedOIDCSecret := "vault-oidc-secret"
	initialVaultToken := "vault-token-value"

	// Write initial secrets
	oidcPath := filepath.Join(tmpDir, "OIDC_CLIENT_SECRET")
	if err := os.WriteFile(oidcPath, []byte(expectedOIDCSecret), 0600); err != nil {
		t.Fatalf("failed to write OIDC_CLIENT_SECRET: %v", err)
	}
	oidcPath = filepath.Join(tmpDir, "OIDC_CLIENT_ID")
	if err := os.WriteFile(oidcPath, []byte(expectedOidcId), 0600); err != nil {
		t.Fatalf("failed to write OIDC_CLIENT_ID: %v", err)
	}
	vaultPath := filepath.Join(tmpDir, "VAULT_TOKEN")
	if err := os.WriteFile(vaultPath, []byte(initialVaultToken), 0600); err != nil {
		t.Fatalf("failed to write VAULT_TOKEN: %v", err)
	}

	// Load initial config
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Create reloader with the vault loader
	loader := NewVaultLoader()
	reloader, err := NewReloader(cfg, loader)
	if err != nil {
		t.Fatalf("failed to create reloader: %v", err)
	}
	defer func() { _ = reloader.Stop() }()

	// Start the reloader
	ctx := context.Background()
	if err := reloader.Start(ctx); err != nil {
		t.Fatalf("failed to start reloader: %v", err)
	}

	// Verify initial config
	currentCfg := reloader.GetConfig()
	if currentCfg.VaultToken != initialVaultToken {
		t.Errorf("expected initial VAULT_TOKEN %q, got %q", initialVaultToken, currentCfg.VaultToken)
	}

	// Update the vault token
	updatedToken := "baz"
	if err := os.WriteFile(vaultPath, []byte(updatedToken), 0600); err != nil {
		t.Fatalf("failed to write updated VAULT_TOKEN: %v", err)
	}

	// Wait for the reloader to detect and apply the change
	// The reloader has a 500ms debounce, so we wait a bit longer
	time.Sleep(1 * time.Second)

	// Verify the config was updated
	currentCfg = reloader.GetConfig()
	if currentCfg.VaultToken != updatedToken {
		t.Errorf("expected VAULT_TOKEN update to %q, got %q", updatedToken, currentCfg.VaultToken)
	}
}
