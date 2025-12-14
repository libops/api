package events

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"

	"github.com/libops/api/internal/db"
)

// QueueProcessor processes queued events and sends them when the circuit is healthy.
type QueueProcessor struct {
	querier      db.Querier
	client       cloudevents.Client
	source       string
	instanceID   string
	batchSize    int32
	maxRetries   int32
	pollInterval time.Duration
	cleanupDays  int32
	staleTimeout int32 // Minutes before recovering stale processing events
	stopCh       chan struct{}
	stoppedCh    chan struct{}
}

// QueueProcessorConfig holds configuration for the queue processor.
type QueueProcessorConfig struct {
	BatchSize    int32
	MaxRetries   int32
	PollInterval time.Duration
	CleanupDays  int32
	StaleTimeout int32 // Minutes before recovering stale processing events
}

// DefaultQueueProcessorConfig returns default queue processor config.
func DefaultQueueProcessorConfig() QueueProcessorConfig {
	return QueueProcessorConfig{
		BatchSize:    10,
		MaxRetries:   5,
		PollInterval: 5 * time.Second,
		CleanupDays:  7,
		StaleTimeout: 5, // 5 minutes
	}
}

// NewQueueProcessor creates a new queue processor.
func NewQueueProcessor(querier db.Querier, client cloudevents.Client, source string, instanceID string, config QueueProcessorConfig) *QueueProcessor {
	return &QueueProcessor{
		querier:      querier,
		client:       client,
		source:       source,
		instanceID:   instanceID,
		batchSize:    config.BatchSize,
		maxRetries:   config.MaxRetries,
		pollInterval: config.PollInterval,
		cleanupDays:  config.CleanupDays,
		staleTimeout: config.StaleTimeout,
		stopCh:       make(chan struct{}),
		stoppedCh:    make(chan struct{}),
	}
}

// Start begins processing queued events.
func (p *QueueProcessor) Start(ctx context.Context) {
	slog.Info("Queue processor started", "instance", p.instanceID)
	ticker := time.NewTicker(p.pollInterval)
	defer ticker.Stop()
	defer close(p.stoppedCh)

	if err := p.querier.RecoverStaleProcessing(ctx, p.staleTimeout); err != nil {
		slog.Error("Error recovering stale events", "error", err)
	}

	for {
		select {
		case <-ctx.Done():
			slog.Info("Queue processor stopped (context cancelled)")
			return
		case <-p.stopCh:
			slog.Info("Queue processor stopped (stop signal)")
			return
		case <-ticker.C:
			// Periodically recover stale events
			if err := p.querier.RecoverStaleProcessing(ctx, p.staleTimeout); err != nil {
				slog.Error("Error recovering stale events", "error", err)
				return
			}

			if err := p.processBatch(ctx); err != nil {
				slog.Error("Error processing event queue batch", "error", err)
			}
		}
	}
}

// Stop signals the processor to stop.
func (p *QueueProcessor) Stop() {
	close(p.stopCh)
	<-p.stoppedCh // Wait for processor to finish
}

// processBatch processes a batch of queued events.
func (p *QueueProcessor) processBatch(ctx context.Context) error {
	result, err := p.querier.ClaimPendingEvents(ctx, db.ClaimPendingEventsParams{
		ProcessingBy: sql.NullString{String: p.instanceID, Valid: true},
		RetryCount:   p.maxRetries,
		Limit:        p.batchSize,
	})
	if err != nil {
		return fmt.Errorf("failed to claim pending events: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return nil
	}

	events, err := p.querier.GetClaimedEvents(ctx, sql.NullString{String: p.instanceID, Valid: true})
	if err != nil {
		return fmt.Errorf("failed to get claimed events: %w", err)
	}

	slog.Info("Processing queued events", "count", len(events), "instance", p.instanceID)

	successCount := 0
	failCount := 0
	deadLetterCount := 0

	for _, queuedEvent := range events {
		event := cloudevents.NewEvent()
		event.SetID(queuedEvent.EventID)
		event.SetSource(queuedEvent.EventSource)
		event.SetType(queuedEvent.EventType)
		if queuedEvent.EventSubject.Valid {
			event.SetSubject(queuedEvent.EventSubject.String)
		}
		event.SetDataContentType(queuedEvent.ContentType)
		event.SetTime(time.Now())

		if err := event.SetData(cloudevents.ApplicationCloudEventsJSON, queuedEvent.EventData); err != nil {
			slog.Error("Failed to set event data", "event_id", queuedEvent.EventID, "error", err)
			continue
		}

		result := p.client.Send(ctx, event)
		if cloudevents.IsNACK(result) {
			if queuedEvent.RetryCount >= int32(p.maxRetries-1) {
				if err := p.querier.MarkEventDeadLetter(ctx, db.MarkEventDeadLetterParams{
					ID:        queuedEvent.ID,
					LastError: sql.NullString{String: fmt.Sprintf("%v", result), Valid: true},
				}); err != nil {
					slog.Error("Failed to mark event as dead letter", "event_id", queuedEvent.EventID, "error", err)
				} else {
					deadLetterCount++
					slog.Warn("Event moved to dead letter", "event_id", queuedEvent.EventID, "retry_count", queuedEvent.RetryCount+1)
				}
			} else {
				if err := p.querier.MarkEventFailed(ctx, db.MarkEventFailedParams{
					ID:        queuedEvent.ID,
					LastError: sql.NullString{String: fmt.Sprintf("%v", result), Valid: true},
				}); err != nil {
					slog.Error("Failed to mark event as failed", "event_id", queuedEvent.EventID, "error", err)
				} else {
					failCount++
				}
			}
		} else {
			if err := p.querier.MarkEventSent(ctx, queuedEvent.ID); err != nil {
				slog.Error("Failed to mark event as sent", "event_id", queuedEvent.EventID, "error", err)
			} else {
				successCount++
			}
		}

		// Slow down to avoid overwhelming Pub/Sub
		time.Sleep(100 * time.Millisecond)
	}

	if successCount > 0 || failCount > 0 || deadLetterCount > 0 {
		slog.Info("Queue batch processed",
			"sent", successCount,
			"failed", failCount,
			"dead_letter", deadLetterCount)
	}

	return nil
}

// CleanupOldEvents removes old sent events from the queue.
func (p *QueueProcessor) CleanupOldEvents(ctx context.Context) error {
	return p.querier.CleanupOldEvents(ctx, p.cleanupDays)
}

// GetStats returns queue statistics.
func (p *QueueProcessor) GetStats(ctx context.Context) (db.GetQueueStatsRow, error) {
	return p.querier.GetQueueStats(ctx)
}
