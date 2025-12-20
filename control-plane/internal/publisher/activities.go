package publisher

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/libops/control-plane/internal/database"
	"github.com/libops/control-plane/internal/workflows"
)

// DatabaseQuerier interface for finding affected resources
type DatabaseQuerier interface {
	GetSitesForOrg(ctx context.Context, orgID int64) ([]database.Site, error)
	GetSitesForProject(ctx context.Context, projectID int64) ([]database.Site, error)
	GetSite(ctx context.Context, siteID int64) (*database.Site, error)
	GetSitesForOrgMembers(ctx context.Context, orgID int64) ([]database.Site, error)
	GetSitesForProjectMembers(ctx context.Context, projectID int64) ([]database.Site, error)
	GetSitesForSiteMembers(ctx context.Context, siteID int64) ([]database.Site, error)
}

// Publisher interface for publishing reconciliation requests
type Publisher interface {
	PublishSiteReconciliation(ctx context.Context, req SiteReconciliationRequest) error
}

// ActivityHandler implements workflow activities for publishing reconciliation requests
type ActivityHandler struct {
	db        DatabaseQuerier
	publisher Publisher
}

// NewActivityHandler creates a new activity handler
func NewActivityHandler(db DatabaseQuerier, publisher Publisher) *ActivityHandler {
	return &ActivityHandler{
		db:        db,
		publisher: publisher,
	}
}

// PublishSiteReconciliation expands events to affected sites and publishes to Pub/Sub
func (h *ActivityHandler) PublishSiteReconciliation(ctx context.Context, input workflows.ReconciliationInput) (workflows.ReconciliationResult, error) {
	slog.Info("PublishSiteReconciliation activity started",
		"org_id", input.OrgID,
		"event_count", len(input.EventIDs),
		"scope", input.Scope)

	// Determine affected sites based on scope
	sites, err := h.getAffectedSites(ctx, input)
	if err != nil {
		slog.Error("Failed to get affected sites", "error", err)
		return workflows.ReconciliationResult{}, fmt.Errorf("failed to get affected sites: %w", err)
	}

	if len(sites) == 0 {
		slog.Warn("No sites found for reconciliation", "org_id", input.OrgID, "scope", input.Scope)
		return workflows.ReconciliationResult{
			Status:        "success",
			Message:       "No sites to reconcile",
			SitesAffected: 0,
		}, nil
	}

	slog.Info("Found affected sites",
		"org_id", input.OrgID,
		"site_count", len(sites),
		"scope", input.Scope,
		"type", input.ReconciliationType)

	// Use reconciliation type from workflow input
	requestType := string(input.ReconciliationType)

	// Publish reconciliation request for each site
	timestamp := time.Now().UTC().Format(time.RFC3339)
	for _, site := range sites {
		req := SiteReconciliationRequest{
			SitePublicID:    site.PublicID,
			ProjectPublicID: site.ProjectPublicID,
			OrgPublicID:     site.OrgPublicID,
			RequestType:     requestType,
			EventIDs:        input.EventIDs,
			Timestamp:       timestamp,
		}

		if err := h.publisher.PublishSiteReconciliation(ctx, req); err != nil {
			slog.Error("Failed to publish site reconciliation",
				"site_public_id", site.PublicID,
				"error", err)
			// Continue with other sites even if one fails
			continue
		}
	}

	slog.Info("Successfully published site reconciliation requests",
		"org_id", input.OrgID,
		"sites_affected", len(sites),
		"request_type", requestType)

	return workflows.ReconciliationResult{
		Status:        "success",
		Message:       fmt.Sprintf("Published reconciliation for %d sites", len(sites)),
		SitesAffected: len(sites),
	}, nil
}

func (h *ActivityHandler) getAffectedSites(ctx context.Context, input workflows.ReconciliationInput) ([]database.Site, error) {
	switch input.Scope {
	case workflows.ScopeOrg:
		return h.db.GetSitesForOrg(ctx, input.OrgID)
	case workflows.ScopeProject:
		if input.ProjectID == nil {
			return nil, fmt.Errorf("project scope requires project_id")
		}
		return h.db.GetSitesForProject(ctx, *input.ProjectID)
	case workflows.ScopeSite:
		if input.SiteID == nil {
			return nil, fmt.Errorf("site scope requires site_id")
		}
		site, err := h.db.GetSite(ctx, *input.SiteID)
		if err != nil {
			return nil, err
		}
		return []database.Site{*site}, nil
	default:
		return nil, fmt.Errorf("unknown scope: %v", input.Scope)
	}
}

// Removed: determineRequestType is now handled in workflows.DetermineReconciliationType
