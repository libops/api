package eventrouter

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/libops/control-plane/internal/workflows"
)

// EventPoller polls the event_queue table and dispatches events to the reconciliation manager
type EventPoller struct {
	db      *sql.DB
	manager *ReconciliationManager
	config  *Config
}

// NewEventPoller creates a new event poller
func NewEventPoller(db *sql.DB, manager *ReconciliationManager, config *Config) *EventPoller {
	return &EventPoller{
		db:      db,
		manager: manager,
		config:  config,
	}
}

// Start begins polling for events
func (p *EventPoller) Start(ctx context.Context) {
	ticker := time.NewTicker(time.Duration(p.config.PollIntervalSeconds) * time.Second)
	defer ticker.Stop()

	slog.Info("Event poller started", "interval_seconds", p.config.PollIntervalSeconds)

	for {
		select {
		case <-ctx.Done():
			slog.Info("Event poller stopped")
			return
		case <-ticker.C:
			if err := p.pollAndDispatch(ctx); err != nil {
				slog.Error("Failed to poll and dispatch events", "error", err)
			}
		}
	}
}

func (p *EventPoller) pollAndDispatch(ctx context.Context) error {
	// Query for pending events, grouped by organization
	query := `
		SELECT
			event_id,
			event_type,
			event_source,
			event_subject,
			event_data,
			content_type,
			organization_id,
			project_id,
			site_id,
			created_at
		FROM event_queue
		WHERE status = 'pending'
		ORDER BY organization_id, created_at
		LIMIT ?
	`

	rows, err := p.db.QueryContext(ctx, query, p.config.MaxConcurrentEvents)
	if err != nil {
		return fmt.Errorf("failed to query events: %w", err)
	}
	defer rows.Close()

	var events []workflows.Event
	
	for rows.Next() {
		var event workflows.Event
		var eventSubject sql.NullString
		var projectID, siteID sql.NullInt64

		err := rows.Scan(
			&event.EventID,
			&event.EventType,
			&event.EventSource,
			&eventSubject,
			&event.EventData,
			&event.ContentType,
			&event.OrganizationID,
			&projectID,
			&siteID,
			&event.CreatedAt,
		)
		if err != nil {
			slog.Error("Failed to scan event", "error", err)
			continue
		}

		if eventSubject.Valid {
			event.EventSubject = eventSubject.String
		}
		if projectID.Valid {
			pID := projectID.Int64
			event.ProjectID = &pID
		}
		if siteID.Valid {
			sID := siteID.Int64
			event.SiteID = &sID
		}

		events = append(events, event)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("error iterating events: %w", err)
	}

	if len(events) == 0 {
		return nil
	}

	slog.Info("Polled events", "count", len(events))

	// Dispatch events to manager
	for _, event := range events {
		p.manager.AcceptEvent(ctx, event)

		// Mark events as sent
		if err := p.markEventSent(ctx, event.EventID); err != nil {
			slog.Error("Failed to mark event as sent",
				"event_id", event.EventID,
				"error", err)
		}
	}

	return nil
}

func (p *EventPoller) markEventSent(ctx context.Context, eventID string) error {
	query := `UPDATE event_queue SET status = 'sent', sent_at = NOW() WHERE event_id = ?`
	_, err := p.db.ExecContext(ctx, query, eventID)
	if err != nil {
		return fmt.Errorf("failed to update event status: %w", err)
	}
	return nil
}

