package vault

import (
	"context"
	"fmt"
	"os"
)

// WriteSecret writes a secret to organization's Vault instance (write-only).
func (c *Client) WriteSecret(ctx context.Context, path string, data map[string]any) error {
	_, err := c.client.Logical().Write(path, data)
	if err != nil {
		return fmt.Errorf("failed to write secret to vault: %w", err)
	}
	return nil
}

// DeleteSecret deletes a secret from organization's Vault instance.
func (c *Client) DeleteSecret(ctx context.Context, path string) error {
	_, err := c.client.Logical().Delete(path)
	if err != nil {
		return fmt.Errorf("failed to delete secret from vault: %w", err)
	}
	return nil
}

// BuildOrganizationSecretPath creates the Vault path for a organization-level secret.
func BuildOrganizationSecretPath(secretName string) string {
	return fmt.Sprintf("secret-global/%s", secretName)
}

// BuildProjectSecretPath creates the Vault path for a project-level secret.
func BuildProjectSecretPath(projectPublicID, secretName string) string {
	return fmt.Sprintf("secret-project/%s/%s", projectPublicID, secretName)
}

// BuildSiteSecretPath creates the Vault path for a site-level secret.
func BuildSiteSecretPath(sitePublicID, secretName string) string {
	return fmt.Sprintf("secret-site/%s/%s", sitePublicID, secretName)
}

// GetOrganizationVaultURL constructs the vault server URL for a organization
// Uses the organization's project number and region to build the Cloud Run URL.
func GetOrganizationVaultURL(projectNumber int64, region string) string {
	// Support integration tests where VAULT_ADDR is set
	if addr := os.Getenv("VAULT_ADDR"); addr != "" {
		return addr
	}
	return fmt.Sprintf("https://vault-server-%d.%s.run.app", projectNumber, region)
}
