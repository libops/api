// Package main implements the Site VM Controller
// This service handles VM-level configuration reconciliation for SSH keys, secrets, and firewall rules
package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/libops/controller/internal/reconciler"
	"golang.org/x/time/rate"
)

// Controller handles HTTP requests and coordinates reconciliation
type Controller struct {
	reconciler *reconciler.Reconciler
	limiter    *rate.Limiter
}

// NewController creates a new controller
func NewController(r *reconciler.Reconciler, rps int, burst int) *Controller {
	return &Controller{
		reconciler: r,
		limiter:    rate.NewLimiter(rate.Limit(rps), burst),
	}
}

// rateLimitMiddleware applies rate limiting to HTTP handlers
func (c *Controller) rateLimitMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !c.limiter.Allow() {
			http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next(w, r)
	}
}

// handleSSHKeysReconcile handles SSH key reconciliation requests
func (c *Controller) handleSSHKeysReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slog.Info("SSH keys reconciliation triggered")

	ctx := r.Context()
	if err := c.reconciler.ReconcileSSHKeys(ctx); err != nil {
		slog.Error("SSH keys reconciliation failed", "error", err)
		http.Error(w, fmt.Sprintf("Reconciliation failed: %v", err), http.StatusInternalServerError)
		return
	}

	slog.Info("SSH keys reconciliation completed successfully")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "SSH keys reconciliation completed\n")
}

// handleSecretsReconcile handles secrets reconciliation requests
func (c *Controller) handleSecretsReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slog.Info("secrets reconciliation triggered")

	ctx := r.Context()
	if err := c.reconciler.ReconcileSecrets(ctx); err != nil {
		slog.Error("secrets reconciliation failed", "error", err)
		http.Error(w, fmt.Sprintf("Reconciliation failed: %v", err), http.StatusInternalServerError)
		return
	}

	slog.Info("secrets reconciliation completed successfully")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "secrets reconciliation completed\n")
}

// handleFirewallReconcile handles firewall reconciliation requests
func (c *Controller) handleFirewallReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slog.Info("firewall reconciliation triggered")

	ctx := r.Context()
	if err := c.reconciler.ReconcileFirewall(ctx); err != nil {
		slog.Error("firewall reconciliation failed", "error", err)
		http.Error(w, fmt.Sprintf("Reconciliation failed: %v", err), http.StatusInternalServerError)
		return
	}

	slog.Info("firewall reconciliation completed successfully")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Firewall reconciliation completed\n")
}

// handleGeneralReconcile handles general (full) reconciliation requests
func (c *Controller) handleGeneralReconcile(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slog.Info("general reconciliation triggered")

	ctx := r.Context()
	if err := c.reconciler.ReconcileAll(ctx); err != nil {
		slog.Error("general reconciliation failed", "error", err)
		http.Error(w, fmt.Sprintf("Reconciliation failed: %v", err), http.StatusInternalServerError)
		return
	}

	slog.Info("general reconciliation completed successfully")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "General reconciliation completed\n")
}

// handleDeployment handles deployment requests (triggered by GitHub webhooks)
func (c *Controller) handleDeployment(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	slog.Info("deployment reconciliation triggered")

	ctx := r.Context()
	if err := c.reconciler.ReconcileDeployment(ctx); err != nil {
		slog.Error("deployment reconciliation failed", "error", err)
		http.Error(w, fmt.Sprintf("Deployment failed: %v", err), http.StatusInternalServerError)
		return
	}

	slog.Info("deployment reconciliation completed successfully")
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "Deployment completed\n")
}

// handleHealth handles health check requests
func (c *Controller) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	fmt.Fprintf(w, "OK\n")
}

// startPeriodicReconciliation runs full reconciliation every 12 hours
func (c *Controller) startPeriodicReconciliation(ctx context.Context) {
	ticker := time.NewTicker(12 * time.Hour)
	defer ticker.Stop()

	slog.Info("starting periodic reconciliation (every 12 hours)")

	// Run once immediately
	if err := c.reconciler.ReconcileAll(ctx); err != nil {
		slog.Error("initial reconciliation failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("stopping periodic reconciliation")
			return
		case <-ticker.C:
			slog.Info("running periodic reconciliation")
			if err := c.reconciler.ReconcileAll(ctx); err != nil {
				slog.Error("periodic reconciliation failed", "error", err)
			}
		}
	}
}

// startCheckInTask runs check-in every 60 seconds
func (c *Controller) startCheckInTask(ctx context.Context) {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	slog.Info("starting check-in task (every 60 seconds)")

	// Run once immediately
	if err := c.reconciler.CheckIn(ctx); err != nil {
		slog.Error("initial check-in failed", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("stopping check-in task")
			return
		case <-ticker.C:
			if err := c.reconciler.CheckIn(ctx); err != nil {
				slog.Error("check-in failed", "error", err)
			}
		}
	}
}

func main() {
	// Setup structured logging
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("starting VM controller service")

	// Get configuration from environment
	apiURL := os.Getenv("LIBOPS_API_URL")
	if apiURL == "" {
		apiURL = "https://api.libops.io"
	}

	siteID := os.Getenv("SITE_ID")
	if siteID == "" {
		slog.Error("SITE_ID environment variable is required")
		os.Exit(1)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Rate limiting configuration
	rps := 10
	burst := 5
	if rpsEnv := os.Getenv("RATE_LIMIT_RPS"); rpsEnv != "" {
		fmt.Sscanf(rpsEnv, "%d", &rps)
	}
	if burstEnv := os.Getenv("RATE_LIMIT_BURST"); burstEnv != "" {
		fmt.Sscanf(burstEnv, "%d", &burst)
	}

	// Initialize reconciler
	rec := reconciler.NewReconciler(apiURL, siteID)

	// Initialize controller
	controller := NewController(rec, rps, burst)

	// Setup HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/health", controller.handleHealth)
	mux.HandleFunc("/reconcile/ssh-keys", controller.rateLimitMiddleware(controller.handleSSHKeysReconcile))
	mux.HandleFunc("/reconcile/secrets", controller.rateLimitMiddleware(controller.handleSecretsReconcile))
	mux.HandleFunc("/reconcile/firewall", controller.rateLimitMiddleware(controller.handleFirewallReconcile))
	mux.HandleFunc("/reconcile/general", controller.rateLimitMiddleware(controller.handleGeneralReconcile))
	mux.HandleFunc("/reconcile/deployment", controller.rateLimitMiddleware(controller.handleDeployment))

	server := &http.Server{
		Addr:         fmt.Sprintf(":%s", port),
		Handler:      mux,
		ReadTimeout:  60 * time.Second, // Long timeout for reconciliation operations
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Create context for background tasks
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start background tasks
	go controller.startPeriodicReconciliation(ctx)
	go controller.startCheckInTask(ctx)

	// Start server in goroutine
	go func() {
		slog.Info("starting HTTP server",
			"port", port,
			"rate_limit_rps", rps,
			"rate_limit_burst", burst,
			"site_id", siteID)

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down gracefully")

	// Cancel background tasks
	cancel()

	// Graceful shutdown
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("server shutdown error", "error", err)
	}

	slog.Info("server stopped")
}
