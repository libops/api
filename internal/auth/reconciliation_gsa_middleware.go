package auth

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/libops/api/db"
)

// contextKey is a custom type for context keys to avoid collisions
type contextKey string

const reconciliationScopeKey contextKey = "reconciliation_scope"

// ReconciliationGSAMiddleware validates that the request is from a valid reconciliation service GSA.
type ReconciliationGSAMiddleware struct {
	queries db.Querier
}

// ReconciliationScope represents the scope of a reconciliation service.
type ReconciliationScope struct {
	Scope           string // "bootstrap", "org", "project"
	ResourceIDShort string // First 8 chars of UUID (for org/project scopes)
	GCPProjectID    string // GCP project ID
	OrganizationID  *int64 // Database organization ID (for org/project scopes)
	ProjectID       *int64 // Database project ID (for project scope)
}

// NewReconciliationGSAMiddleware creates a new reconciliation GSA authentication middleware.
func NewReconciliationGSAMiddleware(queries db.Querier) *ReconciliationGSAMiddleware {
	return &ReconciliationGSAMiddleware{
		queries: queries,
	}
}

// Middleware validates GSA authentication for reconciliation service endpoints.
func (m *ReconciliationGSAMiddleware) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the authenticated service account from the request context
		// This is populated by the JWT validator middleware
		email := getServiceAccountEmail(r)
		if email == "" {
			slog.Warn("reconciliation GSA auth failed: no service account email in request")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Parse and validate the reconciliation GSA email
		scope, err := m.parseAndValidateReconciliationGSA(r.Context(), email)
		if err != nil {
			slog.Warn("reconciliation GSA auth failed",
				"email", email,
				"error", err)
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}

		// Store scope info in request context for authorization
		ctx := context.WithValue(r.Context(), reconciliationScopeKey, scope)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// parseAndValidateReconciliationGSA parses the GSA email and validates it against the database.
// Expected formats:
// - reconciliation-bootstrap@{libops-project-id}.iam.gserviceaccount.com
// - reconciliation-org-{org_uuid_short}@{org_gcp_project_id}.iam.gserviceaccount.com
// - reconciliation-project-{project_uuid_short}@{project_gcp_project_id}.iam.gserviceaccount.com
func (m *ReconciliationGSAMiddleware) parseAndValidateReconciliationGSA(ctx context.Context, email string) (*ReconciliationScope, error) {
	// Parse email format: reconciliation-{scope}-{resource_id_short}@{gcp_project_id}.iam.gserviceaccount.com
	parts := strings.Split(email, "@")
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid GSA email format")
	}

	localPart := parts[0]
	domainPart := parts[1]

	// Extract GCP project ID from domain
	if !strings.HasSuffix(domainPart, ".iam.gserviceaccount.com") {
		return nil, fmt.Errorf("invalid GSA domain")
	}
	gcpProjectID := strings.TrimSuffix(domainPart, ".iam.gserviceaccount.com")

	// Parse local part: reconciliation-{scope}-{resource_id_short} or reconciliation-bootstrap
	if !strings.HasPrefix(localPart, "reconciliation-") {
		return nil, fmt.Errorf("GSA does not start with 'reconciliation-'")
	}

	remainder := strings.TrimPrefix(localPart, "reconciliation-")
	localParts := strings.Split(remainder, "-")

	if len(localParts) == 1 && localParts[0] == "bootstrap" {
		// Bootstrap reconciliation service
		return &ReconciliationScope{
			Scope:        "bootstrap",
			GCPProjectID: gcpProjectID,
		}, nil
	}

	if len(localParts) != 2 {
		return nil, fmt.Errorf("invalid reconciliation GSA format")
	}

	scopeType := localParts[0]       // "org" or "project"
	resourceIDShort := localParts[1] // First 8 chars of UUID

	switch scopeType {
	case "org":
		// Validate organization GSA
		return m.validateOrganizationGSA(ctx, resourceIDShort, gcpProjectID)
	case "project":
		// Validate project GSA
		return m.validateProjectGSA(ctx, resourceIDShort, gcpProjectID)
	default:
		return nil, fmt.Errorf("invalid scope type: %s", scopeType)
	}
}

// validateOrganizationGSA validates an organization-level reconciliation GSA.
func (m *ReconciliationGSAMiddleware) validateOrganizationGSA(ctx context.Context, resourceIDShort, gcpProjectID string) (*ReconciliationScope, error) {
	// Query organization by public_id prefix
	query := `SELECT id, BIN_TO_UUID(public_id) AS public_id, gcp_project_id
	          FROM organizations
	          WHERE BIN_TO_UUID(public_id) LIKE ?`

	var orgID int64
	var publicID, orgGCPProjectID string
	err := m.queries.(*db.Queries).GetDB().QueryRowContext(ctx, query, resourceIDShort+"%").Scan(&orgID, &publicID, &orgGCPProjectID)
	if err != nil {
		return nil, fmt.Errorf("organization not found for ID prefix %s: %w", resourceIDShort, err)
	}

	// Verify GCP project ID matches
	if orgGCPProjectID != gcpProjectID {
		return nil, fmt.Errorf("GCP project ID mismatch: expected %s, got %s", orgGCPProjectID, gcpProjectID)
	}

	// Verify public_id prefix matches
	if !strings.HasPrefix(publicID, resourceIDShort) {
		return nil, fmt.Errorf("organization public_id prefix mismatch")
	}

	return &ReconciliationScope{
		Scope:           "org",
		ResourceIDShort: resourceIDShort,
		GCPProjectID:    gcpProjectID,
		OrganizationID:  &orgID,
	}, nil
}

// validateProjectGSA validates a project-level reconciliation GSA.
func (m *ReconciliationGSAMiddleware) validateProjectGSA(ctx context.Context, resourceIDShort, gcpProjectID string) (*ReconciliationScope, error) {
	// Query project by public_id prefix
	query := `SELECT id, BIN_TO_UUID(public_id) AS public_id, gcp_project_id, organization_id
	          FROM projects
	          WHERE BIN_TO_UUID(public_id) LIKE ?`

	var projectID, organizationID int64
	var publicID, projGCPProjectID string
	err := m.queries.(*db.Queries).GetDB().QueryRowContext(ctx, query, resourceIDShort+"%").Scan(&projectID, &publicID, &projGCPProjectID, &organizationID)
	if err != nil {
		return nil, fmt.Errorf("project not found for ID prefix %s: %w", resourceIDShort, err)
	}

	// Verify GCP project ID matches
	if projGCPProjectID != gcpProjectID {
		return nil, fmt.Errorf("GCP project ID mismatch: expected %s, got %s", projGCPProjectID, gcpProjectID)
	}

	// Verify public_id prefix matches
	if !strings.HasPrefix(publicID, resourceIDShort) {
		return nil, fmt.Errorf("project public_id prefix mismatch")
	}

	return &ReconciliationScope{
		Scope:           "project",
		ResourceIDShort: resourceIDShort,
		GCPProjectID:    gcpProjectID,
		OrganizationID:  &organizationID,
		ProjectID:       &projectID,
	}, nil
}

// GetReconciliationScopeFromContext retrieves the reconciliation scope from the request context.
func GetReconciliationScopeFromContext(ctx context.Context) (*ReconciliationScope, bool) {
	scope, ok := ctx.Value("reconciliation_scope").(*ReconciliationScope)
	return scope, ok
}

// ValidateReconciliationAccess checks if the reconciliation scope has access to the requested resource.
// Bootstrap can access bootstrap operations only.
// Org scope can access org, project, and site operations within that org.
// Project scope can access project and site operations within that project.
func ValidateReconciliationAccess(scope *ReconciliationScope, resourceType string, orgID, projectID, siteID *int64) error {
	switch scope.Scope {
	case "bootstrap":
		// Bootstrap can only access bootstrap operations
		if resourceType != "bootstrap" {
			return fmt.Errorf("bootstrap scope can only access bootstrap operations")
		}
		return nil

	case "org":
		// Org scope can access anything within its organization
		if orgID != nil && scope.OrganizationID != nil && *orgID != *scope.OrganizationID {
			return fmt.Errorf("organization ID mismatch")
		}
		return nil

	case "project":
		// Project scope can only access its own project and sites
		if projectID != nil && scope.ProjectID != nil && *projectID != *scope.ProjectID {
			return fmt.Errorf("project ID mismatch")
		}
		return nil

	default:
		return fmt.Errorf("unknown reconciliation scope: %s", scope.Scope)
	}
}
