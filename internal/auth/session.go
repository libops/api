package auth

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/libops/api/db"
)

// SessionManager handles session management with cookies and tokens.
type SessionManager struct {
	db            db.Querier
	cookieDomain  string
	secureCookies bool
}

// NewSessionManager creates a new session manager.
func NewSessionManager(querier db.Querier, cookieDomain string, secureCookies bool) *SessionManager {
	return &SessionManager{
		db:            querier,
		cookieDomain:  cookieDomain,
		secureCookies: secureCookies,
	}
}

// SetSessionCookies sets both the access token and ID token cookies.
func (sm *SessionManager) SetSessionCookies(w http.ResponseWriter, accessToken, idToken string, expiresIn int) {
	maxAge := expiresIn
	if maxAge == 0 {
		maxAge = 3600 // Default to 1 hour
	}

	slog.Debug("Setting session cookies",
		"domain", sm.cookieDomain,
		"secure", sm.secureCookies,
		"maxAge", maxAge)

	http.SetCookie(w, &http.Cookie{
		Name:     "vault_token",
		Value:    accessToken,
		Path:     "/",
		Domain:   sm.cookieDomain,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   sm.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})

	http.SetCookie(w, &http.Cookie{
		Name:     "id_token",
		Value:    idToken,
		Path:     "/",
		Domain:   sm.cookieDomain,
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   sm.secureCookies,
		SameSite: http.SameSiteLaxMode,
	})
}

// ClearSessionCookies removes all session cookies.
func (sm *SessionManager) ClearSessionCookies(w http.ResponseWriter) {
	cookies := []string{"vault_token", "id_token", "access_token"}

	for _, name := range cookies {
		cookie := &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			Domain:   sm.cookieDomain,
			MaxAge:   -1,
			HttpOnly: true,
			Secure:   sm.secureCookies,
			SameSite: http.SameSiteLaxMode,
		}

		http.SetCookie(w, cookie)
	}
}

// GetVaultTokenFromCookie retrieves the Vault token from cookies.
func (sm *SessionManager) GetVaultTokenFromCookie(r *http.Request) (string, error) {
	cookie, err := r.Cookie("vault_token")
	if err != nil {
		return "", fmt.Errorf("vault token cookie not found: %w", err)
	}
	return cookie.Value, nil
}

// GetIDTokenFromCookie retrieves the ID token from cookies.
func (sm *SessionManager) GetIDTokenFromCookie(r *http.Request) (string, error) {
	cookie, err := r.Cookie("id_token")
	if err != nil {
		return "", fmt.Errorf("id token cookie not found: %w", err)
	}
	return cookie.Value, nil
}
