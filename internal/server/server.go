// Package server sets up and manages the main HTTP API server.
package server

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/libops/api/internal/audit"
	"github.com/libops/api/internal/auth"
	"github.com/libops/api/internal/config"
	"github.com/libops/api/internal/dash"
	"github.com/libops/api/internal/database"
	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/events"
	"github.com/libops/api/internal/router"
	"github.com/libops/api/internal/vault"
)

// Server represents the API server with all its dependencies.
type Server struct {
	config         *config.Config
	reloader       *config.Reloader
	httpServer     *http.Server
	dbPool         *sql.DB
	queueProcessor *events.QueueProcessor
	emailVerifier  *auth.EmailVerifier
	cleanupTicker  *time.Ticker
	cleanupDone    chan bool
}

// New creates a new Server instance with all dependencies initialized.
func New(reloader *config.Reloader) (*Server, error) {
	cfg := reloader.GetConfig()

	// Initialize templates from web/templates
	templatesDir := os.Getenv("TEMPLATES_DIR")
	if templatesDir == "" {
		templatesDir = "web/templates"
	}
	if err := dash.InitTemplates(templatesDir); err != nil {
		slog.Error("Failed to load templates from web/templates", "err", err, "dir", templatesDir)
		return nil, fmt.Errorf("failed to load templates: %w", err)
	}

	dbPool, err := database.NewPool(cfg.DatabaseURL, database.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create database pool: %w", err)
	}
	slog.Info("Database connection pool established")

	queries := db.New(dbPool)

	jwtValidator, libopsTokenIssuer, apiKeyManager, authHandler, authorizer, emailVerifier, userpassClient, sessionManager, err := setupAuth(cfg, queries)
	if err != nil {
		return nil, fmt.Errorf("failed to setup auth: %w", err)
	}

	ceClient, emitter, queueProcessor := setupEvents(cfg, queries)

	routerDeps := &router.Dependencies{
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

	logServerConfig(cfg, ceClient)

	return &Server{
		config:         cfg,
		reloader:       reloader,
		httpServer:     httpServer,
		dbPool:         dbPool,
		queueProcessor: queueProcessor,
		emailVerifier:  emailVerifier,
		cleanupDone:    make(chan bool),
	}, nil
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
	// Start the config reloader
	if err := s.reloader.Start(context.Background()); err != nil {
		return fmt.Errorf("failed to start config reloader: %w", err)
	}

	go s.queueProcessor.Start(context.Background())

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

	slog.Info("Stopping queue processor")
	s.queueProcessor.Stop()

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
	error,
) {
	vaultClient, err := vault.NewClient(&vault.Config{
		Address: cfg.VaultAddr,
		Token:   cfg.VaultToken,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize vault client: %w", err)
	}

	// JWT validator uses APIBaseURL (not VaultAddr) to fetch JWKS via Traefik
	// This ensures consistency with browser-facing OIDC endpoints
	jwtValidator := auth.NewJWTValidator(cfg.VaultAddr, cfg.VaultOIDCProvider)

	if err := jwtValidator.Initialize(context.Background()); err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize JWT validator: %w", err)
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

	// Initialize OIDC client
	// Uses APIBaseURL (not VaultAddr) since browsers access OIDC endpoints via Traefik
	oidcClient, err := auth.NewOIDCClient(&auth.OIDCConfig{
		VaultAddr:         cfg.VaultAddr,
		VaultOIDCProvider: cfg.VaultOIDCProvider,
		ClientID:          cfg.OIDCClientID,
		ClientSecret:      cfg.OIDCClientSecret,
		RedirectURL:       cfg.OIDCRedirectURL,
		Scopes:            []string{"openid", "email", "profile"},
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize OIDC client: %w", err)
	}

	emailVerifier := auth.NewEmailVerifier(queries, nil, cfg.APIBaseURL) // nil = no email sender (dev mode)

	userpassClient := auth.NewUserpassClient(vaultClient, "userpass", queries, emailVerifier)

	authorizer := auth.NewAuthorizer(queries)

	// Initialize auth handler
	authHandler := auth.NewHandler(oidcClient, userpassClient, jwtValidator, sessionManager, queries, vaultClient, cfg.VaultOIDCProvider)

	slog.Info("Authentication enabled",
		"vault", cfg.VaultAddr,
		"provider", cfg.VaultOIDCProvider,
		"token_len", len(cfg.VaultToken))

	return jwtValidator, libopsTokenIssuer, apiKeyManager, authHandler, authorizer, emailVerifier, userpassClient, sessionManager, nil
}

// setupEvents initializes event emitter and queue processor.
func setupEvents(cfg *config.Config, queries db.Querier) (
	cloudevents.Client,
	*events.Emitter,
	*events.QueueProcessor,
) {
	var ceClient cloudevents.Client

	if cfg.GCPProjectID != "" && cfg.EventsTopicID != "" {
		sender, err := events.NewPubSubSender(context.Background(), cfg.GCPProjectID, cfg.EventsTopicID)
		if err != nil {
			slog.Warn("Failed to create Pub/Sub sender, events disabled", "error", err)
			ceClient = events.NewNoOpClient()
		} else {
			slog.Info("Event emitter configured with Pub/Sub",
				"project", cfg.GCPProjectID,
				"topic", cfg.EventsTopicID)
			ceClient = events.NewPubSubCloudEventsClient(sender)
		}
	} else {
		slog.Info("Event emitter disabled (no GCP_PROJECT_ID or EVENTS_TOPIC_ID)")
		ceClient = events.NewNoOpClient()
	}

	emitter := events.NewEmitter(ceClient, events.EventSourceLibOpsAPI, queries)

	hostname, _ := os.Hostname()
	instanceID := fmt.Sprintf("%s-%d", hostname, os.Getpid())
	queueProcessor := events.NewQueueProcessor(
		queries,
		ceClient,
		events.EventSourceLibOpsAPI,
		instanceID,
		events.DefaultQueueProcessorConfig(),
	)

	return ceClient, emitter, queueProcessor
}

// logServerConfig logs the server configuration at startup.
func logServerConfig(cfg *config.Config, ceClient cloudevents.Client) {
	slog.Info("Authorization interceptor enabled")
	slog.Info("Auth middleware enabled")
	slog.Info("Auth routes registered")
}
