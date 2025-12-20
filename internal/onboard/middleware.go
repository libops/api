package onboard

import (
	"log/slog"
	"net/http"

	"github.com/libops/api/db"
	"github.com/libops/api/internal/auth"
)

// Middleware provides middleware functions for onboarding
type Middleware struct {
	db db.Querier
}

// NewMiddleware creates a new onboarding middleware
func NewMiddleware(querier db.Querier) *Middleware {
	return &Middleware{db: querier}
}

// RequireOnboardingComplete redirects to /onboarding if user hasn't completed onboarding
func (m *Middleware) RequireOnboardingComplete(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get user from context (set by auth middleware)
		userInfo, ok := auth.GetUserFromContext(r.Context())
		if !ok {
			// Not authenticated - let auth middleware handle it
			next.ServeHTTP(w, r)
			return
		}

		// Get account
		account, err := m.db.GetAccountByID(r.Context(), userInfo.AccountID)
		if err != nil {
			slog.Error("Failed to get account for onboarding check", "account_id", userInfo.AccountID, "error", err)
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		// Check if onboarding is complete
		if !account.OnboardingCompleted {
			// Redirect to onboarding
			http.Redirect(w, r, "/onboarding", http.StatusSeeOther)
			return
		}

		// Onboarding complete, proceed
		next.ServeHTTP(w, r)
	})
}
