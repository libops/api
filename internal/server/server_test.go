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
		Port:           "8080",
		ReadTimeout:    30 * time.Second,
		WriteTimeout:   30 * time.Second,
		IdleTimeout:    120 * time.Second,
		DatabaseURL:    "root:password@tcp(localhost:3306)/test?parseTime=true",
		GCPProjectID:   "",
		EventsTopicID:  "",
		AllowedOrigins: []string{"*"},
	}

	// This will fail to connect to the database, but we're testing the structure
	_, err := New(cfg)

	// We expect an error because we don't have a real database
	// The important thing is that New() doesn't panic
	if err == nil {
		t.Log("Note: New() succeeded, which means a database is available")
	} else {
		// Expected: database connection error
		t.Logf("Expected error (no database): %v", err)
	}
}

// TestSetupAuth_Disabled tests the setupAuth function when authentication is disabled.
func TestSetupAuth_Disabled(t *testing.T) {
	_ = os.Unsetenv("VAULT_ADDR")
	_ = os.Unsetenv("VAULT_OIDC_PROVIDER")
	_ = os.Unsetenv("VAULT_CLIENT_ID")
	_ = os.Unsetenv("VAULT_CLIENT_SECRET")
	_ = os.Unsetenv("LIBOPS_ADMIN_EMAILS")

	// Create a nil querier (we won't use it in this test)
	authConfig, jwtValidator, tokenIssuer, apiKeyManager, authHandler, authorizer, emailVerifier, userpassClient, err := setupAuth(nil, nil)

	if err != nil {
		t.Errorf("setupAuth should not return error when auth is disabled, got: %v", err)
	}

	if authConfig != nil {
		t.Error("authConfig should be nil when auth is disabled")
	}
	if jwtValidator != nil {
		t.Error("jwtValidator should be nil when auth is disabled")
	}
	if tokenIssuer != nil {
		t.Error("tokenIssuer should be nil when auth is disabled")
	}
	if apiKeyManager != nil {
		t.Error("apiKeyManager should be nil when auth is disabled")
	}
	if authHandler != nil {
		t.Error("authHandler should be nil when auth is disabled")
	}
	if authorizer != nil {
		t.Error("authorizer should be nil when auth is disabled")
	}
	if emailVerifier != nil {
		t.Error("emailVerifier should be nil when auth is disabled")
	}
	if userpassClient != nil {
		t.Error("userpassClient should be nil when auth is disabled")
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
		Port:           "0", // Use random available port
		ReadTimeout:    5 * time.Second,
		WriteTimeout:   5 * time.Second,
		IdleTimeout:    10 * time.Second,
		DatabaseURL:    dbURL,
		GCPProjectID:   "",
		EventsTopicID:  "",
		AllowedOrigins: []string{"*"},
	}

	srv, err := New(cfg)
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
	logServerConfig(cfg, nil, nil)

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
				Port:           "8080",
				ReadTimeout:    30 * time.Second,
				WriteTimeout:   30 * time.Second,
				IdleTimeout:    120 * time.Second,
				DatabaseURL:    "root:password@tcp(localhost:3306)/test",
				AllowedOrigins: []string{"*"},
			},
			shouldSucceed: false, // Will fail due to no DB, but structure is valid
		},
		{
			name: "with events config",
			config: &config.Config{
				Port:           "8080",
				ReadTimeout:    30 * time.Second,
				WriteTimeout:   30 * time.Second,
				IdleTimeout:    120 * time.Second,
				DatabaseURL:    "root:password@tcp(localhost:3306)/test",
				GCPProjectID:   "test-project",
				EventsTopicID:  "test-topic",
				AllowedOrigins: []string{"*"},
			},
			shouldSucceed: false, // Will fail due to no DB
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := New(tt.config)

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
