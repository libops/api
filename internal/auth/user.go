package auth

import "context"

// ContextKey is the type for context keys.
type ContextKey string

const (
	// UserContextKey is the key for storing user info in context.
	UserContextKey ContextKey = "user"
)

// UserInfo represents authenticated user information from Vault.
type UserInfo struct {
	EntityID  string // Vault entity ID (stable across all auth methods)
	Email     string
	Name      string
	AccountID int64
	Metadata  map[string]string
	Scopes    []Scope
}

// GetUserFromContext extracts user info from request context.
func GetUserFromContext(ctx context.Context) (*UserInfo, bool) {
	user, ok := ctx.Value(UserContextKey).(*UserInfo)
	return user, ok
}

// ExtractAccountIDFromContext extracts just the account ID from context
// This is a convenience function for audit logging.
func ExtractAccountIDFromContext(ctx context.Context) (int64, bool) {
	userInfo, ok := GetUserFromContext(ctx)
	if !ok || userInfo == nil {
		return 0, false
	}
	return userInfo.AccountID, true
}
