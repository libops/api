package config

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"
)

// TokenChangeCallback is called when the Vault token changes
type TokenChangeCallback func(newToken string)

// Reloader watches for configuration changes and reloads config in memory
type Reloader struct {
	config            atomic.Pointer[Config]
	loader            *VaultLoader
	watcher           *fsnotify.Watcher
	watchedFiles      map[string]bool
	stopCh            chan struct{}
	tokenChangeCallbacks []TokenChangeCallback
}

// NewReloader creates a new config reloader
func NewReloader(initialConfig *Config, loader *VaultLoader) (*Reloader, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	r := &Reloader{
		loader:               loader,
		watcher:              watcher,
		watchedFiles:         make(map[string]bool),
		stopCh:               make(chan struct{}),
		tokenChangeCallbacks: make([]TokenChangeCallback, 0),
	}
	r.config.Store(initialConfig)

	return r, nil
}

// GetConfig returns the current configuration atomically
func (r *Reloader) GetConfig() *Config {
	return r.config.Load()
}

// OnTokenChange registers a callback to be called when the Vault token changes
func (r *Reloader) OnTokenChange(callback TokenChangeCallback) {
	r.tokenChangeCallbacks = append(r.tokenChangeCallbacks, callback)
}

// Start begins watching for configuration changes
func (r *Reloader) Start(ctx context.Context) error {
	// Watch the vault secrets directory
	if err := r.watcher.Add(r.loader.secretsDir); err != nil {
		return fmt.Errorf("failed to watch secrets directory: %w", err)
	}

	go r.watchLoop(ctx)
	slog.Info("Config reloader started", "secrets_dir", r.loader.secretsDir)
	return nil
}

// Stop stops watching for configuration changes
func (r *Reloader) Stop() error {
	close(r.stopCh)
	return r.watcher.Close()
}

// watchLoop processes file system events
func (r *Reloader) watchLoop(ctx context.Context) {
	// Debounce rapid file changes (Vault Agent may write multiple times)
	debounceTimer := time.NewTimer(0)
	<-debounceTimer.C // drain initial timer
	needsReload := false

	defer debounceTimer.Stop() // Prevent timer leak

	for {
		select {
		case <-ctx.Done():
			return
		case <-r.stopCh:
			return
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}

			// Watch for Write and Create events on secret files
			if event.Op&(fsnotify.Write|fsnotify.Create) != 0 {
				slog.Debug("Config file changed", "file", event.Name, "op", event.Op)
				needsReload = true
				debounceTimer.Reset(500 * time.Millisecond)
			}

		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			slog.Error("File watcher error", "error", err)

		case <-debounceTimer.C:
			if needsReload {
				if err := r.reload(); err != nil {
					slog.Error("Failed to reload configuration", "error", err)
				}
				needsReload = false
			}
		}
	}
}

// reload reloads the configuration from vault/environment
func (r *Reloader) reload() error {
	slog.Info("Reloading configuration")

	newConfig, err := Load()
	if err != nil {
		return fmt.Errorf("failed to load new config: %w", err)
	}

	// Atomically swap the config
	oldConfig := r.config.Swap(newConfig)

	// Log changes
	r.logChanges(oldConfig, newConfig)

	slog.Info("Configuration reloaded successfully")
	return nil
}

// logChanges logs what changed between old and new config
func (r *Reloader) logChanges(old, new *Config) {
	if old.Port != new.Port {
		slog.Info("Config changed", "key", "PORT", "old", old.Port, "new", new.Port)
	}
	if old.DatabaseURL != new.DatabaseURL {
		slog.Info("Config changed", "key", "DATABASE_URL")
	}
	if old.APIBaseURL != new.APIBaseURL {
		slog.Info("Config changed", "key", "API_BASE_URL", "old", old.APIBaseURL, "new", new.APIBaseURL)
	}
	if old.GCPProjectID != new.GCPProjectID {
		slog.Info("Config changed", "key", "GCP_PROJECT_ID", "old", old.GCPProjectID, "new", new.GCPProjectID)
	}
	if old.OIDCClientSecret != new.OIDCClientSecret {
		slog.Info("Config changed", "key", "OIDC_CLIENT_SECRET")
	}
	if old.VaultToken != new.VaultToken {
		slog.Info("Config changed", "key", "VAULT_TOKEN")
		// Notify all registered callbacks about the token change
		for _, callback := range r.tokenChangeCallbacks {
			callback(new.VaultToken)
		}
	}
}
