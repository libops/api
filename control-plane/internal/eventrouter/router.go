// Package eventrouter implements the control plane event routing service
package eventrouter

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/libops/control-plane/internal/database"
	"github.com/libops/control-plane/internal/publisher"
)

// Config holds event router configuration
type Config struct {
	DatabaseURL         string
	PollIntervalSeconds int
	MaxConcurrentEvents int
	LogLevel            string
	Port                string
	ProjectID           string // GCP project ID for Pub/Sub
}

// Run starts the event router service
func Run() error {
	cfg, err := loadConfig()
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	// Setup logging
	setupLogging(cfg.LogLevel)

	slog.Info("Starting event router service")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Connect to MariaDB (for event_queue and resource lookups)
	eventsDB, err := sql.Open("mysql", cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to connect to events database: %w", err)
	}
	defer eventsDB.Close()

	if err := eventsDB.Ping(); err != nil {
		return fmt.Errorf("failed to ping events database: %w", err)
	}

	slog.Info("Connected to events database")

	// Create database querier for resource lookups
	dbQuerier := database.NewQuerier(eventsDB)

	// Create Pub/Sub publisher
	pubsubPublisher, err := publisher.NewPubSubPublisher(ctx, cfg.ProjectID)
	if err != nil {
		return fmt.Errorf("failed to create Pub/Sub publisher: %w", err)
	}
	defer pubsubPublisher.Close()

	slog.Info("Pub/Sub publisher initialized")

	// Create activity handler
	activityHandler := publisher.NewActivityHandler(dbQuerier, pubsubPublisher)

	// Create reconciliation manager
	manager := NewReconciliationManager(activityHandler)

	// Create event poller
	poller := NewEventPoller(eventsDB, manager, cfg)

	// Start event poller
	go poller.Start(ctx)

	// Start HTTP server for health checks
	go startHealthServer(cfg.Port)

	slog.Info("Event router service started", "port", cfg.Port)

	// Wait for shutdown signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	slog.Info("Shutting down event router service...")
	cancel()

	slog.Info("Event router service stopped")
	return nil
}
func loadConfig() (*Config, error) {
	pollInterval, _ := strconv.Atoi(getEnv("POLL_INTERVAL_SECONDS", "5"))
	maxConcurrent, _ := strconv.Atoi(getEnv("MAX_CONCURRENT_EVENTS", "10"))
	c := Config{
		PollIntervalSeconds: pollInterval,
		MaxConcurrentEvents: maxConcurrent,
		LogLevel:            getEnv("LOG_LEVEL", "INFO"),
		Port:                getEnv("PORT", "8081"),
		ProjectID:           getEnv("PROJECT_ID", ""),
	}

	if passwordFile := os.Getenv("MARIADB_PASSWORD_FILE"); passwordFile != "" {
		password, err := os.ReadFile(passwordFile)
		if err != nil {
			return nil, fmt.Errorf("failed to read MARIADB_PASSWORD_FILE: %w", err)
		}
		c.DatabaseURL = fmt.Sprintf("libops:%s@tcp(mariadb:3306)/libops?parseTime=true", strings.TrimSpace(string(password)))
	}

	return &c, nil
}
func setupLogging(level string) {
	var logLevel slog.Level
	switch level {
	case "DEBUG":
		logLevel = slog.LevelDebug
	case "INFO":
		logLevel = slog.LevelInfo
	case "WARN":
		logLevel = slog.LevelWarn
	case "ERROR":
		logLevel = slog.LevelError
	default:
		logLevel = slog.LevelInfo
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)
}

func startHealthServer(port string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	server := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	slog.Info("Health server listening", "port", port)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		slog.Error("Health server error", "error", err)
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
