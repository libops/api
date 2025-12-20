// Package vault provides clients and utilities for interacting with HashiCorp Vault.
package vault

import (
	"context"
	"fmt"

	"github.com/hashicorp/vault/api"
)

// KVv1 provides helpers for working with Vault KV v1 secrets engine.
type KVv1 struct {
	client    *Client
	mountPath string
}

// NewKVv1 creates a new KV v1 helper.
func NewKVv1(client *Client, mountPath string) *KVv1 {
	return &KVv1{
		client:    client,
		mountPath: mountPath,
	}
}

// Write writes a secret to KV v1 with retry logic.
func (kv *KVv1) Write(ctx context.Context, path string, data map[string]any) error {
	fullPath := fmt.Sprintf("%s/%s", kv.mountPath, path)

	_, err := retryWithBackoff(ctx, fmt.Sprintf("write %s", fullPath), func() (*api.Secret, error) {
		return kv.client.client.Logical().Write(fullPath, data)
	})

	if err != nil {
		return fmt.Errorf("failed to write secret to %s: %w", fullPath, err)
	}

	return nil
}

// Read reads a secret from KV v1 with retry logic.
func (kv *KVv1) Read(ctx context.Context, path string) (map[string]any, error) {
	fullPath := fmt.Sprintf("%s/%s", kv.mountPath, path)

	secret, err := retryWithBackoff(ctx, fmt.Sprintf("read %s", fullPath), func() (*api.Secret, error) {
		return kv.client.client.Logical().Read(fullPath)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to read secret from %s: %w", fullPath, err)
	}

	if secret == nil || secret.Data == nil {
		return nil, fmt.Errorf("no secret found at %s", fullPath)
	}

	return secret.Data, nil
}

// Delete deletes a secret from KV v1 with retry logic.
func (kv *KVv1) Delete(ctx context.Context, path string) error {
	fullPath := fmt.Sprintf("%s/%s", kv.mountPath, path)

	_, err := retryWithBackoff(ctx, fmt.Sprintf("delete %s", fullPath), func() (*api.Secret, error) {
		return kv.client.client.Logical().Delete(fullPath)
	})

	if err != nil {
		return fmt.Errorf("failed to delete secret at %s: %w", fullPath, err)
	}

	return nil
}

// List lists secrets at a path in KV v1 with retry logic.
func (kv *KVv1) List(ctx context.Context, path string) ([]any, error) {
	fullPath := fmt.Sprintf("%s/%s", kv.mountPath, path)

	secret, err := retryWithBackoff(ctx, fmt.Sprintf("list %s", fullPath), func() (*api.Secret, error) {
		return kv.client.client.Logical().List(fullPath)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to list secrets at %s: %w", fullPath, err)
	}

	if secret == nil || secret.Data == nil {
		return []any{}, nil
	}

	keys, ok := secret.Data["keys"].([]any)
	if !ok {
		return []any{}, nil
	}

	return keys, nil
}

// Exists checks if a secret exists at the given path with retry logic.
func (kv *KVv1) Exists(ctx context.Context, path string) (bool, error) {
	fullPath := fmt.Sprintf("%s/%s", kv.mountPath, path)

	secret, err := retryWithBackoff(ctx, fmt.Sprintf("exists %s", fullPath), func() (*api.Secret, error) {
		return kv.client.client.Logical().Read(fullPath)
	})

	if err != nil {
		return false, fmt.Errorf("failed to check existence at %s: %w", fullPath, err)
	}

	return secret != nil && secret.Data != nil, nil
}
