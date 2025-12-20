package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	// Environment
	Environment string

	// Database
	DatabaseURL string

	// GCP
	GCPProjectID       string
	GCPRegion          string
	PubSubSubscription string
	CloudRunJobName    string

	// API
	APIBaseURL string

	// Stripe
	StripeAPIKey        string
	StripeWebhookSecret string

	// Terraform
	TerraformStateBucket string
	TerraformModulesDir  string

	// Subscriber
	PollIntervalSeconds int
	MaxConcurrentEvents int
}

func Load() (*Config, error) {
	cfg := &Config{
		Environment:          getEnv("LIBOPS_ENV", "production"),
		DatabaseURL:          getEnv("DATABASE_URL", ""),
		GCPProjectID:         getEnv("GCP_PROJECT_ID", ""),
		GCPRegion:            getEnv("GCP_REGION", "us-central1"),
		PubSubSubscription:   getEnv("PUBSUB_SUBSCRIPTION_ID", "libops-events-sub"),
		CloudRunJobName:      getEnv("CLOUD_RUN_JOB_NAME", "terraform-runner"),
		APIBaseURL:           getEnv("API_BASE_URL", "http://localhost:8080"),
		StripeAPIKey:         getEnv("STRIPE_API_KEY", ""),
		StripeWebhookSecret:  getEnv("STRIPE_WEBHOOK_SECRET", ""),
		TerraformStateBucket: getEnv("TERRAFORM_STATE_BUCKET", ""),
		TerraformModulesDir:  getEnv("TERRAFORM_MODULES_DIR", "/terraform/modules"),
		PollIntervalSeconds:  getEnvInt("POLL_INTERVAL_SECONDS", 1),
		MaxConcurrentEvents:  getEnvInt("MAX_CONCURRENT_EVENTS", 10),
	}

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	return cfg, nil
}

func (c *Config) Validate() error {
	if c.DatabaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	// Only require GCP config in production
	if !c.IsDevelopment() && c.GCPProjectID == "" {
		return fmt.Errorf("GCP_PROJECT_ID is required")
	}
	return nil
}

func (c *Config) IsDevelopment() bool {
	return c.Environment == "development"
}

func (c *Config) PollInterval() time.Duration {
	return time.Duration(c.PollIntervalSeconds) * time.Second
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
