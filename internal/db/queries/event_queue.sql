-- name: EnqueueEvent :exec
INSERT INTO event_queue (
    event_id,
    event_type,
    event_source,
    event_subject,
    event_data,
    content_type,
    created_at
) VALUES (?, ?, ?, ?, ?, ?, NOW());

-- name: GetPendingEvents :many
SELECT
    id,
    event_id,
    event_type,
    event_source,
    event_subject,
    event_data,
    content_type,
    retry_count,
    created_at,
    last_retry_at
FROM event_queue
WHERE status = 'pending'
  AND retry_count < ?
ORDER BY created_at ASC
LIMIT ?;

-- name: MarkEventSent :exec
UPDATE event_queue
SET status = 'sent',
    sent_at = NOW()
WHERE id = ?;

-- name: MarkEventFailed :exec
UPDATE event_queue
SET retry_count = retry_count + 1,
    last_retry_at = NOW(),
    last_error = ?
WHERE id = ?;

-- name: MarkEventDeadLetter :exec
UPDATE event_queue
SET status = 'dead_letter',
    last_error = ?
WHERE id = ?;

-- name: GetQueueStats :one
SELECT
    COUNT(*) as total_events,
    SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending_events,
    SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END) as sent_events,
    SUM(CASE WHEN status = 'dead_letter' THEN 1 ELSE 0 END) as dead_letter_events
FROM event_queue;

-- name: CleanupOldEvents :exec
DELETE FROM event_queue
WHERE status = 'sent'
  AND sent_at < DATE_SUB(NOW(), INTERVAL ? DAY);
