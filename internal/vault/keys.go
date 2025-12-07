package vault

import (
	"context"
	"fmt"
)

// KeysStore manages API keys in Vault KV v1 secret engine
// Keys are stored at two paths:
// keys/{secret_value} - Maps secret to account UUID and API key UUID
// keys-by-uuid/{api_key_uuid} - Maps API key UUID to secret value.
type KeysStore struct {
	kv        *KVv1
	kvReverse *KVv1
}

// NewKeysStore creates a new keys store.
func NewKeysStore(client *Client) *KeysStore {
	return &KeysStore{
		kv:        NewKVv1(client, "keys"),
		kvReverse: NewKVv1(client, "keys-by-uuid"),
	}
}

// APIKeySecret represents the data stored in Vault for an API key.
type APIKeySecret struct {
	AccountUUID string `json:"account_uuid"`
	APIKeyUUID  string `json:"api_key_uuid"`
}

// StoreKey stores an API key in Vault at keys/{secretValue} and keys-by-uuid/{apiKeyUUID}.
func (ks *KeysStore) StoreKey(ctx context.Context, secretValue, accountUUID, apiKeyUUID string) error {
	keyData := map[string]any{
		"account_uuid": accountUUID,
		"api_key_uuid": apiKeyUUID,
	}

	if err := ks.kv.Write(ctx, secretValue, keyData); err != nil {
		return fmt.Errorf("failed to store key secret: %w", err)
	}

	reverseData := map[string]any{
		"secret_value": secretValue,
	}
	if err := ks.kvReverse.Write(ctx, apiKeyUUID, reverseData); err != nil {
		// Rollback forward mapping
		_ = ks.kv.Delete(ctx, secretValue)
		return fmt.Errorf("failed to store reverse key secret: %w", err)
	}

	return nil
}

// GetKeyBySecret retrieves API key information by secret value.
func (ks *KeysStore) GetKeyBySecret(ctx context.Context, secretValue string) (*APIKeySecret, error) {
	data, err := ks.kv.Read(ctx, secretValue)
	if err != nil {
		return nil, fmt.Errorf("failed to read key secret: %w", err)
	}

	accountUUID, ok := data["account_uuid"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid account_uuid in secret")
	}

	apiKeyUUID, ok := data["api_key_uuid"].(string)
	if !ok {
		return nil, fmt.Errorf("invalid api_key_uuid in secret")
	}

	return &APIKeySecret{
		AccountUUID: accountUUID,
		APIKeyUUID:  apiKeyUUID,
	}, nil
}

// GetSecretValueByAPIKeyUUID retrieves the secret value by API key UUID.
func (ks *KeysStore) GetSecretValueByAPIKeyUUID(ctx context.Context, apiKeyUUID string) (string, error) {
	data, err := ks.kvReverse.Read(ctx, apiKeyUUID)
	if err != nil {
		return "", fmt.Errorf("failed to read reverse key secret: %w", err)
	}

	secretValue, ok := data["secret_value"].(string)
	if !ok {
		return "", fmt.Errorf("invalid secret_value in reverse secret")
	}

	return secretValue, nil
}

// DeleteKey removes an API key from Vault using the apiKeyUUID.
func (ks *KeysStore) DeleteKey(ctx context.Context, apiKeyUUID string) error {
	secretValue, err := ks.GetSecretValueByAPIKeyUUID(ctx, apiKeyUUID)
	if err != nil {
		// If the reverse mapping doesn't exist, we can't delete the forward mapping.
		// This is a problem, but we'll log it and continue.
		return fmt.Errorf("could not find secret value for api key %s: %w", apiKeyUUID, err)
	}

	if err := ks.kv.Delete(ctx, secretValue); err != nil {
		return fmt.Errorf("failed to delete key secret: %w", err)
	}

	if err := ks.kvReverse.Delete(ctx, apiKeyUUID); err != nil {
		return fmt.Errorf("failed to delete reverse key secret: %w", err)
	}
	return nil
}
