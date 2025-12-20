package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/libops/api/db"
)

// GSAMiddleware validates that the request is from a valid site GSA.
type GSAMiddleware struct {
	queries db.Querier
}

// NewGSAMiddleware creates a new GSA authentication middleware.
func NewGSAMiddleware(queries db.Querier) *GSAMiddleware {
	return &GSAMiddleware{
		queries: queries,
	}
}

// Middleware validates GSA authentication for site reconciliation endpoints.
func (m *GSAMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the authenticated service account from the request context
		// This is populated by the JWT validator middleware
		email := getServiceAccountEmail(r)
		if email == "" {
			slog.Warn("GSA auth failed: no service account email in request")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Extract site ID from the path
		siteID := r.PathValue("siteId")
		if siteID == "" {
			// For project endpoints, we don't validate GSA format
			// since projects don't have their own GSAs
			next.ServeHTTP(w, r)
			return
		}

		// Validate that the GSA matches the expected format for this site
		if !m.validateSiteGSA(r.Context(), email, siteID) {
			slog.Warn("GSA auth failed: invalid service account for site",
				"email", email,
				"site_id", siteID)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// validateSiteGSA checks if the GSA email matches the expected format for the site.
func (m *GSAMiddleware) validateSiteGSA(ctx context.Context, email, siteID string) bool {
	// Get the site from the database
	site, err := m.queries.GetSite(ctx, siteID)
	if err != nil {
		slog.Error("failed to get site for GSA validation", "site_id", siteID, "error", err)
		return false
	}

	// Get the project to get the GCP project ID
	project, err := m.queries.GetProjectByID(ctx, site.ProjectID)
	if err != nil {
		slog.Error("failed to get project for GSA validation", "project_id", site.ProjectID, "error", err)
		return false
	}

	// Construct expected GSA email: vm-${uuid_short}@${gcp_project_id}.iam.gserviceaccount.com
	uuidShort := siteID[:8]
	expectedEmail := fmt.Sprintf("vm-%s@%s.iam.gserviceaccount.com", uuidShort, project.GcpProjectID.String)

	return email == expectedEmail
}

// getServiceAccountEmail extracts the service account email from the request context.
// This should be populated by the JWT validator middleware.
func getServiceAccountEmail(r *http.Request) string {
	// Get user info from context (set by JWT middleware)
	userInfo, ok := GetUserFromContext(r.Context())
	if !ok || userInfo == nil {
		return ""
	}

	return userInfo.Email
}
