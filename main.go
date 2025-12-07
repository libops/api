package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/libops/api/internal/config"
	"github.com/libops/api/internal/logging"
	"github.com/libops/api/internal/server"
)

func main() {
	// Set up context-aware logging as default
	setupLogging()

	if err := run(); err != nil {
		slog.Error("Application error", "err", err)
		os.Exit(1)
	}
}

func setupLogging() {
	// Get log level from environment variable
	level := getLogLevel()

	// Create a text handler for human-readable logging
	textHandler := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: level,
	})

	// Wrap it with the context handler to include request IDs
	contextHandler := logging.NewContextHandler(textHandler)

	// Set as default logger
	slog.SetDefault(slog.New(contextHandler))
}

func getLogLevel() slog.Level {
	switch strings.ToLower(os.Getenv("LOG_LEVEL")) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

func run() error {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Create and initialize server
	srv, err := server.New(cfg)
	if err != nil {
		return err
	}

	// Start server in goroutine
	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- srv.Start()
	}()

	// Wait for interrupt signal or server error
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		return err
	case sig := <-shutdown:
		slog.Info("Shutdown signal received", "signal", sig)

		// Give server 30 seconds to gracefully shutdown
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		return srv.Shutdown(ctx)
	}
}
