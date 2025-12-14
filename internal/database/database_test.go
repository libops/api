package database

import (
	"testing"
	"time"
)

// TestDefaultConfig tests the DefaultConfig function to ensure it returns a valid default configuration.
func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg == nil {
		t.Fatal("DefaultConfig() returned nil")
	}

	if cfg.MaxOpenConns <= 0 {
		t.Error("MaxOpenConns should be positive")
	}

	if cfg.MaxIdleConns <= 0 {
		t.Error("MaxIdleConns should be positive")
	}

	if cfg.ConnMaxLifetime <= 0 {
		t.Error("ConnMaxLifetime should be positive")
	}

	if cfg.ConnMaxIdleTime <= 0 {
		t.Error("ConnMaxIdleTime should be positive")
	}

	if cfg.PingTimeout <= 0 {
		t.Error("PingTimeout should be positive")
	}
}

// TestNewPool_InvalidConnectionString verifies that NewPool returns an error when provided with an invalid database connection string.
func TestNewPool_InvalidConnectionString(t *testing.T) {
	cfg := &Config{
		MaxOpenConns:    10,
		MaxIdleConns:    5,
		ConnMaxLifetime: 5 * time.Minute,
		ConnMaxIdleTime: 1 * time.Minute,
		PingTimeout:     1 * time.Second,
	}

	_, err := NewPool("invalid-connection-string", cfg)
	if err == nil {
		t.Error("NewPool() should fail with invalid connection string")
	}
}

// TestNewPool_NilConfig verifies that NewPool handles a nil Config gracefully by using a default configuration.
func TestNewPool_NilConfig(t *testing.T) {
	// NewPool should use default config when nil is passed
	// This will fail because we don't have a real database, but it tests that
	// nil config handling works
	_, err := NewPool("user:pass@tcp(nonexistent:3306)/db", nil)
	// We expect an error due to connection failure, not a panic from nil config
	if err == nil {
		t.Error("Expected connection error")
	}
}
