package vault

import (
	"context"
	"fmt"
)

// KeysStore manages API keys in Vault KV v1 secret engine.
// API keys are stored at keys/{accountUUID}/{keyUUID} with the random secret as the value.
// The full API key format is: libops_{accountUUID}_{keyUUID}_{randomSecret}
// Only the randomSecret component is stored in Vault, using nested paths.
type KeysStore struct {
	kv *KVv1
}

// NewKeysStore creates a new keys store.
func NewKeysStore(client *Client) *KeysStore {
	return &KeysStore{
		kv: NewKVv1(client, "keys"),
	}
}

// StoreKey stores an API key's random secret in Vault at keys/{accountUUID}/{keyUUID}.
// The random secret is stored for verification during authentication.
func (ks *KeysStore) StoreKey(ctx context.Context, accountUUID string, keyUUID string, randomSecret string) error {
	keyData := map[string]any{
		"secret": randomSecret,
	}

	// Remove dashes from UUIDs for cleaner paths
	accountUUIDNoDashes := stripDashes(accountUUID)
	keyUUIDNoDashes := stripDashes(keyUUID)
	path := fmt.Sprintf("%s/%s", accountUUIDNoDashes, keyUUIDNoDashes)

	if err := ks.kv.Write(ctx, path, keyData); err != nil {
		return fmt.Errorf("failed to store key secret: %w", err)
	}

	return nil
}

// GetKeySecret retrieves the random secret for an API key from Vault.
// Returns an error if the key doesn't exist or cannot be retrieved.
func (ks *KeysStore) GetKeySecret(ctx context.Context, accountUUID string, keyUUID string) (string, error) {
	// Remove dashes from UUIDs for cleaner paths
	accountUUIDNoDashes := stripDashes(accountUUID)
	keyUUIDNoDashes := stripDashes(keyUUID)
	path := fmt.Sprintf("%s/%s", accountUUIDNoDashes, keyUUIDNoDashes)

	data, err := ks.kv.Read(ctx, path)
	if err != nil {
		if err.Error() == "secret not found" {
			return "", fmt.Errorf("API key not found")
		}
		return "", fmt.Errorf("failed to retrieve key secret: %w", err)
	}

	secret, ok := data["secret"].(string)
	if !ok {
		return "", fmt.Errorf("invalid secret format in Vault")
	}

	return secret, nil
}

// DeleteKey removes an API key from Vault by accountUUID and keyUUID.
// This should be called when an API key is revoked.
func (ks *KeysStore) DeleteKey(ctx context.Context, accountUUID string, keyUUID string) error {
	// Remove dashes from UUIDs for cleaner paths
	accountUUIDNoDashes := stripDashes(accountUUID)
	keyUUIDNoDashes := stripDashes(keyUUID)
	path := fmt.Sprintf("%s/%s", accountUUIDNoDashes, keyUUIDNoDashes)

	if err := ks.kv.Delete(ctx, path); err != nil {
		return fmt.Errorf("failed to delete key secret: %w", err)
	}
	return nil
}

// stripDashes removes dashes from a UUID string.
func stripDashes(uuid string) string {
	result := make([]byte, 0, 32)
	for i := 0; i < len(uuid); i++ {
		if uuid[i] != '-' {
			result = append(result, uuid[i])
		}
	}
	return string(result)
}
