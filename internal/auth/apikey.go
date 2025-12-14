// Package auth provides authentication and authorization functionality including
// OIDC, API keys, JWT validation, and role-based access control.
package auth

import (
	"context"
	"database/sql"
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
	keyUUID := uuid.New()

	// Format: libops_{keyUUID_no_dashes}_{accountUUID_no_dashes}
	// The secret itself encodes the metadata, no additional random component needed
	secretValue := formatAPIKeySecret(keyUUID.String(), accountUUID)

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

	// Store in Vault: keys/{secretValue} (no additional metadata needed - it's in the key)
	err = akm.keysStore.StoreKey(ctx, secretValue)
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
	// Parse embedded UUIDs from the API key secret
	keyUUID, accountUUID, err := parseAPIKeySecret(secretValue)
	if err != nil {
		slog.Error("ValidateAPIKey: failed to parse API key format", "error", err)
		return nil, fmt.Errorf("invalid API key")
	}

	// Verify the key exists in Vault (confirms it hasn't been deleted)
	exists, err := akm.keysStore.KeyExists(ctx, secretValue)
	if err != nil {
		slog.Error("ValidateAPIKey: Vault lookup failed", "error", err)
		return nil, fmt.Errorf("invalid API key")
	}
	if !exists {
		slog.Error("ValidateAPIKey: API key not found in Vault", "key_uuid", keyUUID)
		return nil, fmt.Errorf("invalid API key")
	}

	// Verify the key is active in the database
	keyMeta, err := akm.db.GetActiveAPIKeyByUUID(ctx, keyUUID)
	if err != nil {
		slog.Error("ValidateAPIKey: DB key lookup failed", "error", err, "uuid", keyUUID)
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid API key")
		}
		return nil, fmt.Errorf("failed to validate API key: %w", err)
	}

	// Verify the account exists and matches the embedded account UUID
	account, err := akm.db.GetAccount(ctx, accountUUID)
	if err != nil {
		slog.Error("ValidateAPIKey: DB account lookup failed", "error", err, "account_uuid", accountUUID)
		return nil, fmt.Errorf("invalid API key")
	}

	// Update last_used_at in background (don't fail request if this fails)
	go func() {
		_ = akm.db.UpdateAPIKeyLastUsed(context.Background(), keyUUID)
	}()

	var scopes []Scope
	if len(keyMeta.Scopes) > 0 {
		var scopeStrings []string
		if err := json.Unmarshal(keyMeta.Scopes, &scopeStrings); err != nil {
			slog.Error("ValidateAPIKey: failed to unmarshal API key scopes", "error", err)
			return nil, fmt.Errorf("invalid API key")
		}
		parsedScopes, err := ParseScopes(scopeStrings)
		if err != nil {
			slog.Error("ValidateAPIKey: failed to parse API key scopes", "error", err, "scopes", scopeStrings)
			return nil, fmt.Errorf("invalid API key")
		}
		scopes = parsedScopes
	}

	return &APIKeyInfo{
		KeyUUID:     keyUUID,
		AccountID:   account.ID,
		AccountUUID: accountUUID,
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

// DeleteAPIKey removes an API key from the database.
// Note: The corresponding Vault entry cannot be deleted without the secret value,
// which is only shown once at creation. Orphaned Vault entries are cleaned up
// by a background job that compares Vault keys with active database records.
func (akm *APIKeyManager) DeleteAPIKey(ctx context.Context, keyUUID string) error {
	key, err := akm.GetAPIKey(ctx, keyUUID)
	if err != nil {
		return err
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

// formatAPIKeySecret creates an API key in the format:
// libops_{keyUUID_no_dashes}_{accountUUID_no_dashes}
// Example: libops_075913e793285264b6846ae0163b8096_01052d4d93be51a39684c357297533cd
func formatAPIKeySecret(keyUUID, accountUUID string) string {
	keyUUIDNoDashes := stripUUIDDashes(keyUUID)
	accountUUIDNoDashes := stripUUIDDashes(accountUUID)
	return fmt.Sprintf("libops_%s_%s", keyUUIDNoDashes, accountUUIDNoDashes)
}

// parseAPIKeySecret extracts the key UUID and account UUID from an API key secret.
// Format: libops_{keyUUID_no_dashes}_{accountUUID_no_dashes}
// Returns the UUIDs in standard format (with dashes).
func parseAPIKeySecret(secretValue string) (keyUUID, accountUUID string, err error) {
	parts := splitAPIKeySecret(secretValue)
	if len(parts) != 3 || parts[0] != "libops" {
		return "", "", fmt.Errorf("invalid API key format: expected libops_{keyUUID}_{accountUUID}")
	}

	keyUUID, err = formatUUIDWithDashes(parts[1])
	if err != nil {
		return "", "", fmt.Errorf("invalid key UUID: %w", err)
	}

	accountUUID, err = formatUUIDWithDashes(parts[2])
	if err != nil {
		return "", "", fmt.Errorf("invalid account UUID: %w", err)
	}

	return keyUUID, accountUUID, nil
}

// stripUUIDDashes removes dashes from a UUID and converts to lowercase.
func stripUUIDDashes(uuidStr string) string {
	result := make([]byte, 0, 32)
	for i := 0; i < len(uuidStr); i++ {
		c := uuidStr[i]
		if c != '-' {
			if c >= 'A' && c <= 'Z' {
				c = c + ('a' - 'A')
			}
			result = append(result, c)
		}
	}
	return string(result)
}

// formatUUIDWithDashes adds dashes to a UUID string (32 hex chars) to standard format.
// Format: xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx
func formatUUIDWithDashes(noDashes string) (string, error) {
	if len(noDashes) != 32 {
		return "", fmt.Errorf("UUID must be 32 characters without dashes")
	}
	return fmt.Sprintf("%s-%s-%s-%s-%s",
		noDashes[0:8],
		noDashes[8:12],
		noDashes[12:16],
		noDashes[16:20],
		noDashes[20:32],
	), nil
}

// splitAPIKeySecret splits the secret on underscores.
// Expected format: libops_{keyUUID}_{accountUUID} (3 parts, 2 underscores)
func splitAPIKeySecret(secretValue string) []string {
	parts := make([]string, 0, 3)
	start := 0
	underscoreCount := 0

	for i := 0; i < len(secretValue); i++ {
		if secretValue[i] == '_' {
			parts = append(parts, secretValue[start:i])
			start = i + 1
			underscoreCount++
		}
	}
	if start < len(secretValue) {
		parts = append(parts, secretValue[start:])
	}
	return parts
}

// isDuplicateKeyError checks if an error is a MySQL duplicate key error (1062).
func isDuplicateKeyError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return false
}
