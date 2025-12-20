-- name: GetDeployment :one
SELECT id, site_id, `status`, github_run_id, github_run_url, started_at, completed_at, error_message, created_at
FROM deployments WHERE id = ?;

-- name: CreateDeployment :exec
INSERT INTO deployments (
  id, site_id, `status`, github_run_id, github_run_url, started_at, completed_at, error_message, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW());

-- name: UpdateDeployment :exec
UPDATE deployments SET
  `status` = ?,
  github_run_id = ?,
  github_run_url = ?,
  completed_at = ?,
  error_message = ?
WHERE id = ?;

-- name: DeleteDeployment :exec
DELETE FROM deployments WHERE id = ?;

-- name: ListSiteDeployments :many
SELECT * FROM deployments
WHERE site_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: GetLatestSiteDeployment :one
SELECT * FROM deployments
WHERE site_id = ?
ORDER BY created_at DESC
LIMIT 1;
