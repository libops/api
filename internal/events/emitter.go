package events

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/google/uuid"
	"github.com/sony/gobreaker/v2"
	"google.golang.org/protobuf/proto"

	"github.com/libops/api/internal/db"
)

// Emitter encapsulates the CloudEvents client and a constant source.
type Emitter struct {
	client  cloudevents.Client
	source  string // e.g., "io.libops.api"
	cb      *gobreaker.CircuitBreaker[cloudevents.Result]
	querier db.Querier // For fallback queue
}

// EmitterConfig holds configuration for the emitter.
type EmitterConfig struct {
	MaxRequests uint32
	Interval    time.Duration
	Timeout     time.Duration
	ReadyToTrip func(counts gobreaker.Counts) bool
}

// DefaultEmitterConfig returns default circuit breaker config.
func DefaultEmitterConfig() EmitterConfig {
	return EmitterConfig{
		MaxRequests: 3,
		Interval:    10 * time.Second,
		Timeout:     30 * time.Second,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			// Trip after 5 consecutive failures
			return counts.ConsecutiveFailures > 5
		},
	}
}

// NewEmitter creates a new event emitter with circuit breaker and database queue fallback.
func NewEmitter(client cloudevents.Client, source string, querier db.Querier) *Emitter {
	return NewEmitterWithConfig(client, source, querier, DefaultEmitterConfig())
}

// NewEmitterWithConfig creates a new event emitter with custom circuit breaker config.
func NewEmitterWithConfig(client cloudevents.Client, source string, querier db.Querier, config EmitterConfig) *Emitter {
	settings := gobreaker.Settings{
		Name:        "EventEmitter",
		MaxRequests: config.MaxRequests,
		Interval:    config.Interval,
		Timeout:     config.Timeout,
		ReadyToTrip: config.ReadyToTrip,
		OnStateChange: func(name string, from gobreaker.State, to gobreaker.State) {
			slog.Info("Circuit breaker state changed",
				"name", name,
				"from", from.String(),
				"to", to.String())
		},
	}

	return &Emitter{
		client:  client,
		source:  source,
		cb:      gobreaker.NewCircuitBreaker[cloudevents.Result](settings),
		querier: querier,
	}
}

// SendProtoEvent is the generic helper that accepts any generated Protobuf message
// and emits it as a CloudEvent.
//
// If the circuit breaker is open, the event is queued to the database.
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
// This method uses a circuit breaker. If the circuit is open (too many failures),
// the event is queued to the database for later retry.
func (e *Emitter) SendProtoEventWithSubject(ctx context.Context, eventType, subject string, data proto.Message) error {
	protoData, err := proto.Marshal(data)
	if err != nil {
		return fmt.Errorf("failed to marshal proto data: %w", err)
	}

	event := cloudevents.NewEvent()
	eventID := uuid.NewString()
	event.SetID(eventID)
	event.SetSource(e.source)
	event.SetTime(time.Now())
	event.SetType(eventType)
	if subject != "" {
		event.SetSubject(subject)
	}
	event.SetDataContentType("application/protobuf")

	if err := event.SetData(cloudevents.ApplicationCloudEventsJSON, protoData); err != nil {
		return fmt.Errorf("failed to set cloudevent data: %w", err)
	}

	_, err = e.cb.Execute(func() (cloudevents.Result, error) {
		result := e.client.Send(ctx, event)
		if cloudevents.IsNACK(result) {
			return result, fmt.Errorf("event NACK: %w", result)
		}
		return result, nil
	})

	if err != nil {
		if e.querier != nil {
			queueErr := e.enqueueEvent(ctx, eventID, eventType, subject, protoData)
			if queueErr != nil {
				return fmt.Errorf("failed to send event and failed to queue: send_err=%w, queue_err=%v", err, queueErr)
			}
			// Event queued successfully, return nil (no error to caller)
			slog.Info("Event queued to database (circuit open or send failed)",
				"event_type", eventType)
			return nil
		}
		return fmt.Errorf("failed to send event: %w", err)
	}

	return nil
}

// enqueueEvent saves an event to the database queue.
func (e *Emitter) enqueueEvent(ctx context.Context, eventID, eventType, subject string, data []byte) error {
	// Convert subject to sql.NullString
	var subjectSQL sql.NullString
	if subject != "" {
		subjectSQL = sql.NullString{String: subject, Valid: true}
	}

	return e.querier.EnqueueEvent(ctx, db.EnqueueEventParams{
		EventID:      eventID,
		EventType:    eventType,
		EventSource:  e.source,
		EventSubject: subjectSQL,
		EventData:    data,
		ContentType:  "application/protobuf",
	})
}
