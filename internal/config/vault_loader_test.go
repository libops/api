package config

import (
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"
)

// syncTestTime implements timeSource using synctest's time package
type syncTestTime struct{}

func (syncTestTime) Now() time.Time                         { return time.Now() }
func (syncTestTime) After(d time.Duration) <-chan time.Time { return time.After(d) }

func TestVaultLoader_LoadEnv_FromEnvironment(t *testing.T) {
	// Set an environment variable
	key := "TEST_VAR_ENV"
	expectedValue := "from-env"
	os.Setenv(key, expectedValue)
	defer os.Unsetenv(key)

	loader := NewVaultLoader()
	value, err := loader.LoadEnv(key, false)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if value != expectedValue {
		t.Errorf("expected %q, got %q", expectedValue, value)
	}
}

func TestVaultLoader_LoadEnv_FromVault(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Create a temporary directory for secrets
		tmpDir := t.TempDir()
		loader := &VaultLoader{
			secretsDir: tmpDir,
			timeout:    5 * time.Second,
			timeSource: syncTestTime{},
		}

		// Write a secret file
		key := "TEST_VAR_VAULT"
		expectedValue := "from-vault"
		secretPath := filepath.Join(tmpDir, key)
		if err := os.WriteFile(secretPath, []byte(expectedValue), 0600); err != nil {
			t.Fatalf("failed to write secret file: %v", err)
		}

		value, err := loader.LoadEnv(key, false)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if value != expectedValue {
			t.Errorf("expected %q, got %q", expectedValue, value)
		}
	})
}

func TestVaultLoader_LoadEnv_OptionalNotFound(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tmpDir := t.TempDir()
		loader := &VaultLoader{
			secretsDir: tmpDir,
			timeout:    5 * time.Second,
			timeSource: syncTestTime{},
		}

		key := "NONEXISTENT_VAR"
		value, err := loader.LoadEnv(key, false)

		if err != nil {
			t.Fatalf("unexpected error for optional var: %v", err)
		}
		if value != "" {
			t.Errorf("expected empty string, got %q", value)
		}
	})
}

func TestVaultLoader_LoadEnv_RequiredTimeout(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tmpDir := t.TempDir()

		// Use synctest's time package which works with fake clock
		loader := &VaultLoader{
			secretsDir: tmpDir,
			timeout:    2 * time.Second,
			timeSource: syncTestTime{},
		}

		key := "REQUIRED_VAR_TIMEOUT"
		start := time.Now()
		_, err := loader.LoadEnv(key, true)
		elapsed := time.Since(start)

		if err == nil {
			t.Fatal("expected timeout error, got nil")
		}

		// In synctest, time should advance to at least the timeout duration
		if elapsed < 2*time.Second {
			t.Errorf("expected elapsed time to be at least 2s, got %v", elapsed)
		}
	})
}

func TestVaultLoader_LoadEnv_RequiredAppears(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tmpDir := t.TempDir()
		loader := &VaultLoader{
			secretsDir: tmpDir,
			timeout:    5 * time.Second,
			timeSource: syncTestTime{},
		}

		key := "REQUIRED_VAR_APPEARS"
		expectedValue := "appeared"
		secretPath := filepath.Join(tmpDir, key)

		// Write the secret file after a delay in a goroutine
		go func() {
			time.Sleep(1 * time.Second)
			if err := os.WriteFile(secretPath, []byte(expectedValue), 0600); err != nil {
				t.Errorf("failed to write secret file in goroutine: %v", err)
			}
		}()

		start := time.Now()
		value, err := loader.LoadEnv(key, true)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if value != expectedValue {
			t.Errorf("expected %q, got %q", expectedValue, value)
		}

		// In synctest, time advances precisely
		// Should be 1s sleep + up to 2s poll = at least 1s, at most 3s
		if elapsed < 1*time.Second {
			t.Errorf("expected to wait at least 1s, waited %v", elapsed)
		}
		if elapsed > 3*time.Second {
			t.Errorf("expected to wait at most 3s, waited %v", elapsed)
		}
	})
}

func TestVaultLoader_LoadEnvWithDefault(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tmpDir := t.TempDir()
		loader := &VaultLoader{
			secretsDir: tmpDir,
			timeout:    5 * time.Second,
			timeSource: syncTestTime{},
		}

		key := "VAR_WITH_DEFAULT"
		defaultValue := "default-value"

		value := loader.LoadEnvWithDefault(key, defaultValue)
		if value != defaultValue {
			t.Errorf("expected default %q, got %q", defaultValue, value)
		}
	})
}

func TestVaultLoader_MustLoadEnv_Success(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tmpDir := t.TempDir()
		loader := &VaultLoader{
			secretsDir: tmpDir,
			timeout:    5 * time.Second,
			timeSource: syncTestTime{},
		}

		key := "MUST_LOAD_VAR"
		expectedValue := "must-have"
		secretPath := filepath.Join(tmpDir, key)
		if err := os.WriteFile(secretPath, []byte(expectedValue), 0600); err != nil {
			t.Fatalf("failed to write secret file: %v", err)
		}

		value := loader.MustLoadEnv(key)
		if value != expectedValue {
			t.Errorf("expected %q, got %q", expectedValue, value)
		}
	})
}

func TestVaultLoader_MustLoadEnv_Panic(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		tmpDir := t.TempDir()
		loader := &VaultLoader{
			secretsDir: tmpDir,
			timeout:    1 * time.Second, // Short timeout
			timeSource: syncTestTime{},
		}

		defer func() {
			if r := recover(); r == nil {
				t.Error("expected panic, got none")
			}
		}()

		loader.MustLoadEnv("NONEXISTENT_REQUIRED_VAR")
	})
}
