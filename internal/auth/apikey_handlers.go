package auth

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

const (
	// maxNameLength defines the maximum allowed length for an API key name.
	maxNameLength = 255

	// maxDescriptionLength defines the maximum allowed length for an API key description.
	maxDescriptionLength = 1024
)

// HandleCreateAPIKey creates a new API key for the authenticated user.
func (akm *APIKeyManager) HandleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Scopes      []string `json:"scopes"`     // Optional: OAuth scope strings
		ExpiresIn   *int     `json:"expires_in"` // Optional: days until expiration
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if len(req.Name) > maxNameLength {
		http.Error(w, fmt.Sprintf("name too long (max %d chars)", maxNameLength), http.StatusBadRequest)
		return
	}
	if !isValidAPIKeyName(req.Name) {
		http.Error(w, "name contains invalid characters (alphanumeric, spaces, and dashes only)", http.StatusBadRequest)
		return
	}

	if len(req.Description) > maxDescriptionLength {
		http.Error(w, fmt.Sprintf("description too long (max %d chars)", maxDescriptionLength), http.StatusBadRequest)
		return
	}
	if !isValidAPIKeyDescription(req.Description) {
		http.Error(w, "description contains invalid characters (printable ASCII only)", http.StatusBadRequest)
		return
	}

	var expiresAt *time.Time
	if req.ExpiresIn != nil && *req.ExpiresIn > 0 {
		exp := time.Now().AddDate(0, 0, *req.ExpiresIn)
		expiresAt = &exp
	}

	account, err := akm.db.GetAccountByEmail(r.Context(), userInfo.Email)
	if err != nil {
		slog.Error("Failed to get account", "err", err, "email", userInfo.Email)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	secretValue, keyMeta, err := akm.CreateAPIKey(
		r.Context(),
		account.ID,
		account.PublicID,
		req.Name,
		req.Description,
		req.Scopes,
		expiresAt,
		account.ID,
	)
	if err != nil {
		slog.Error("Failed to create API key", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	if err := json.NewEncoder(w).Encode(map[string]any{
		"secret":      secretValue,
		"api_key_id":  keyMeta.PublicID,
		"name":        keyMeta.Name,
		"description": keyMeta.Description.String,
		"created_at":  keyMeta.CreatedAt,
		"expires_at":  keyMeta.ExpiresAt.Time,
		"active":      keyMeta.Active,
		"warning":     "Save this secret now - it will not be shown again",
	}); err != nil {
		slog.Error("Failed to encode response", "err", err)
	}
}

// HandleListAPIKeys lists all API keys for the authenticated user.
func (akm *APIKeyManager) HandleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	account, err := akm.db.GetAccountByEmail(r.Context(), userInfo.Email)
	if err != nil {
		slog.Error("Failed to get account", "err", err, "email", userInfo.Email)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	keys, err := akm.ListAPIKeys(r.Context(), account.ID)
	if err != nil {
		slog.Error("Failed to list API keys", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{
		"api_keys": keys,
	}); err != nil {
		slog.Error("Failed to encode response", "err", err)
	}
}

// HandleDeleteAPIKey deletes an API key.
func (akm *APIKeyManager) HandleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	userInfo, ok := GetUserFromContext(r.Context())
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	keyUUID := r.PathValue("key_id")
	if keyUUID == "" {
		http.Error(w, "key_id required", http.StatusBadRequest)
		return
	}

	key, err := akm.GetAPIKey(r.Context(), keyUUID)
	if err != nil {
		slog.Error("Failed to get API key", "err", err, "key_id", keyUUID)
		http.Error(w, "Not Found", http.StatusNotFound)
		return
	}

	account, err := akm.db.GetAccountByEmail(r.Context(), userInfo.Email)
	if err != nil {
		slog.Error("Failed to get account", "err", err, "email", userInfo.Email)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if key.AccountID != account.ID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if err := akm.DeleteAPIKey(r.Context(), keyUUID); err != nil {
		slog.Error("Failed to delete API key", "err", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// isValidAPIKeyName checks if the name contains only alphanumeric, spaces, and dashes.
func isValidAPIKeyName(name string) bool {
	for _, r := range name {
		if (r < 'a' || r > 'z') &&
			(r < 'A' || r > 'Z') &&
			(r < '0' || r > '9') &&
			r != ' ' && r != '-' {
			return false
		}
	}
	return true
}

// isValidAPIKeyDescription checks if the description contains only printable ASCII characters.
func isValidAPIKeyDescription(description string) bool {
	for _, r := range description {
		if r < 32 || r > 126 { // Printable ASCII range
			return false
		}
	}
	return true
}
