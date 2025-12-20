package eventrouter

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/libops/control-plane/internal/publisher"
	"github.com/libops/control-plane/internal/workflows"
)

// ReconciliationManager manages the reconciliation process for organizations
type ReconciliationManager struct {
	activityHandler *publisher.ActivityHandler
	orgStates       map[int64]*OrgState
	mu              sync.Mutex
}

// OrgState tracks the state of reconciliation for an organization
type OrgState struct {
	PendingEvents    []workflows.Event
	DebounceTimer    *time.Timer
	CurrentScope     workflows.EventScope
	CurrentProjectID *int64
	CurrentSiteID    *int64
}

// NewReconciliationManager creates a new reconciliation manager
func NewReconciliationManager(activityHandler *publisher.ActivityHandler) *ReconciliationManager {
	return &ReconciliationManager{
		activityHandler: activityHandler,
		orgStates:       make(map[int64]*OrgState),
	}
}

// AcceptEvent adds an event to the organization's processing queue
func (rm *ReconciliationManager) AcceptEvent(ctx context.Context, event workflows.Event) {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	orgID := event.OrganizationID
	state, exists := rm.orgStates[orgID]
	if !exists {
		state = &OrgState{
			CurrentScope: workflows.ScopeUnknown,
		}
		rm.orgStates[orgID] = state
	}

	// Update scope
	eventScope := workflows.DetermineScope(event)
	if eventScope > state.CurrentScope {
		state.CurrentScope = eventScope
		switch eventScope {
		case workflows.ScopeProject:
			state.CurrentProjectID = event.ProjectID
			state.CurrentSiteID = nil
		case workflows.ScopeOrg:
			state.CurrentProjectID = nil
			state.CurrentSiteID = nil
		}
	} else if eventScope == workflows.ScopeSite && state.CurrentScope == workflows.ScopeSite {
		state.CurrentSiteID = event.SiteID
	}

	state.PendingEvents = append(state.PendingEvents, event)

	// Determine debounce time
	debounceTime := 5 * time.Second
	if eventScope == workflows.ScopeOrg {
		debounceTime = 2 * time.Second
	}

	// Reset or start timer
	if state.DebounceTimer != nil {
		state.DebounceTimer.Stop()
	}

	state.DebounceTimer = time.AfterFunc(debounceTime, func() {
		rm.processOrgEvents(context.Background(), orgID)
	})

	slog.Info("Event accepted and scheduled for processing",
		"org_id", orgID,
		"event_id", event.EventID,
		"debounce_seconds", debounceTime.Seconds())
}

func (rm *ReconciliationManager) processOrgEvents(ctx context.Context, orgID int64) {
	rm.mu.Lock()
	state, exists := rm.orgStates[orgID]
	if !exists {
		rm.mu.Unlock()
		return
	}

	// Copy state for processing
	events := state.PendingEvents
	scope := state.CurrentScope
	projectID := state.CurrentProjectID
	siteID := state.CurrentSiteID

	// Clear state
	delete(rm.orgStates, orgID)
	rm.mu.Unlock()

	if len(events) == 0 {
		return
	}

	slog.Info("Processing accumulated events",
		"org_id", orgID,
		"event_count", len(events),
		"scope", scope)

	// Collect event types and IDs
	eventIDs := make([]string, len(events))
	eventTypes := make([]string, len(events))
	for i, e := range events {
		eventIDs[i] = e.EventID
		eventTypes[i] = e.EventType
	}

	// Determine reconciliation type
	reconciliationType := workflows.DetermineReconciliationType(eventTypes)

	input := workflows.ReconciliationInput{
		OrgID:              orgID,
		ProjectID:          projectID,
		SiteID:             siteID,
		EventIDs:           eventIDs,
		EventTypes:         eventTypes,
		Scope:              scope,
		ReconciliationType: reconciliationType,
	}

	// Execute activity directly
	_, err := rm.activityHandler.PublishSiteReconciliation(ctx, input)
	if err != nil {
		slog.Error("Reconciliation failed", "org_id", orgID, "error", err)
	} else {
		slog.Info("Reconciliation completed", "org_id", orgID)
	}
}
