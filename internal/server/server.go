// Package server sets up and manages the main HTTP API server.
package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/libops/api/db"
	"github.com/libops/api/internal/audit"
	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/config"
	"github.com/libops/api/internal/dash"
	"github.com/libops/api/internal/database"
	"github.com/libops/api/internal/events"
	"github.com/libops/api/internal/router"
	"github.com/libops/api/internal/vault"
)

// Server represents the API server with all its dependencies.
type Server struct {
	config        *config.Config
	reloader      *config.Reloader
	httpServer    *http.Server
	dbPool        *sql.DB
	emailVerifier *auth.EmailVerifier
	vaultClient   *vault.Client
	cleanupTicker *time.Ticker
	cleanupDone   chan bool
}

// findTemplatesDir searches for the templates directory starting from the current directory
// and walking up to find the project root
func findTemplatesDir(startPath string) (string, error) {
	// First check if TEMPLATES_DIR environment variable is set
	if envDir := os.Getenv("TEMPLATES_DIR"); envDir != "" {
		if _, err := os.Stat(filepath.Join(envDir, "base.html")); err == nil {
			return envDir, nil
		}
	}

	// Try the provided path as-is
	if _, err := os.Stat(filepath.Join(startPath, "base.html")); err == nil {
		return startPath, nil
	}

	// Walk up the directory tree to find web/templates
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}

	for {
		templatesPath := filepath.Join(dir, "web", "templates")
		if _, err := os.Stat(filepath.Join(templatesPath, "base.html")); err == nil {
			return templatesPath, nil
		}

		// Move up one directory
		parent := filepath.Dir(dir)
		if parent == dir {
			// Reached root without finding templates
			break
		}
		dir = parent
	}

	return "", fmt.Errorf("templates directory not found")
}

// New creates a new Server instance with all dependencies initialized.
func New(reloader *config.Reloader) (*Server, error) {
	cfg := reloader.GetConfig()

	// Initialize templates from web/templates
	templatesDir, err := findTemplatesDir("web/templates")
	if err != nil {
		slog.Error("Failed to find templates directory", "err", err)
		return nil, fmt.Errorf("failed to find templates: %w", err)
	}
	if err := dash.InitTemplates(templatesDir); err != nil {
		slog.Error("Failed to load templates", "err", err, "dir", templatesDir)
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	dbPool, err := database.NewPool(cfg.DatabaseURL, database.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create database pool: %w", err)
	}
	slog.Info("Database connection pool established")

	// Run database migrations (same in dev and prod)
	slog.Info("Running database migrations")
	if err := database.Migrate(dbPool); err != nil {
		return nil, fmt.Errorf("failed to run migrations: %w", err)
	}
	slog.Info("Database migrations completed successfully")

	queries := db.New(dbPool)

	jwtValidator, libopsTokenIssuer, apiKeyManager, authHandler, authorizer, emailVerifier, userpassClient, sessionManager, vaultClient, err := setupAuth(cfg, queries)
	if err != nil {
		return nil, fmt.Errorf("failed to setup auth: %w", err)
	}

	emitter := setupEvents(queries)

	routerDeps := &router.Dependencies{
		Config:            cfg,
		Queries:           queries,
		Emitter:           emitter,
		Authorizer:        authorizer,
		JWTValidator:      jwtValidator,
		LibopsTokenIssuer: libopsTokenIssuer,
		APIKeyManager:     apiKeyManager,
		AuthHandler:       authHandler,
		UserpassClient:    userpassClient,
		SessionManager:    sessionManager,
		AllowedOrigins:    cfg.AllowedOrigins,
	}
	handler := router.New(routerDeps)

	httpServer := &http.Server{
		Addr:         fmt.Sprintf(":%s", cfg.Port),
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	server := &Server{
		config:        cfg,
		reloader:      reloader,
		httpServer:    httpServer,
		dbPool:        dbPool,
		emailVerifier: emailVerifier,
		vaultClient:   vaultClient,
		cleanupDone:   make(chan bool),
	}

	// Register callback to update Vault token when config changes
	reloader.OnTokenChange(func(newToken string) {
		slog.Info("Updating Vault client token after config reload")
		vaultClient.SetToken(newToken)
	})

	return server, nil
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	// Start the config reloader
	if err := s.reloader.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start config reloader: %w", err)
	}

	if s.emailVerifier != nil {
		s.cleanupTicker = time.NewTicker(1 * time.Hour)
		go func() {
			for {
				select {
				case <-s.cleanupTicker.C:
					ctx := context.Background()
					if err := s.emailVerifier.CleanupExpiredTokens(ctx); err != nil {
						slog.Error("failed to cleanup expired verification tokens", "err", err)
					} else {
						slog.Debug("cleaned up expired verification tokens")
					}
				case <-s.cleanupDone:
					return
				}
			}
		}()
		slog.Info("Email verification cleanup job started (runs every 1 hour)")
	}

	slog.Info("Starting LibOps API v1 (ConnectRPC)", "addr", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server.
func (s *Server) Shutdown(ctx context.Context) error {
	slog.Info("Starting graceful shutdown")

	// Stop the config reloader
	if err := s.reloader.Stop(); err != nil {
		slog.Error("Error stopping config reloader", "error", err)
	} else {
		slog.Info("Config reloader stopped")
	}

	if s.cleanupTicker != nil {
		s.cleanupTicker.Stop()
		close(s.cleanupDone)
		slog.Info("Stopped email verification cleanup job")
	}

	if err := s.httpServer.Shutdown(ctx); err != nil {
		_ = s.httpServer.Close()
		return fmt.Errorf("could not stop server gracefully: %w", err)
	}

	if err := s.dbPool.Close(); err != nil {
		return fmt.Errorf("error closing database: %w", err)
	}

	slog.Info("Server stopped gracefully")
	return nil
}

// setupAuth initializes authentication components.
func setupAuth(cfg *config.Config, queries db.Querier) (
	*auth.VaultJWTValidator,
	*auth.LibopsTokenIssuer,
	*auth.APIKeyManager,
	*auth.Handler,
	*auth.Authorizer,
	*auth.EmailVerifier,
	*auth.UserpassClient,
	*auth.SessionManager,
	*vault.Client,
	error,
) {
	vaultClient, err := vault.NewClient(&vault.Config{
		Address: cfg.VaultAddr,
		Token:   cfg.VaultToken,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize vault client: %w", err)
	}

	// JWT validator uses APIBaseURL (not VaultAddr) to fetch JWKS via Traefik
	// This ensures consistency with browser-facing OIDC endpoints
	jwtValidator := auth.NewJWTValidator(cfg.VaultAddr, cfg.VaultOIDCProvider)

	if err := jwtValidator.Initialize(context.Background()); err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize JWT validator: %w", err)
	}

	auditLogger := audit.New(queries)

	// Initialize session manager
	// In development mode, disable secure cookies to allow HTTP connections
	secureCookies := os.Getenv("LIBOPS_ENV") != "development"
	sessionManager := auth.NewSessionManager(queries, "", secureCookies)

	// Initialize unified token issuer
	libopsTokenIssuer := auth.NewLibopsTokenIssuer(vaultClient, queries, sessionManager, cfg.VaultAddr, cfg.VaultOIDCProvider, auditLogger)

	apiKeyManager := auth.NewAPIKeyManager(vaultClient, queries, auditLogger)

	jwtValidator.SetAPIKeyManager(apiKeyManager)

	emailVerifier := auth.NewEmailVerifier(queries, nil, cfg.APIBaseURL) // nil = no email sender (dev mode)

	userpassClient := auth.NewUserpassClient(vaultClient, "userpass", queries, emailVerifier)

	authorizer := auth.NewAuthorizer(queries)

	// Initialize Goth OAuth manager (if configured)
	var gothManager *auth.GothOAuthManager
	if cfg.GoogleClientID != "" || cfg.GitHubClientID != "" {
		var googleCfg, githubCfg *auth.ProviderConfig

		if cfg.GoogleClientID != "" {
			googleCfg = &auth.ProviderConfig{
				ClientID:     cfg.GoogleClientID,
				ClientSecret: cfg.GoogleClientSecret,
				CallbackURL:  cfg.GoogleCallbackURL,
			}
		}

		if cfg.GitHubClientID != "" {
			githubCfg = &auth.ProviderConfig{
				ClientID:     cfg.GitHubClientID,
				ClientSecret: cfg.GitHubClientSecret,
				CallbackURL:  cfg.GitHubCallbackURL,
			}
		}

		var err error
		gothManager, err = auth.NewGothOAuthManager(googleCfg, githubCfg, queries)
		if err != nil {
			slog.Warn("Failed to initialize Goth OAuth manager", "error", err)
		} else {
			slog.Info("Goth OAuth manager initialized",
				"google_configured", googleCfg != nil,
				"github_configured", githubCfg != nil)
		}
	}

	// Initialize auth handler
	authHandler := auth.NewHandler(userpassClient, jwtValidator, sessionManager, queries, vaultClient, cfg.VaultOIDCProvider, gothManager, libopsTokenIssuer)

	slog.Info("Authentication enabled",
		"vault", cfg.VaultAddr,
		"provider", cfg.VaultOIDCProvider,
		"token_len", len(cfg.VaultToken))

	return jwtValidator, libopsTokenIssuer, apiKeyManager, authHandler, authorizer, emailVerifier, userpassClient, sessionManager, vaultClient, nil
}

// setupEvents initializes event emitter.
// Events are written to the event_queue table and processed by the orchestrator.
func setupEvents(queries db.Querier) *events.Emitter {
	slog.Info("Event emitter configured to use database queue")
	emitter := events.NewEmitter(queries, events.EventSourceLibOpsAPI)
	return emitter
}
