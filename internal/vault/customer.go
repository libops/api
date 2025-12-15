package vault

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// NewCustomerVaultClient returns or creates a Vault client for the customer's vault instance.
func NewCustomerVaultClient(ctx context.Context, organizationID, projectNumber int64, region string) (*Client, error) {
	vaultURL := getCustomerVaultURL(projectNumber, region)

	// Create new vault client
	client, err := NewClientFromAddr(vaultURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	// Cache the client
	err = client.JwtAuthVaultClient()
	if err != nil {
		return nil, fmt.Errorf("failed to auth vault client: %w", err)
	}

	return client, nil
}

// JwtAuthVaultClient performs JWT auth to customer Vault
func (c *Client) JwtAuthVaultClient() error {
	// hack for local development
	customerVaultToken := os.Getenv("CUSTOMER_VAULT_TOKEN")
	if customerVaultToken != "" {
		c.SetToken(customerVaultToken)
		return nil
	}

	// otherwise JWT auth to the client
	jwtBytes, err := os.ReadFile(jwtFilePath)
	if err != nil {
		slog.Error("Error reading JWT file", "jwtFilePath", jwtFilePath, "err", err)
		return err
	}
	jwtToken := string(jwtBytes)

	loginData := map[string]any{
		"jwt":  jwtToken,
		"role": roleName,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	secret, err := c.client.Logical().WriteWithContext(ctx, "auth/jwt/login", loginData)
	if err != nil {
		slog.Error("JWT login failed", "err", err)
		return err
	}
	if secret == nil || secret.Auth == nil {
		return fmt.Errorf("login successful, but no auth info returned")
	}

	c.SetToken(secret.Auth.ClientToken)

	return nil
}

// getCustomerVaultURL constructs the vault server URL for an organization
// Uses the organization's project number and region to build the Cloud Run URL.
func getCustomerVaultURL(projectNumber int64, region string) string {
	// Support integration tests where VAULT_ADDR is set
	if addr := os.Getenv("CUSTOMER_VAULT_ADDR"); addr != "" {
		return addr
	}
	return fmt.Sprintf("https://vault-server-%d.%s.run.app", projectNumber, region)
}
