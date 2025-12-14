package vault

import (
	"context"
	"fmt"
)

// KeysStore manages API keys in Vault KV v1 secret engine.
// API keys are stored with embedded metadata in the secret value itself:
// Format: {keyUUID_no_dashes}_{accountUUID_no_dashes}_{randomSecret}
// This eliminates the need to store metadata separately.
type KeysStore struct {
	kv *KVv1
}

// NewKeysStore creates a new keys store.
func NewKeysStore(client *Client) *KeysStore {
	return &KeysStore{
		kv: NewKVv1(client, "keys"),
	}
}

// StoreKey stores an API key secret in Vault at keys/{secretValue}.
// The secret value is stored in the data for verification during authentication.
func (ks *KeysStore) StoreKey(ctx context.Context, secretValue string) error {
	keyData := map[string]any{
		"secret": secretValue,
	}

	if err := ks.kv.Write(ctx, secretValue, keyData); err != nil {
		return fmt.Errorf("failed to store key secret: %w", err)
	}

	return nil
}

// KeyExists checks if an API key exists in Vault.
func (ks *KeysStore) KeyExists(ctx context.Context, secretValue string) (bool, error) {
	_, err := ks.kv.Read(ctx, secretValue)
	if err != nil {
		if err.Error() == "secret not found" {
			return false, nil
		}
		return false, fmt.Errorf("failed to check key existence: %w", err)
	}
	return true, nil
}

// DeleteKey removes an API key from Vault by secret value.
// Since secret values are only shown once at creation, deletion typically happens
// via database-only deletion with orphaned Vault entries cleaned up by a background job.
func (ks *KeysStore) DeleteKey(ctx context.Context, secretValue string) error {
	if err := ks.kv.Delete(ctx, secretValue); err != nil {
		return fmt.Errorf("failed to delete key secret: %w", err)
	}
	return nil
}
