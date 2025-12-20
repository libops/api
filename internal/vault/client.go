package vault

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"os"
	"time"

	"github.com/hashicorp/vault/api"
)

// Client wraps the Vault API client for libops-specific operations.
type Client struct {
	client *api.Client
}

// Config holds Vault client configuration.
type Config struct {
	Address string
	Token   string // Optional: for service-level operations
}

const (
	jwtFilePath = "/vault/secrets/GOOGLE_ACCESS_TOKEN"
	roleName    = "libops-api"

	// Retry configuration for Vault requests
	maxRetries     = 3
	initialBackoff = 100 * time.Millisecond
	maxBackoff     = 2 * time.Second
	backoffFactor  = 2.0
)

// NewClient creates a new Vault client wrapper.
func NewClient(config *Config) (*Client, error) {
	vaultConfig := api.DefaultConfig()
	vaultConfig.Address = config.Address

	client, err := api.NewClient(vaultConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	if config.Token != "" {
		client.SetToken(config.Token)
	} else if token := os.Getenv("VAULT_TOKEN"); token != "" {
		client.SetToken(token)
	}

	return &Client{
		client: client,
	}, nil
}

// NewClientFromAddr creates a Vault client with just an address.
func NewClientFromAddr(vaultAddr string) (*Client, error) {
	return NewClient(&Config{
		Address: vaultAddr,
	})
}

// GetAPIClient returns the underlying Vault API client
// Use this when you need direct access to Vault API.
func (c *Client) GetAPIClient() *api.Client {
	return c.client
}

// SetToken sets the token for the client.
func (c *Client) SetToken(token string) {
	c.client.SetToken(token)
}

// Clone creates a new client with the same configuration.
func (c *Client) Clone() (*Client, error) {
	cloned, err := c.client.Clone()
	if err != nil {
		return nil, fmt.Errorf("failed to clone vault client: %w", err)
	}

	return &Client{
		client: cloned,
	}, nil
}

// WithToken creates a new client with a different token
// Useful for making requests with user tokens.
func (c *Client) WithToken(token string) (*Client, error) {
	cloned, err := c.Clone()
	if err != nil {
		return nil, err
	}

	cloned.SetToken(token)
	return cloned, nil
}

// LookupToken looks up information about the current token.
func (c *Client) LookupToken(ctx context.Context) (*api.Secret, error) {
	secret, err := c.client.Auth().Token().LookupSelf()
	if err != nil {
		return nil, fmt.Errorf("failed to lookup token: %w", err)
	}
	return secret, nil
}

// GetEntityID gets the entity ID associated with the current token.
func (c *Client) GetEntityID(ctx context.Context) (string, error) {
	secret, err := c.LookupToken(ctx)
	if err != nil {
		return "", err
	}

	if secret.Data == nil {
		return "", fmt.Errorf("no data in token lookup response")
	}

	entityID, ok := secret.Data["entity_id"].(string)
	if !ok || entityID == "" {
		return "", fmt.Errorf("no entity_id found in token")
	}

	return entityID, nil
}

// GetTokenMetadata retrieves metadata from the current token.
func (c *Client) GetTokenMetadata(ctx context.Context, key string) (string, error) {
	secret, err := c.LookupToken(ctx)
	if err != nil {
		return "", err
	}

	if secret.Data == nil {
		return "", fmt.Errorf("no data in token lookup response")
	}

	metadata, ok := secret.Data["meta"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("no metadata found in token")
	}

	value, ok := metadata[key].(string)
	if !ok {
		return "", fmt.Errorf("metadata key %s not found", key)
	}

	return value, nil
}

// retryWithBackoff executes an operation with exponential backoff retry logic.
// It retries transient errors up to maxRetries times with exponentially increasing delays.
func retryWithBackoff[T any](ctx context.Context, operation string, fn func() (T, error)) (T, error) {
	var result T
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		result, lastErr = fn()
		if lastErr == nil {
			return result, nil
		}

		// Check if we should retry
		if !isRetryableError(lastErr) {
			return result, lastErr
		}

		// Don't sleep after the last attempt
		if attempt == maxRetries {
			break
		}

		// Calculate backoff duration with exponential increase
		backoff := time.Duration(float64(initialBackoff) * math.Pow(backoffFactor, float64(attempt)))
		if backoff > maxBackoff {
			backoff = maxBackoff
		}

		slog.Warn("Vault operation failed, retrying",
			"operation", operation,
			"attempt", attempt+1,
			"max_attempts", maxRetries+1,
			"backoff", backoff,
			"error", lastErr)

		select {
		case <-ctx.Done():
			return result, ctx.Err()
		case <-time.After(backoff):
			// Continue to next retry
		}
	}

	return result, fmt.Errorf("vault operation %s failed after %d attempts: %w", operation, maxRetries+1, lastErr)
}

// isRetryableError determines if a Vault error should trigger a retry.
// Retries transient network errors and server errors, but not auth/permission errors.
func isRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Check for Vault API response errors
	if respErr, ok := err.(*api.ResponseError); ok {
		// Retry on 5xx server errors and 429 rate limiting
		statusCode := respErr.StatusCode
		return statusCode == 429 || (statusCode >= 500 && statusCode < 600)
	}

	// Retry on generic network/timeout errors
	// The Vault Go client wraps these in various ways, so we check the error message
	errMsg := err.Error()
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporary failure",
		"no such host",
		"i/o timeout",
		"tls handshake timeout",
	}

	for _, pattern := range retryablePatterns {
		if containsString(errMsg, pattern) {
			return true
		}
	}

	return false
}

// containsString checks if a string contains a substring (case-insensitive)
func containsString(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
