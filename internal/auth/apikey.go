// Package auth provides authentication and authorization functionality including
// OIDC, API keys, JWT validation, and role-based access control.
package auth

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/google/uuid"

	"github.com/libops/api/internal/audit"
	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/vault"
)

// APIKeyManager handles API key creation, validation, and management.
type APIKeyManager struct {
	vaultClient *vault.Client
	keysStore   *vault.KeysStore
	db          db.Querier
	auditLogger *audit.Logger
}

// NewAPIKeyManager creates a new API key manager.
func NewAPIKeyManager(vaultClient *vault.Client, querier db.Querier, auditLogger *audit.Logger) *APIKeyManager {
	return &APIKeyManager{
		vaultClient: vaultClient,
		keysStore:   vault.NewKeysStore(vaultClient),
		db:          querier,
		auditLogger: auditLogger,
	}
}

// CreateAPIKey creates a new API key for an account.
// It returns the key secret value (which is only shown once) and the API key's metadata.
// The 'scopes' parameter is a required list of OAuth scope strings (e.g., ["read:organization", "write:site"]).
func (akm *APIKeyManager) CreateAPIKey(ctx context.Context, accountID int64, accountUUID, name, description string, scopes []string, expiresAt *time.Time, createdBy int64) (string, *db.GetAPIKeyByUUIDRow, error) {
	// UUID v4 has extremely low collision probability (2^122 possible values)
	// Database UNIQUE constraint on api_key_uuid provides atomic collision detection
	keyUUID := uuid.New()

	// Format: libops_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxx (prefix + 43 random chars, 32 bytes = 256 bits)
	// The secret is derived from the UUID + random bytes for maximum entropy
	secretBytes := make([]byte, 32)
	if _, err := rand.Read(secretBytes); err != nil {
		return "", nil, fmt.Errorf("failed to generate random secret: %w", err)
	}
	secretValue := "libops_" + base64.RawURLEncoding.EncodeToString(secretBytes)

	var expiresAtSQL sql.NullTime
	if expiresAt != nil {
		expiresAtSQL = sql.NullTime{Time: *expiresAt, Valid: true}
	}

	var scopesJSON json.RawMessage
	if len(scopes) > 0 {
		scopesBytes, err := json.Marshal(scopes)
		if err != nil {
			return "", nil, fmt.Errorf("failed to marshal scopes: %w", err)
		}
		scopesJSON = scopesBytes
	}

	// The UNIQUE constraint on api_key_uuid provides atomic collision detection
	// If a duplicate UUID is generated (extremely unlikely), the INSERT will fail
	err := akm.db.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		PublicID:  keyUUID.String(),
		AccountID: accountID,
		Name:      name,
		Description: sql.NullString{
			String: description,
			Valid:  description != "",
		},
		Scopes:    scopesJSON,
		ExpiresAt: expiresAtSQL,
		Active:    true,
		CreatedBy: sql.NullInt64{
			Int64: createdBy,
			Valid: createdBy > 0,
		},
	})
	if err != nil {
		// In the extremely unlikely event of UUID collision, retry once
		if isDuplicateKeyError(err) {
			slog.Warn("UUID collision detected (extremely rare), retrying with new UUID",
				"uuid", keyUUID)
			// Retry with a new UUID - recursive call with depth limit would be better
			// but for UUID v4 collisions, a single retry is sufficient
			return akm.CreateAPIKey(ctx, accountID, accountUUID, name, description, scopes, expiresAt, createdBy)
		}
		return "", nil, fmt.Errorf("failed to create API key in database: %w", err)
	}

	// Store in Vault: keys/{secretValue} -> {account_uuid, api_key_uuid}
	err = akm.keysStore.StoreKey(ctx, secretValue, accountUUID, keyUUID.String())
	if err != nil {
		_ = akm.db.DeleteAPIKey(ctx, keyUUID.String())
		return "", nil, fmt.Errorf("failed to store API key in Vault: %w", err)
	}

	// Fetch the created key to return metadata
	keyMeta, err := akm.db.GetAPIKeyByUUID(ctx, keyUUID.String())
	if err != nil {
		return "", nil, fmt.Errorf("failed to fetch created API key: %w", err)
	}

	akm.auditLogger.Log(ctx, accountID, keyMeta.ID, audit.APIKeyEntityType, audit.APIKeyCreate, map[string]any{
		"name": name,
	})

	return secretValue, &keyMeta, nil
}

// ValidateAPIKey validates an API key secret and returns account information.
// It also updates the last_used_at timestamp for the API key.
func (akm *APIKeyManager) ValidateAPIKey(ctx context.Context, secretValue string) (*APIKeyInfo, error) {
	slog.Info("ValidateAPIKey: starting validation", "token", secretValue)

	keySecret, err := akm.keysStore.GetKeyBySecret(ctx, secretValue)
	if err != nil {
		slog.Error("ValidateAPIKey: Vault lookup failed", "error", err)
		return nil, fmt.Errorf("invalid API key")
	}
	slog.Info("ValidateAPIKey: Vault lookup success",
		"api_key_uuid", keySecret.APIKeyUUID,
		"account_uuid", keySecret.AccountUUID)

	keyUUID, err := uuid.Parse(keySecret.APIKeyUUID)
	if err != nil {
		slog.Error("ValidateAPIKey: invalid API key UUID format", "error", err)
		return nil, fmt.Errorf("invalid API key")
	}

	keyMeta, err := akm.db.GetActiveAPIKeyByUUID(ctx, keyUUID.String())
	if err != nil {
		slog.Error("ValidateAPIKey: DB key lookup failed", "error", err, "uuid", keySecret.APIKeyUUID)
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid API key")
		}
		return nil, fmt.Errorf("failed to validate API key: %w", err)
	}
	slog.Info("ValidateAPIKey: DB key lookup success", "key_id", keyMeta.ID)

	accountUUID, err := uuid.Parse(keySecret.AccountUUID)
	if err != nil {
		slog.Error("ValidateAPIKey: invalid account UUID format", "error", err)
		return nil, fmt.Errorf("invalid API key")
	}

	account, err := akm.db.GetAccount(ctx, accountUUID.String())
	if err != nil {
		slog.Error("ValidateAPIKey: DB account lookup failed", "error", err, "account_uuid", accountUUID)
		return nil, fmt.Errorf("invalid API key")
	}
	slog.Info("ValidateAPIKey: DB account lookup success", "account_id", account.ID)

	// Update last_used_at in background (don't fail request if this fails)
	go func() {
		_ = akm.db.UpdateAPIKeyLastUsed(context.Background(), keyUUID.String())
	}()

	var scopes []Scope
	if len(keyMeta.Scopes) > 0 {
		var scopeStrings []string
		if err := json.Unmarshal(keyMeta.Scopes, &scopeStrings); err != nil {
			slog.Error("ValidateAPIKey: failed to unmarshal API key scopes", "error", err)
			return nil, fmt.Errorf("invalid API key")
		}
		// API keys store scopes in structured format (resource:level), not OAuth format
		// Parse them directly instead of using OAuth mapping
		parsedScopes, err := ParseScopes(scopeStrings)
		if err != nil {
			slog.Error("ValidateAPIKey: failed to parse API key scopes", "error", err, "scopes", scopeStrings)
			return nil, fmt.Errorf("invalid API key")
		}
		scopes = parsedScopes
	}
	// Empty scopes means no scope restrictions - authorization will be based on membership/roles

	return &APIKeyInfo{
		KeyUUID:     keySecret.APIKeyUUID,
		AccountID:   account.ID,
		AccountUUID: account.PublicID,
		Email:       account.Email,
		Name:        account.Name.String,
		EntityID:    account.VaultEntityID.String,
		KeyName:     keyMeta.Name,
		Scopes:      scopes,
	}, nil
}

// APIKeyInfo represents validated API key information.
type APIKeyInfo struct {
	KeyUUID     string
	AccountID   int64
	AccountUUID string
	Email       string
	Name        string
	EntityID    string
	KeyName     string
	Scopes      []Scope // Permission scopes granted to this API key
}

// ListAPIKeys lists all API keys for an account.
func (akm *APIKeyManager) ListAPIKeys(ctx context.Context, accountID int64) ([]db.ListAPIKeysByAccountRow, error) {
	return akm.db.ListAPIKeysByAccount(ctx, db.ListAPIKeysByAccountParams{
		AccountID: accountID,
		Limit:     1000, // Return all keys for now
		Offset:    0,
	})
}

// DeactivateAPIKey deactivates an API key.
func (akm *APIKeyManager) DeactivateAPIKey(ctx context.Context, keyUUID string) error {
	keyUUIDParsed, err := uuid.Parse(keyUUID)
	if err != nil {
		return fmt.Errorf("invalid key UUID: %w", err)
	}
	return akm.db.UpdateAPIKeyActive(ctx, db.UpdateAPIKeyActiveParams{
		Active:   false,
		PublicID: keyUUIDParsed.String(),
	})
}

// DeleteAPIKey completely removes an API key from database and Vault.
func (akm *APIKeyManager) DeleteAPIKey(ctx context.Context, keyUUID string) error {
	key, err := akm.GetAPIKey(ctx, keyUUID)
	if err != nil {
		return err
	}

	if err := akm.keysStore.DeleteKey(ctx, keyUUID); err != nil {
		// If the key is not in Vault, we can still try to delete it from the database.
		// Log the error and continue.
		slog.Warn("failed to delete API key from Vault, will attempt to delete from database", "err", err)
	}

	keyUUIDParsed, err := uuid.Parse(keyUUID)
	if err != nil {
		return fmt.Errorf("invalid key UUID: %w", err)
	}

	err = akm.db.DeleteAPIKey(ctx, keyUUIDParsed.String())
	if err != nil {
		return fmt.Errorf("failed to delete API key from database: %w", err)
	}

	akm.auditLogger.Log(ctx, key.AccountID, key.ID, audit.APIKeyEntityType, audit.APIKeyDelete, nil)

	return nil
}

// GetAPIKey gets API key metadata by UUID.
func (akm *APIKeyManager) GetAPIKey(ctx context.Context, keyUUID string) (*db.GetAPIKeyByUUIDRow, error) {
	keyUUIDParsed, err := uuid.Parse(keyUUID)
	if err != nil {
		return nil, fmt.Errorf("invalid key UUID: %w", err)
	}
	key, err := akm.db.GetAPIKeyByUUID(ctx, keyUUIDParsed.String())
	if err != nil {
		return nil, fmt.Errorf("invalid API key")
	}
	return &key, nil
}

// isDuplicateKeyError checks if an error is a MySQL duplicate key error (1062).
func isDuplicateKeyError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062 // Duplicate entry
	}
	return false
}
