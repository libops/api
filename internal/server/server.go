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
	"github.com/libops/api/internal/database"
	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/events"
	"github.com/libops/api/internal/router"
	"github.com/libops/api/internal/vault"
)

// Server represents the API server with all its dependencies.
type Server struct {
	config         *config.Config
	httpServer     *http.Server
	dbPool         *sql.DB
	queueProcessor *events.QueueProcessor
	emailVerifier  *auth.EmailVerifier
	cleanupTicker  *time.Ticker
	cleanupDone    chan bool
}

// New creates a new Server instance with all dependencies initialized.
func New(cfg *config.Config) (*Server, error) {
	dbPool, err := database.NewPool(cfg.DatabaseURL, database.DefaultConfig())
	if err != nil {
		return nil, fmt.Errorf("failed to create database pool: %w", err)
	}
	slog.Info("Database connection pool established")

	queries := db.New(dbPool)

	authConfig, jwtValidator, libopsTokenIssuer, apiKeyManager, authHandler, authorizer, emailVerifier, userpassClient, err := setupAuth(cfg, queries)
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

	logServerConfig(cfg, authConfig, ceClient)

	return &Server{
		config:         cfg,
		httpServer:     httpServer,
		dbPool:         dbPool,
		queueProcessor: queueProcessor,
		emailVerifier:  emailVerifier,
		cleanupDone:    make(chan bool),
	}, nil
}

// Start begins listening for HTTP requests.
func (s *Server) Start() error {
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
	*auth.Config,
	*auth.VaultJWTValidator,
	*auth.LibopsTokenIssuer,
	*auth.APIKeyManager,
	*auth.Handler,
	*auth.Authorizer,
	*auth.EmailVerifier,
	*auth.UserpassClient,
	error,
) {
	authConfig, err := auth.NewConfigFromEnv()
	if err != nil {
		slog.Warn("Authentication disabled", "error", err)
		return nil, nil, nil, nil, nil, nil, nil, nil, nil
	}

	vaultClient, err := vault.NewClient(&vault.Config{
		Address: authConfig.VaultAddr,
		Token:   authConfig.VaultToken,
	})
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize vault client: %w", err)
	}

	jwtValidator := auth.NewJWTValidator(authConfig.VaultAddr, authConfig.VaultOIDCProvider)

	if err := jwtValidator.Initialize(context.Background()); err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize JWT validator: %w", err)
	}

	auditLogger := audit.New(queries)

	// Initialize session manager
	sessionManager := auth.NewSessionManager(queries, "", true)

	// Initialize unified token issuer
	libopsTokenIssuer := auth.NewLibopsTokenIssuer(vaultClient, queries, sessionManager, authConfig.VaultAddr, authConfig.VaultOIDCProvider, authConfig.AdminEmails, auditLogger)

	apiKeyManager := auth.NewAPIKeyManager(vaultClient, queries, auditLogger)

	jwtValidator.SetAPIKeyManager(apiKeyManager)

	// Initialize OIDC client
	oidcClient, err := auth.NewOIDCClient(authConfig)
	if err != nil {
		return nil, nil, nil, nil, nil, nil, nil, nil, fmt.Errorf("failed to initialize OIDC client: %w", err)
	}

	emailVerifier := auth.NewEmailVerifier(queries, nil, cfg.APIBaseURL) // nil = no email sender (dev mode)

	userpassClient := auth.NewUserpassClient(vaultClient, "userpass", queries, emailVerifier)

	authorizer := auth.NewAuthorizer(queries, authConfig.AdminEmails)

	// Initialize auth handler
	authHandler := auth.NewHandler(oidcClient, userpassClient, jwtValidator, sessionManager, queries, vaultClient, authConfig.VaultOIDCProvider)

	slog.Info("Authentication enabled",
		"vault", authConfig.VaultAddr,
		"provider", authConfig.VaultOIDCProvider,
		"token_len", len(authConfig.VaultToken))

	if len(authConfig.AdminEmails) > 0 {
		slog.Info("Admin accounts configured", "count", len(authConfig.AdminEmails))
	}

	return authConfig, jwtValidator, libopsTokenIssuer, apiKeyManager, authHandler, authorizer, emailVerifier, userpassClient, nil
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
func logServerConfig(cfg *config.Config, authConfig *auth.Config, ceClient cloudevents.Client) {
	if authConfig != nil {
		slog.Info("Authorization interceptor enabled")
		slog.Info("Auth middleware enabled")
		slog.Info("Auth routes registered")
	}
}
