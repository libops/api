package config

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// VaultSecretsDir is the default mount path for Vault secrets
	VaultSecretsDir = "/vault/secrets"
	// DefaultTimeout is the default timeout for waiting for secrets
	DefaultTimeout = 120 * time.Second
	// PollInterval is how often to check for secret files
	PollInterval = 2 * time.Second
)

// timeSource is an interface for getting current time and creating tickers
// This allows us to inject fake time in tests
type timeSource interface {
	Now() time.Time
	After(d time.Duration) <-chan time.Time
}

// realTime implements timeSource using the real time package
type realTime struct{}

func (realTime) Now() time.Time                         { return time.Now() }
func (realTime) After(d time.Duration) <-chan time.Time { return time.After(d) }

// VaultLoader loads environment variables from Vault secret mount
type VaultLoader struct {
	secretsDir string
	timeout    time.Duration
	timeSource timeSource
}

// NewVaultLoader creates a new vault loader
func NewVaultLoader() *VaultLoader {
	secretsDir := os.Getenv("VAULT_SECRETS_DIR")
	if secretsDir == "" {
		secretsDir = VaultSecretsDir
	}

	return &VaultLoader{
		secretsDir: secretsDir,
		timeout:    DefaultTimeout,
		timeSource: realTime{},
	}
}

// LoadEnv loads an environment variable from Vault or environment
// Priority: 1) Current environment, 2) Vault secrets file
// If required=true and not found, waits up to timeout for the secret to appear
func (v *VaultLoader) LoadEnv(key string, required bool) (string, error) {
	// Check if already set in environment
	if value := os.Getenv(key); value != "" {
		slog.Debug("Using environment variable", "key", key)
		return value, nil
	}

	secretPath := filepath.Join(v.secretsDir, key)
	if !required {
		if value, err := v.readSecretFile(secretPath); err == nil && value != "" {
			slog.Debug("Loaded optional variable from Vault", "key", key)
			return value, nil
		}
		slog.Debug("Optional variable not found", "key", key)
		return "", nil
	}

	// Required variable - wait with timeout
	slog.Info("Waiting for required variable", "key", key, "timeout", v.timeout)
	return v.waitForSecret(key, secretPath)
}

// readSecretFile reads a secret from the filesystem
func (v *VaultLoader) readSecretFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	// Trim whitespace and newlines from secrets
	// This is important because vault-agent and other tools often write trailing newlines
	value := string(data)
	return trimSecret(value), nil
}

// trimSecret removes leading/trailing whitespace from secrets
func trimSecret(s string) string {
	// Remove common whitespace characters that can appear in secret files
	return strings.TrimSpace(s)
}

// waitForSecret waits for a secret file to appear, up to the timeout
func (v *VaultLoader) waitForSecret(key, path string) (string, error) {
	deadline := v.timeSource.Now().Add(v.timeout)

	for {
		// Check if secret exists
		value, err := v.readSecretFile(path)
		if err == nil && value != "" {
			elapsed := v.timeout - time.Until(deadline)
			slog.Info("Loaded required variable from Vault",
				"key", key,
				"elapsed", elapsed.Round(time.Second))
			return value, nil
		}

		// Check if we've exceeded the timeout
		if v.timeSource.Now().After(deadline) {
			return "", fmt.Errorf("timeout waiting for required variable %s after %v", key, v.timeout)
		}

		// Wait for poll interval
		<-v.timeSource.After(PollInterval)
	}
}

// MustLoadEnv loads a required environment variable or panics
func (v *VaultLoader) MustLoadEnv(key string) string {
	value, err := v.LoadEnv(key, true)
	if err != nil {
		panic(fmt.Sprintf("failed to load required config %s: %v", key, err))
	}
	return value
}

// LoadEnvWithDefault loads an environment variable with a default fallback
func (v *VaultLoader) LoadEnvWithDefault(key, defaultValue string) string {
	value, err := v.LoadEnv(key, false)
	if err != nil || value == "" {
		return defaultValue
	}
	return value
}
