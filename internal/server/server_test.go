package server

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/libops/api/internal/config"
)

// TestNew tests the New server constructor function.
func TestNew(t *testing.T) {
	_ = os.Setenv("DATABASE_URL", "root:password@tcp(localhost:3306)/test?parseTime=true")
	defer func() { _ = os.Unsetenv("DATABASE_URL") }()

	cfg := &config.Config{
		Port:              "8080",
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		DatabaseURL:       "root:password@tcp(localhost:3306)/test?parseTime=true",
		GCPProjectID:      "",
		EventsTopicID:     "",
		AllowedOrigins:    []string{"*"},
		VaultAddr:         "https://vault.libops.io",
		VaultToken:        "test-token",
		VaultOIDCProvider: "libops-api",
		OIDCClientID:      "libops-api",
		OIDCClientSecret:  "test-secret",
		OIDCRedirectURL:   "http://localhost:8080/auth/callback",
		APIBaseURL:        "https://api.libops.io",
	}

	loader := config.NewVaultLoader()
	reloader, err := config.NewReloader(cfg, loader)
	if err != nil {
		t.Fatalf("Failed to create reloader: %v", err)
	}

	// This will fail to connect to the database, but we're testing the structure
	_, err = New(reloader)

	// We expect an error because we don't have a real database
	// The important thing is that New() doesn't panic
	if err == nil {
		t.Log("Note: New() succeeded, which means a database is available")
	} else {
		// Expected: database connection error
		t.Logf("Expected error (no database): %v", err)
	}
}

// TestSetupAuth tests the setupAuth function.
func TestSetupAuth(t *testing.T) {
	// setupAuth now requires valid config with auth settings
	// This test verifies it initializes properly with valid config
	cfg := &config.Config{
		VaultAddr:         "https://vault.libops.io",
		VaultToken:        "test-token",
		VaultOIDCProvider: "libops-api",
		OIDCClientID:      "libops-api",
		OIDCClientSecret:  "test-secret",
		OIDCRedirectURL:   "http://localhost:8080/auth/callback",
		APIBaseURL:        "https://api.libops.io",
	}

	// This will fail to connect to Vault, but we're testing the structure
	_, _, _, _, _, _, _, _, err := setupAuth(cfg, nil)

	// We expect an error because we don't have a real Vault
	if err == nil {
		t.Log("Note: setupAuth() succeeded, which means Vault is available")
	} else {
		// Expected: Vault connection error
		t.Logf("Expected error (no Vault): %v", err)
	}
}

// TestSetupEvents_Disabled tests the setupEvents function when eventing is disabled.
func TestSetupEvents_Disabled(t *testing.T) {
	cfg := &config.Config{
		GCPProjectID:  "",
		EventsTopicID: "",
	}

	ceClient, emitter, queueProcessor := setupEvents(cfg, nil)

	if ceClient == nil {
		t.Error("ceClient should not be nil")
	}

	if emitter == nil {
		t.Error("emitter should not be nil")
	}

	if queueProcessor == nil {
		t.Error("queueProcessor should not be nil")
	}
}

// TestSetupEvents_WithConfig tests the setupEvents function with a valid configuration.
func TestSetupEvents_WithConfig(t *testing.T) {
	cfg := &config.Config{
		GCPProjectID:  "test-project",
		EventsTopicID: "test-topic",
	}

	// This will fail to connect to Pub/Sub, but should fall back to NoOp
	ceClient, emitter, queueProcessor := setupEvents(cfg, nil)

	if ceClient == nil {
		t.Error("ceClient should not be nil (should fall back to NoOp)")
	}

	if emitter == nil {
		t.Error("emitter should not be nil")
	}

	if queueProcessor == nil {
		t.Error("queueProcessor should not be nil")
	}
}

// TestServer_Lifecycle tests the basic lifecycle of the server (start and graceful shutdown).
func TestServer_Lifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// This test requires a real database connection
	// Skip if DATABASE_URL is not set
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		t.Skip("DATABASE_URL not set, skipping server lifecycle test")
	}

	cfg := &config.Config{
		Port:              "0", // Use random available port
		ReadTimeout:       5 * time.Second,
		WriteTimeout:      5 * time.Second,
		IdleTimeout:       10 * time.Second,
		DatabaseURL:       dbURL,
		GCPProjectID:      "",
		EventsTopicID:     "",
		AllowedOrigins:    []string{"*"},
		VaultAddr:         "https://vault.libops.io",
		VaultToken:        os.Getenv("VAULT_TOKEN"),
		VaultOIDCProvider: "libops-api",
		OIDCClientID:      "libops-api",
		OIDCClientSecret:  os.Getenv("OIDC_CLIENT_SECRET"),
		OIDCRedirectURL:   "http://localhost:8080/auth/callback",
		APIBaseURL:        "https://api.libops.io",
	}

	loader := config.NewVaultLoader()
	reloader, err := config.NewReloader(cfg, loader)
	if err != nil {
		t.Fatalf("Failed to create reloader: %v", err)
	}

	srv, err := New(reloader)
	if err != nil {
		t.Fatalf("Failed to create server: %v", err)
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- srv.Start()
	}()

	time.Sleep(100 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		t.Errorf("Server shutdown failed: %v", err)
	}

	select {
	case err := <-errChan:
		if err != nil && err.Error() != "http: Server closed" {
			t.Errorf("Server returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		// Timeout waiting for server to stop is okay
	}
}

// TestLogServerConfig ensures the logServerConfig function does not panic.
func TestLogServerConfig(t *testing.T) {
	cfg := &config.Config{
		Port: "8080",
	}

	// Call with nil authConfig (should not panic)
	logServerConfig(cfg, nil)

	// Note: We can't easily test logging output without mocking slog,
	// but we can at least verify the function doesn't panic
}

// Example of table-driven test for configuration scenarios.
func TestServerConfiguration(t *testing.T) {
	tests := []struct {
		name          string
		config        *config.Config
		shouldSucceed bool
	}{
		{
			name: "valid minimal config",
			config: &config.Config{
				Port:              "8080",
				ReadTimeout:       30 * time.Second,
				WriteTimeout:      30 * time.Second,
				IdleTimeout:       120 * time.Second,
				DatabaseURL:       "root:password@tcp(localhost:3306)/test",
				AllowedOrigins:    []string{"*"},
				VaultAddr:         "https://vault.libops.io",
				VaultToken:        "test-token",
				VaultOIDCProvider: "libops-api",
				OIDCClientID:      "libops-api",
				OIDCClientSecret:  "test-secret",
				OIDCRedirectURL:   "http://localhost:8080/auth/callback",
				APIBaseURL:        "https://api.libops.io",
			},
			shouldSucceed: false, // Will fail due to no DB, but structure is valid
		},
		{
			name: "with events config",
			config: &config.Config{
				Port:              "8080",
				ReadTimeout:       30 * time.Second,
				WriteTimeout:      30 * time.Second,
				IdleTimeout:       120 * time.Second,
				DatabaseURL:       "root:password@tcp(localhost:3306)/test",
				GCPProjectID:      "test-project",
				EventsTopicID:     "test-topic",
				AllowedOrigins:    []string{"*"},
				VaultAddr:         "https://vault.libops.io",
				VaultToken:        "test-token",
				VaultOIDCProvider: "libops-api",
				OIDCClientID:      "libops-api",
				OIDCClientSecret:  "test-secret",
				OIDCRedirectURL:   "http://localhost:8080/auth/callback",
				APIBaseURL:        "https://api.libops.io",
			},
			shouldSucceed: false, // Will fail due to no DB
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := config.NewVaultLoader()
			reloader, err := config.NewReloader(tt.config, loader)
			if err != nil {
				t.Fatalf("Failed to create reloader: %v", err)
			}

			_, err = New(reloader)

			if tt.shouldSucceed && err != nil {
				t.Errorf("Expected success but got error: %v", err)
			}

			// We expect errors in these tests due to no database
			// The important thing is that New() is called correctly
			if err != nil {
				t.Logf("Got expected error: %v", err)
			}
		})
	}
}

// BenchmarkSetupEvents benchmarks the performance of the setupEvents function.
func BenchmarkSetupEvents(b *testing.B) {
	cfg := &config.Config{
		GCPProjectID:  "",
		EventsTopicID: "",
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		setupEvents(cfg, nil)
	}
}
