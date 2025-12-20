package events

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"

	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"

	"github.com/libops/api/db"
)

// Emitter writes events to the database queue for processing by the orchestrator.
type Emitter struct {
	querier db.Querier
	source  string // e.g., "io.libops.api"
}

// NewEmitter creates a new event emitter that writes to the database queue.
func NewEmitter(querier db.Querier, source string) *Emitter {
	return &Emitter{
		querier: querier,
		source:  source,
	}
}

// SendProtoEvent is the generic helper that accepts any generated Protobuf message
// and emits it as a CloudEvent.
//
// Events are written to the event_queue table for processing by the orchestrator.
//
// Parameters:
//   - ctx: Context for the operation
//   - eventType: The CloudEvent type (e.g., "io.libops.account.created.v1")
//   - data: Any protobuf message implementing proto.Message interface
//
// The event will have:
//   - ID: auto-generated UUID
//   - Source: the emitter's configured source
//   - Time: current timestamp
//   - Type: the provided eventType
//   - DataContentType: "application/protobuf"
//   - Data: the marshalled protobuf message
func (e *Emitter) SendProtoEvent(ctx context.Context, eventType string, data proto.Message) error {
	return e.SendProtoEventWithSubject(ctx, eventType, "", data)
}

// SendProtoEventWithSubject is like SendProtoEvent but also sets a subject field
// Subject typically identifies the resource the event is about (e.g., account ID)
//
// Events are written to the event_queue table for processing by the orchestrator.
func (e *Emitter) SendProtoEventWithSubject(ctx context.Context, eventType, subject string, data proto.Message) error {
	return e.SendScopedProtoEvent(ctx, eventType, subject, nil, nil, nil, data)
}

// SendScopedProtoEvent emits an event with optional organization, project, and site IDs.
// IDs can be provided as public UUID strings, which will be resolved to internal int64 IDs.
func (e *Emitter) SendScopedProtoEvent(ctx context.Context, eventType, subject string, orgID, projectID, siteID *string, data proto.Message) error {
	protoData, err := proto.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal proto data: %w", err)
	}

	eventID := uuid.NewString()

	var orgIDInt, projectIDInt, siteIDInt *int64

	// Resolve Organization ID
	if orgID != nil && *orgID != "" {
		org, err := e.querier.GetOrganization(ctx, *orgID)
		if err == nil {
			id := org.ID
			orgIDInt = &id
		} else {
			slog.Warn("failed to resolve organization ID for event", "org_id", *orgID, "error", err)
		}
	}

	// Resolve Project ID and auto-populate Organization ID
	if projectID != nil && *projectID != "" {
		proj, err := e.querier.GetProject(ctx, *projectID)
		if err == nil {
			id := proj.ID
			projectIDInt = &id
			// Auto-populate organization ID from project
			if orgIDInt == nil {
				orgID := proj.OrganizationID
				orgIDInt = &orgID
			}
		} else {
			slog.Warn("failed to resolve project ID for event", "project_id", *projectID, "error", err)
		}
	}

	// Resolve Site ID and auto-populate Project ID and Organization ID
	if siteID != nil && *siteID != "" {
		site, err := e.querier.GetSite(ctx, *siteID)
		if err == nil {
			id := site.ID
			siteIDInt = &id
			// Auto-populate project ID from site
			if projectIDInt == nil {
				projID := site.ProjectID
				projectIDInt = &projID
				// Also fetch the project to get org ID
				if orgIDInt == nil {
					if proj, err := e.querier.GetProjectByID(ctx, site.ProjectID); err == nil {
						orgID := proj.OrganizationID
						orgIDInt = &orgID
					}
				}
			}
		} else {
			slog.Warn("failed to resolve site ID for event", "site_id", *siteID, "error", err)
		}
	}

	// Write event to queue
	if err := e.enqueueEvent(ctx, eventID, eventType, subject, orgIDInt, projectIDInt, siteIDInt, protoData); err != nil {
		return fmt.Errorf("failed to enqueue event: %w", err)
	}

	slog.Info("Event queued",
		"event_id", eventID,
		"event_type", eventType)

	return nil
}

// enqueueEvent saves an event to the database queue.
func (e *Emitter) enqueueEvent(ctx context.Context, eventID, eventType, subject string, orgID, projectID, siteID *int64, data []byte) error {
	// Convert subject to sql.NullString
	var subjectSQL sql.NullString
	if subject != "" {
		subjectSQL = sql.NullString{String: subject, Valid: true}
	}

	return e.querier.EnqueueEvent(ctx, db.EnqueueEventParams{
		EventID:        eventID,
		EventType:      eventType,
		EventSource:    e.source,
		EventSubject:   subjectSQL,
		EventData:      data,
		ContentType:    "application/protobuf",
		OrganizationID: toNullInt64(orgID),
		ProjectID:      toNullInt64(projectID),
		SiteID:         toNullInt64(siteID),
	})
}

func toNullInt64(i *int64) sql.NullInt64 {
	if i == nil {
		return sql.NullInt64{}
	}
	return sql.NullInt64{Int64: *i, Valid: true}
}
