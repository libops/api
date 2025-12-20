package account

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/libops/api/db"
)

// Repository contains shared business logic for account operations.
type Repository struct {
	db db.Querier
}

// NewRepository creates a new account repository.
func NewRepository(querier db.Querier) *Repository {
	return &Repository{
		db: querier,
	}
}

// GetAccountByEmail retrieves an account by email address.
func (r *Repository) GetAccountByEmail(ctx context.Context, email string) (db.GetAccountByEmailRow, error) {
	return r.db.GetAccountByEmail(ctx, email)
}

// GetAccountByPublicID retrieves an account by public ID.
func (r *Repository) GetAccountByPublicID(ctx context.Context, publicID uuid.UUID) (db.GetAccountRow, error) {
	return r.db.GetAccount(ctx, publicID.String())
}

// CreateAccount creates a new account.
func (r *Repository) CreateAccount(ctx context.Context, params db.CreateAccountParams) error {
	return r.db.CreateAccount(ctx, params)
}

// UpdateAccount updates an existing account.
func (r *Repository) UpdateAccount(ctx context.Context, params db.UpdateAccountParams) error {
	return r.db.UpdateAccount(ctx, params)
}

// DeleteAccount deletes an account.
func (r *Repository) DeleteAccount(ctx context.Context, publicID uuid.UUID) error {
	return r.db.DeleteAccount(ctx, publicID.String())
}

// ListAccounts lists accounts with pagination.
func (r *Repository) ListAccounts(ctx context.Context, params db.ListAccountsParams) ([]db.ListAccountsRow, error) {
	return r.db.ListAccounts(ctx, params)
}

// ListAccountOrganizations lists organizations accessible to an account.
func (r *Repository) ListAccountOrganizations(ctx context.Context, params db.ListAccountOrganizationsParams) ([]db.ListAccountOrganizationsRow, error) {
	return r.db.ListAccountOrganizations(ctx, params)
}

// Helper functions

// fromNullString extracts the string value from a sql.NullString, returning an empty string if not valid.
func fromNullString(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// fromNullStringPtr converts a sql.NullString to an optional pointer to a string, returning nil if not valid.
func fromNullStringPtr(ns sql.NullString) *string {
	if ns.Valid {
		return &ns.String
	}
	return nil
}

// stringValue extracts a string from a pointer, returning defaultVal if nil.
func stringValue(ptr *string, defaultVal string) string {
	if ptr != nil {
		return *ptr
	}
	return defaultVal
}

// =============================================================================
// API Key Management
// =============================================================================

// CreateAPIKeyParams contains parameters for creating an API key.
type CreateAPIKeyParams struct {
	PublicID    string
	AccountID   int64
	Name        string
	Description string
	Scopes      []string
	CreatedBy   int64
}

// GenerateAPIKey generates a cryptographically secure API key and its UUID.
// The UUID is stored in the database, and the key is stored in Vault via the auth layer.
func (r *Repository) GenerateAPIKey(ctx context.Context) (apiKey string, publicID string, err error) {
	// Generate public ID (UUID v7)
	publicID = uuid.Must(uuid.NewV7()).String()

	// Generate cryptographically secure random bytes for the API key
	keyBytes := make([]byte, 32) // 256 bits
	if _, err := rand.Read(keyBytes); err != nil {
		return "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode as base64 URL-safe for the API key
	// Format: libops_<base64>
	apiKey = "libops_" + base64.URLEncoding.WithPadding(base64.NoPadding).EncodeToString(keyBytes)

	// Note: The actual key storage in Vault will be handled by the auth.APIKeyManager
	// We just generate and return the key here
	return apiKey, publicID, nil
}

// CreateAPIKey creates a new API key in the database.
func (r *Repository) CreateAPIKey(ctx context.Context, params CreateAPIKeyParams) (time.Time, error) {
	// Marshal scopes to JSON
	var scopesJSON []byte
	if len(params.Scopes) > 0 {
		var err error
		scopesJSON, err = json.Marshal(params.Scopes)
		if err != nil {
			return time.Time{}, fmt.Errorf("failed to marshal scopes: %w", err)
		}
	}

	err := r.db.CreateAPIKey(ctx, db.CreateAPIKeyParams{
		PublicID:    params.PublicID,
		AccountID:   params.AccountID,
		Name:        params.Name,
		Description: toNullString(params.Description),
		Scopes:      scopesJSON,
		ExpiresAt:   sql.NullTime{Valid: false}, // No expiration
		Active:      true,
		CreatedBy:   toNullInt64(params.CreatedBy),
	})
	if err != nil {
		return time.Time{}, err
	}

	return time.Now(), nil
}

// ListAPIKeysByAccount lists all API keys for an account.
func (r *Repository) ListAPIKeysByAccount(ctx context.Context, accountID int64, limit, offset int32) ([]db.ListAPIKeysByAccountRow, error) {
	return r.db.ListAPIKeysByAccount(ctx, db.ListAPIKeysByAccountParams{
		AccountID: accountID,
		Limit:     limit,
		Offset:    offset,
	})
}

// GetAPIKeyByUUID retrieves an API key by its UUID.
func (r *Repository) GetAPIKeyByUUID(ctx context.Context, publicID string) (db.GetAPIKeyByUUIDRow, error) {
	return r.db.GetAPIKeyByUUID(ctx, publicID)
}

// RevokeAPIKey revokes an API key by setting active=false.
func (r *Repository) RevokeAPIKey(ctx context.Context, publicID string) error {
	return r.db.UpdateAPIKeyActive(ctx, db.UpdateAPIKeyActiveParams{
		PublicID: publicID,
		Active:   false,
	})
}

// Helper functions for API keys

// toNullString converts a string to sql.NullString.
func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}

// toNullInt64 converts an int64 to sql.NullInt64.
func toNullInt64(i int64) sql.NullInt64 {
	if i == 0 {
		return sql.NullInt64{Valid: false}
	}
	return sql.NullInt64{Int64: i, Valid: true}
}

// unmarshalScopes unmarshals JSON scopes to a string slice.
func unmarshalScopes(scopesJSON []byte) []string {
	if len(scopesJSON) == 0 {
		return []string{}
	}

	var scopes []string
	if err := json.Unmarshal(scopesJSON, &scopes); err != nil {
		return []string{}
	}
	return scopes
}
