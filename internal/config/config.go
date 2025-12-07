package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds all application configuration.
type Config struct {
	Port         string
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration

	APIBaseURL string // Base URL for the API (e.g., https://api.libops.io)

	DatabaseURL string

	GCPProjectID  string
	EventsTopicID string

	AllowedOrigins []string
}

// Load loads configuration from environment variables.
func Load() (*Config, error) {
	cfg := &Config{
		Port:         getEnv("PORT", "8080"),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,

		APIBaseURL: getEnv("API_BASE_URL", "http://localhost:8080"),

		DatabaseURL: os.Getenv("DATABASE_URL"),

		GCPProjectID:  os.Getenv("GCP_PROJECT_ID"),
		EventsTopicID: os.Getenv("EVENTS_TOPIC_ID"),

		AllowedOrigins: parseAllowedOrigins(os.Getenv("ALLOWED_ORIGINS")),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// Validate checks that required configuration is present.
func (cfg *Config) Validate() error {
	if cfg.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	return nil
}

// getEnv retrieves an environment variable with a fallback default value.
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// parseAllowedOrigins parses ALLOWED_ORIGINS env var or returns secure defaults
// Format: comma-separated list of origins (e.g., "https://api.libops.io,https://dash.libops.io")
// Default: Production origins (api.libops.io and dash.libops.io).
func parseAllowedOrigins(originsEnv string) []string {
	if originsEnv != "" {
		origins := strings.Split(originsEnv, ",")
		for i := range origins {
			origins[i] = strings.TrimSpace(origins[i])
		}
		return origins
	}

	// Default to production origins with both HTTP and HTTPS
	// This ensures the API works in both development and production
	return []string{
		"https://api.libops.io",
		"https://dash.libops.io",
		"http://localhost:3000", // Local development (dash)
		"http://localhost:5173", // Local development (vite)
		"http://localhost:8080", // Local development (API)
	}
}
