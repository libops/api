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
	"strings"
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

	// Generate a random secret component (64 bytes = 512 bits of entropy)
	randomSecret, err := generateRandomSecret(64)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate random secret: %w", err)
	}

	// Format: libops_{accountUUID_no_dashes}_{keyUUID_no_dashes}_{randomSecret}
	secretValue := formatAPIKeySecret(accountUUID, keyUUID.String(), randomSecret)

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
	err = akm.db.CreateAPIKey(ctx, db.CreateAPIKeyParams{
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

	// Store the random secret in Vault at keys/{accountUUID}/{keyUUID}
	// This allows us to validate that the user knows the secret during auth
	err = akm.keysStore.StoreKey(ctx, accountUUID, keyUUID.String(), randomSecret)
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
	// Parse embedded UUIDs and random secret from the API key
	accountUUID, keyUUID, randomSecret, err := parseAPIKeySecret(secretValue)
	if err != nil {
		slog.Error("ValidateAPIKey: failed to parse API key format", "error", err)
		return nil, fmt.Errorf("invalid API key")
	}

	// Verify the random secret matches what's stored in Vault at keys/{accountUUID}/{keyUUID}
	storedSecret, err := akm.keysStore.GetKeySecret(ctx, accountUUID, keyUUID)
	if err != nil {
		slog.Error("ValidateAPIKey: Vault lookup failed", "error", err, "key_uuid", keyUUID, "account_uuid", accountUUID)
		return nil, fmt.Errorf("invalid API key")
	}
	if storedSecret != randomSecret {
		slog.Error("ValidateAPIKey: secret mismatch", "key_uuid", keyUUID)
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

// DeactivateAPIKey deactivates an API key and removes it from Vault.
// This prevents the key from being used for authentication.
func (akm *APIKeyManager) DeactivateAPIKey(ctx context.Context, keyUUID string) error {
	keyUUIDParsed, err := uuid.Parse(keyUUID)
	if err != nil {
		return fmt.Errorf("invalid key UUID: %w", err)
	}

	// Get the key to retrieve the account UUID for Vault deletion
	key, err := akm.db.GetAPIKeyByUUID(ctx, keyUUIDParsed.String())
	if err != nil {
		return fmt.Errorf("failed to get API key: %w", err)
	}

	// Get the account UUID
	account, err := akm.db.GetAccountByID(ctx, key.AccountID)
	if err != nil {
		return fmt.Errorf("failed to get account: %w", err)
	}

	// Delete from Vault first (if it fails, the key remains active in DB)
	if err := akm.keysStore.DeleteKey(ctx, account.PublicID, keyUUIDParsed.String()); err != nil {
		slog.Error("failed to delete API key from Vault", "key_uuid", keyUUID, "account_uuid", account.PublicID, "error", err)
		// Continue anyway - the DB deactivation is the source of truth
	}

	// Deactivate in database
	return akm.db.UpdateAPIKeyActive(ctx, db.UpdateAPIKeyActiveParams{
		Active:   false,
		PublicID: keyUUIDParsed.String(),
	})
}

// DeleteAPIKey removes an API key from both the database and Vault.
func (akm *APIKeyManager) DeleteAPIKey(ctx context.Context, keyUUID string) error {
	key, err := akm.GetAPIKey(ctx, keyUUID)
	if err != nil {
		return err
	}

	keyUUIDParsed, err := uuid.Parse(keyUUID)
	if err != nil {
		return fmt.Errorf("invalid key UUID: %w", err)
	}

	// Get the account UUID for Vault deletion
	account, err := akm.db.GetAccountByID(ctx, key.AccountID)
	if err != nil {
		return fmt.Errorf("failed to get account: %w", err)
	}

	// Delete from Vault first
	if err := akm.keysStore.DeleteKey(ctx, account.PublicID, keyUUIDParsed.String()); err != nil {
		slog.Error("failed to delete API key from Vault", "key_uuid", keyUUID, "account_uuid", account.PublicID, "error", err)
		// Continue anyway - the DB deletion is the source of truth
	}

	// Delete from database
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
// libops_{accountUUID_no_dashes}_{keyUUID_no_dashes}_{randomSecret}
// Example: libops_01052d4d93be51a39684c357297533cd_075913e793285264b6846ae0163b8096_Kq3xY9zT...
func formatAPIKeySecret(accountUUID, keyUUID, randomSecret string) string {
	accountUUIDNoDashes := stripUUIDDashes(accountUUID)
	keyUUIDNoDashes := stripUUIDDashes(keyUUID)
	return fmt.Sprintf("libops_%s_%s_%s", accountUUIDNoDashes, keyUUIDNoDashes, randomSecret)
}

// parseAPIKeySecret extracts the account UUID, key UUID, and random secret from an API key secret.
// Format: libops_{accountUUID_no_dashes}_{keyUUID_no_dashes}_{randomSecret}
// Returns the UUIDs in standard format (with dashes) and the random secret.
func parseAPIKeySecret(secretValue string) (accountUUID, keyUUID, randomSecret string, err error) {
	parts := splitAPIKeySecret(secretValue)
	if len(parts) < 4 || parts[0] != "libops" {
		return "", "", "", fmt.Errorf("invalid API key format: expected libops_{accountUUID}_{keyUUID}_{secret}")
	}

	accountUUID, err = formatUUIDWithDashes(parts[1])
	if err != nil {
		return "", "", "", fmt.Errorf("invalid account UUID: %w", err)
	}

	keyUUID, err = formatUUIDWithDashes(parts[2])
	if err != nil {
		return "", "", "", fmt.Errorf("invalid key UUID: %w", err)
	}

	randomSecret = strings.Join(parts[3:], "_")
	if len(randomSecret) == 0 {
		return "", "", "", fmt.Errorf("invalid API key format: missing random secret")
	}

	return accountUUID, keyUUID, randomSecret, nil
}

// stripUUIDDashes removes dashes from a UUID and converts to lowercase.
func stripUUIDDashes(uuidStr string) string {
	return strings.ToLower(strings.ReplaceAll(uuidStr, "-", ""))
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
// Expected format: libops_{keyUUID}_{accountUUID}_{secret} (4 parts, 3 underscores)
func splitAPIKeySecret(secretValue string) []string {
	return strings.Split(secretValue, "_")
}

// generateRandomSecret generates a cryptographically secure random secret.
// The secret is base64url encoded (URL-safe, no padding) for easy transmission.
func generateRandomSecret(numBytes int) (string, error) {
	bytes := make([]byte, numBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	// Use URL-safe base64 encoding without padding
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// isDuplicateKeyError checks if an error is a MySQL duplicate key error (1062).
func isDuplicateKeyError(err error) bool {
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	return false
}
