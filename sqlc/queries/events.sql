-- name: GetQueueStats :one
SELECT
    COUNT(*) as total_events,
    SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending_events,
    SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END) as sent_events,
    SUM(CASE WHEN status = 'dead_letter' THEN 1 ELSE 0 END) as dead_letter_events
FROM event_queue;

-- EVENT QUEUE

-- name: EnqueueEvent :exec
INSERT INTO event_queue (
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
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NOW());

-- name: GetPendingEvents :many
SELECT id, event_id, event_type, event_source, event_subject, event_data, content_type,
        organization_id, project_id, site_id, created_at
FROM event_queue
WHERE status = 'pending'
ORDER BY created_at ASC
LIMIT ?;

-- name: MarkEventSent :exec
UPDATE event_queue
SET status = 'sent',
    sent_at = NOW()
WHERE id = ?;

-- name: MarkEventSentOrStatus :exec
UPDATE event_queue
SET status = CASE
    WHEN status IN ('executed', 'collapsed') THEN status
    ELSE 'sent'
END,
sent_at = CASE
    WHEN status IN ('executed', 'collapsed') THEN sent_at
    ELSE NOW()
END
WHERE event_id = ? AND status = 'pending';

-- name: MarkEventExecuted :exec
UPDATE event_queue
SET status = 'executed',
    created_run_id = ?,
    processed_at = NOW()
WHERE event_id = ?;

-- name: MarkEventCollapsed :exec
UPDATE event_queue
SET status = 'collapsed',
    collapsed_into_run_id = ?,
    processed_at = NOW()
WHERE event_id = ?;

-- name: MarkEventDeadLetter :exec
UPDATE event_queue
SET status = 'dead_letter',
    processed_at = NOW()
WHERE event_id = ?;
