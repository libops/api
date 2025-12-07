package vault

import (
	"context"
	"fmt"

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
