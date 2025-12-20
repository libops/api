-- Reconciliation run queries (supports both terraform and VM reconciliation)

-- name: CreateReconciliationRun :execresult
INSERT INTO reconciliations (
    run_id,
    organization_id,
    project_id,
    site_id,
    run_type,
    reconciliation_type,
    modules,
    target_site_ids,
    event_ids,
    first_event_at,
    last_event_at,
    status
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'pending');

-- name: GetReconciliationRunByID :one
SELECT * FROM reconciliations
WHERE run_id = ?
LIMIT 1;

-- name: GetPendingReconciliationRunByResource :one
SELECT * FROM reconciliations
WHERE organization_id = COALESCE(?, organization_id)
  AND project_id = COALESCE(?, project_id)
  AND site_id = COALESCE(?, site_id)
  AND status IN ('pending', 'running')
LIMIT 1;

-- name: GetPendingReconciliationRunByOrg :one
SELECT * FROM reconciliations
WHERE organization_id = ? AND status IN ('pending', 'running')
LIMIT 1;

-- name: GetPendingReconciliationRunByProject :one
SELECT * FROM reconciliations
WHERE project_id = ? AND status IN ('pending', 'running')
LIMIT 1;

-- name: GetPendingReconciliationRunBySite :one
SELECT * FROM reconciliations
WHERE site_id = ? AND status IN ('pending', 'running')
LIMIT 1;

-- name: GetStaleReconciliationRuns :many
SELECT * FROM reconciliations
WHERE status = 'running'
  AND started_at < NOW() - INTERVAL 30 MINUTE;

-- name: UpdateReconciliationRunStatus :exec
UPDATE reconciliations
SET status = CAST(sqlc.arg(status) AS CHAR),
    triggered_at = CASE WHEN sqlc.arg(status) = 'triggered' THEN CURRENT_TIMESTAMP ELSE triggered_at END,
    started_at = CASE WHEN sqlc.arg(status) = 'running' THEN CURRENT_TIMESTAMP ELSE started_at END,
    completed_at = CASE WHEN sqlc.arg(status) IN ('completed', 'failed') THEN CURRENT_TIMESTAMP ELSE completed_at END,
    error_message = sqlc.arg(error_message)
WHERE run_id = sqlc.arg(run_id);

-- name: UpdateReconciliationRunTriggered :exec
UPDATE reconciliations
SET status = 'triggered',
    triggered_at = CURRENT_TIMESTAMP
WHERE run_id = ?;

-- name: UpdateReconciliationRunStarted :exec
UPDATE reconciliations
SET status = 'running',
    started_at = CURRENT_TIMESTAMP
WHERE run_id = ?;

-- name: UpdateReconciliationRunCompleted :exec
UPDATE reconciliations
SET status = 'completed',
    completed_at = CURRENT_TIMESTAMP
WHERE run_id = ?;

-- name: UpdateReconciliationRunFailed :exec
UPDATE reconciliations
SET status = 'failed',
    completed_at = CURRENT_TIMESTAMP,
    error_message = ?
WHERE run_id = ?;

-- name: AppendEventIDsToRun :exec
UPDATE reconciliations
SET event_ids = JSON_ARRAY_APPEND(event_ids, '$', ?),
    last_event_at = ?
WHERE run_id = ?;

-- name: UpgradeReconciliationRunScope :exec
UPDATE reconciliations
SET organization_id = COALESCE(?, organization_id),
    project_id = COALESCE(?, project_id),
    site_id = COALESCE(?, site_id),
    modules = COALESCE(?, modules),
    target_site_ids = COALESCE(?, target_site_ids),
    event_ids = JSON_ARRAY_APPEND(event_ids, '$', ?),
    last_event_at = ?
WHERE run_id = ?;

-- Reconciliation result queries

-- name: CreateReconciliationResult :execresult
INSERT INTO reconciliation_results (
    run_id,
    result_type,
    module_type,
    site_id,
    resource_id,
    status,
    output,
    error_message,
    started_at,
    completed_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetReconciliationResults :many
SELECT * FROM reconciliation_results
WHERE run_id = ?
ORDER BY started_at ASC;

-- name: GetReconciliationResultsBySite :many
SELECT * FROM reconciliation_results
WHERE run_id = ? AND site_id = ?
ORDER BY started_at ASC;

-- name: GetSiteIDsByOrganization :many
SELECT s.id FROM sites s
JOIN projects p ON s.project_id = p.id
WHERE p.organization_id = ?
  AND s.gcp_external_ip IS NOT NULL
  AND s.checkin_at > NOW() - INTERVAL 15 MINUTE;

-- name: GetSiteIDsByProject :many
SELECT id FROM sites
WHERE project_id = ?
  AND gcp_external_ip IS NOT NULL
  AND checkin_at > NOW() - INTERVAL 15 MINUTE;

-- name: GetSiteIDsBySite :many
SELECT id FROM sites
WHERE id = ?
  AND gcp_external_ip IS NOT NULL
  AND checkin_at > NOW() - INTERVAL 15 MINUTE;

-- name: GetRunningReconciliations :many
SELECT run_id, organization_id, project_id, site_id, run_type
FROM reconciliations
WHERE status IN ('pending', 'running')
ORDER BY
  CASE
    WHEN organization_id IS NOT NULL AND project_id IS NULL AND site_id IS NULL THEN 1
    WHEN project_id IS NOT NULL AND site_id IS NULL THEN 2
    WHEN site_id IS NOT NULL THEN 3
  END ASC
LIMIT 1;

-- name: ClearStaleLocks :execresult
UPDATE reconciliations
SET status = 'failed',
    error_message = 'Lock expired after 30 minutes',
    completed_at = CURRENT_TIMESTAMP
WHERE status = 'running'
  AND started_at < NOW() - INTERVAL 30 MINUTE;
