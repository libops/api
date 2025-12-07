package vault

import (
	"context"
	"fmt"
)

// EntityInfo represents Vault entity information.
type EntityInfo struct {
	ID       string
	Name     string
	Metadata map[string]string
	Policies []string
	Disabled bool
}

// ValidateEntity checks if an entity exists and is active in Vault.
func (c *Client) ValidateEntity(ctx context.Context, entityID string) (*EntityInfo, error) {
	path := fmt.Sprintf("identity/entity/id/%s", entityID)
	secret, err := c.client.Logical().Read(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read entity: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("entity not found: %s", entityID)
	}

	disabled, _ := secret.Data["disabled"].(bool)
	if disabled {
		return nil, fmt.Errorf("entity is disabled: %s", entityID)
	}

	entityInfo := &EntityInfo{
		ID:       entityID,
		Disabled: disabled,
	}

	if name, ok := secret.Data["name"].(string); ok {
		entityInfo.Name = name
	}

	if metadataRaw, ok := secret.Data["metadata"].(map[string]any); ok {
		entityInfo.Metadata = make(map[string]string)
		for k, v := range metadataRaw {
			if strVal, ok := v.(string); ok {
				entityInfo.Metadata[k] = strVal
			}
		}
	}

	if policiesRaw, ok := secret.Data["policies"].([]any); ok {
		entityInfo.Policies = make([]string, 0, len(policiesRaw))
		for _, p := range policiesRaw {
			if pStr, ok := p.(string); ok {
				entityInfo.Policies = append(entityInfo.Policies, pStr)
			}
		}
	}

	return entityInfo, nil
}

// CreateEntityToken creates a token for a specific entity with proper policy assignment.
func (c *Client) CreateEntityToken(ctx context.Context, entityID string, policies []string, ttl string) (string, error) {
	_, err := c.ValidateEntity(ctx, entityID)
	if err != nil {
		return "", fmt.Errorf("entity validation failed: %w", err)
	}

	tokenRequest := map[string]any{
		"entity_id": entityID,
		"ttl":       ttl,
		"policies":  policies,
	}

	secret, err := c.client.Logical().Write("auth/token/create", tokenRequest)
	if err != nil {
		return "", fmt.Errorf("failed to create entity token: %w", err)
	}

	if secret == nil || secret.Auth == nil {
		return "", fmt.Errorf("no auth data in token creation response")
	}

	clientToken := secret.Auth.ClientToken
	if clientToken == "" {
		return "", fmt.Errorf("empty client token in response")
	}

	return clientToken, nil
}

// GetOIDCTokenWithAccountID requests an OIDC token from Vault's OIDC provider using an entity token.
// accountID is ignored as it should be present in the entity metadata for the template to pick up.
func (c *Client) GetOIDCTokenWithAccountID(ctx context.Context, entityToken, provider string, accountID int64, scopes []string) (string, int, error) {
	tokenClient, err := c.WithToken(entityToken)
	if err != nil {
		return "", 0, fmt.Errorf("failed to create token client: %w", err)
	}

	path := fmt.Sprintf("identity/oidc/token/%s", provider)

	// Vault OIDC token endpoint uses GET.
	// Parameters like scope must be passed as query parameters.
	if len(scopes) > 0 {
		path += "?scope=" + scopes[0]
		for i := 1; i < len(scopes); i++ {
			path += "+" + scopes[i]
		}
	}

	secret, err := tokenClient.client.Logical().Read(path)
	if err != nil {
		return "", 0, fmt.Errorf("failed to request OIDC token: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return "", 0, fmt.Errorf("no data in OIDC token response")
	}

	token, ok := secret.Data["token"].(string)
	if !ok || token == "" {
		return "", 0, fmt.Errorf("no token in OIDC response")
	}

	ttl := 3600 // default 1 hour
	if ttlRaw, ok := secret.Data["ttl"]; ok {
		switch v := ttlRaw.(type) {
		case int:
			ttl = v
		case int64:
			ttl = int(v)
		case float64:
			ttl = int(v)
		}
	}

	return token, ttl, nil
}

// DeterminePolicies determines which Vault policies should be assigned based on account attributes.
func DeterminePolicies(isAdmin bool, authMethod string) []string {
	policies := []string{"default", "libops-user"}

	if isAdmin {
		policies = append(policies, "admin-policy")
	} else {
		policies = append(policies, "organization-policy")
	}

	switch authMethod {
	case "google":
		// Google OAuth users might have specific policies
		policies = append(policies, "oauth-user-policy")
	case "gcloud":
		// Service accounts have restricted policies
		policies = append(policies, "service-account-policy")
	}

	return policies
}
