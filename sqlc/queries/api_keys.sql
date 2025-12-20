-- name: CreateAPIKey :exec
INSERT INTO api_keys (
  public_id, account_id, `name`, description, scopes, created_at, expires_at, active, created_by
) VALUES (UUID_TO_BIN(sqlc.arg(public_id)), ?, ?, ?, ?, NOW(), ?, ?, ?);


-- name: GetAPIKeyByUUID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, account_id, `name`, description,
       COALESCE(scopes, '[]') as scopes,
       created_at, last_used_at, expires_at, active, created_by
FROM api_keys WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: GetAPIKeyByID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, account_id, `name`, description,
       COALESCE(scopes, '[]') as scopes,
       created_at, last_used_at, expires_at, active, created_by
FROM api_keys WHERE id = ?;


-- name: UpdateAPIKeyLastUsed :exec
UPDATE api_keys SET
  last_used_at = NOW()
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: UpdateAPIKeyActive :exec
UPDATE api_keys SET
  active = ?
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: DeleteAPIKey :exec
DELETE FROM api_keys WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: GetActiveAPIKeyByUUID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, account_id, `name`, description,
       COALESCE(scopes, '[]') as scopes,
       created_at, last_used_at, expires_at, active, created_by
FROM api_keys
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id))
  AND active = TRUE
  AND (expires_at IS NULL OR expires_at > NOW());

-- =============================================================================
-- ORGANIZATION MEMBERS
-- =============================================================================


