package vault

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
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

// GetAuthMountAccessor returns the accessor for a given auth mount type.
func (c *Client) GetAuthMountAccessor(ctx context.Context, mountType string) (string, error) {
	secret, err := c.client.Sys().ListAuth()
	if err != nil {
		return "", fmt.Errorf("failed to list auth mounts: %w", err)
	}

	if secret == nil {
		return "", fmt.Errorf("no auth mounts found")
	}

	for _, mountData := range secret {
		if mountData.Type == mountType {
			return mountData.Accessor, nil
		}
	}

	return "", fmt.Errorf("mount accessor not found for type: %s", mountType)
}

// CreateEntityAlias creates an entity alias for a specific auth mount.
func (c *Client) CreateEntityAlias(ctx context.Context, entityID, mountAccessor, name string) error {
	aliasData := map[string]interface{}{
		"name":           name,
		"canonical_id":   entityID,
		"mount_accessor": mountAccessor,
	}

	_, err := c.client.Logical().Write("identity/entity-alias", aliasData)
	if err != nil {
		return fmt.Errorf("failed to create entity alias: %w", err)
	}

	slog.Info("Created entity alias", "entity_id", entityID, "alias_name", name, "mount_accessor", mountAccessor)
	return nil
}

// EnsureTokenAlias ensures an entity has a token alias for entity token creation
func (c *Client) EnsureTokenAlias(ctx context.Context, entityID string, email string) error {
	// Check if entity already has a token alias
	hasAlias, err := c.hasTokenAlias(ctx, entityID)
	if err != nil {
		return fmt.Errorf("failed to check for token alias: %w", err)
	}

	if hasAlias {
		slog.Debug("Entity already has token alias", "entity_id", entityID)
		return nil
	}

	// Get token mount accessor
	secret, err := c.client.Sys().ListAuth()
	if err != nil {
		return fmt.Errorf("failed to list auth mounts: %w", err)
	}

	if secret == nil {
		return fmt.Errorf("no auth mounts found")
	}

	var tokenAccessor string
	for _, mountData := range secret {
		if mountData.Type == "token" {
			tokenAccessor = mountData.Accessor
			break
		}
	}

	if tokenAccessor == "" {
		return fmt.Errorf("token mount accessor not found")
	}

	// Create token alias - use "libops-oidc-{email}" as the alias name
	aliasName := fmt.Sprintf("libops-oidc-%s", strings.ReplaceAll(email, "@", "_"))
	aliasData := map[string]interface{}{
		"name":           aliasName,
		"canonical_id":   entityID,
		"mount_accessor": tokenAccessor,
	}

	_, err = c.client.Logical().Write("identity/entity-alias", aliasData)
	if err != nil {
		return fmt.Errorf("failed to create token alias: %w", err)
	}

	slog.Info("Created token alias for entity", "entity_id", entityID, "alias_name", aliasName)
	return nil
}

// hasTokenAlias checks if an entity has a token alias
func (c *Client) hasTokenAlias(ctx context.Context, entityID string) (bool, error) {
	path := fmt.Sprintf("identity/entity/id/%s", entityID)
	secret, err := c.client.Logical().Read(path)
	if err != nil {
		return false, err
	}

	if secret == nil || secret.Data == nil {
		return false, nil
	}

	if aliasesRaw, ok := secret.Data["aliases"].([]interface{}); ok {
		for _, aliasRaw := range aliasesRaw {
			if aliasMap, ok := aliasRaw.(map[string]interface{}); ok {
				if mountType, ok := aliasMap["mount_type"].(string); ok && mountType == "token" {
					return true, nil
				}
			}
		}
	}

	return false, nil
}

// CreateEntityToken creates a token for a specific entity.
// It looks up the entity's userpass credentials and authenticates to get a token.
// Returns both the token and the entity ID from the auth response.
func (c *Client) CreateEntityToken(ctx context.Context, entityID string, policies []string, ttl string) (string, string, error) {
	// Read entity to get userpass alias
	path := fmt.Sprintf("identity/entity/id/%s", entityID)
	secret, err := c.client.Logical().Read(path)
	if err != nil {
		return "", "", fmt.Errorf("failed to read entity: %w", err)
	}

	if secret == nil || secret.Data == nil {
		return "", "", fmt.Errorf("entity not found: %s", entityID)
	}

	// Extract token alias ID
	var tokenAliasID string
	var tokenAliasName string
	if aliasesRaw, ok := secret.Data["aliases"].([]interface{}); ok {
		for _, aliasRaw := range aliasesRaw {
			if aliasMap, ok := aliasRaw.(map[string]interface{}); ok {
				// Look specifically for token mount type
				if mountType, ok := aliasMap["mount_type"].(string); ok && mountType == "token" {
					if id, ok := aliasMap["id"].(string); ok {
						tokenAliasID = id
					}
					if name, ok := aliasMap["name"].(string); ok {
						tokenAliasName = name
					}
					if tokenAliasID != "" {
						break
					}
				}
			}
		}
	}

	if tokenAliasID == "" {
		return "", "", fmt.Errorf("entity %s has no token alias - cannot create token", entityID)
	}

	// Create a token using the entity-token role with the token alias
	tokenRequest := map[string]any{
		"ttl":          ttl,
		"policies":     policies,
		"entity_alias": tokenAliasID,
	}

	slog.Debug("Creating entity token", "entity_id", entityID, "token_alias_id", tokenAliasID, "token_alias_name", tokenAliasName, "policies", policies, "ttl", ttl)

	secret, err = c.client.Logical().Write("auth/token/create/entity-token", tokenRequest)
	if err != nil {
		return "", "", fmt.Errorf("failed to create entity token: %w", err)
	}

	if secret == nil || secret.Auth == nil {
		return "", "", fmt.Errorf("no auth data in token creation response")
	}

	clientToken := secret.Auth.ClientToken
	if clientToken == "" {
		return "", "", fmt.Errorf("empty client token in response")
	}

	// Get the actual entity ID from the token
	tokenEntityID := secret.Auth.EntityID
	if tokenEntityID == "" {
		slog.Warn("Created token has no entity association", "requested_entity_id", entityID)
		// Token was created but without entity association - return it anyway with empty entity ID
		return clientToken, "", nil
	}

	slog.Info("Entity token created",
		"requested_entity_id", entityID,
		"token_entity_id", tokenEntityID,
		"matches", entityID == tokenEntityID)

	if tokenEntityID != entityID {
		slog.Warn("Token created with different entity than requested",
			"requested", entityID, "actual", tokenEntityID, "token_alias_id", tokenAliasID)
	}

	return clientToken, tokenEntityID, nil
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
func DeterminePolicies(authMethod string) []string {
	policies := []string{"default", "libops-user"}

	return policies
}

// CreateEntity creates a new Vault entity with the given name and metadata.
func (c *Client) CreateEntity(ctx context.Context, name string, metadata map[string]string, policies []string) (string, error) {
	data := map[string]any{
		"name":     name,
		"policies": policies,
	}

	if metadata != nil {
		data["metadata"] = metadata
	}

	secret, err := c.client.Logical().Write("identity/entity", data)
	if err != nil {
		return "", fmt.Errorf("failed to create entity: %w", err)
	}

	if secret == nil || secret.Data == nil {
		// Fallback: Try to look up by name
		slog.Warn("CreateEntity returned no data, attempting lookup by name", "name", name)
		path := fmt.Sprintf("identity/entity/name/%s", name)
		secret, err = c.client.Logical().Read(path)
		if err != nil {
			return "", fmt.Errorf("failed to look up entity after creation: %w", err)
		}
		if secret == nil || secret.Data == nil {
			return "", fmt.Errorf("no data in entity creation response and lookup failed")
		}
	}

	entityID, ok := secret.Data["id"].(string)
	if !ok || entityID == "" {
		return "", fmt.Errorf("no entity ID in response")
	}

	return entityID, nil
}

// UpdateEntity updates an existing Vault entity.
func (c *Client) UpdateEntity(ctx context.Context, entityID string, metadata map[string]string) error {
	path := fmt.Sprintf("identity/entity/id/%s", entityID)
	data := map[string]any{
		"id":       entityID,
		"metadata": metadata,
	}

	slog.Debug("Updating Vault entity", "path", path, "entity_id", entityID, "metadata", metadata)

	resp, err := c.client.Logical().Write(path, data)
	if err != nil {
		return fmt.Errorf("failed to update entity: %w", err)
	}

	slog.Debug("Vault entity update response", "entity_id", entityID, "response", resp)
	return nil
}
