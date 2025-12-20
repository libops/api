-- name: CreateAuditEvent :exec
INSERT INTO audit (
  account_id, entity_id, entity_type, event_name, event_data
) VALUES (?, ?, ?, ?, ?);
