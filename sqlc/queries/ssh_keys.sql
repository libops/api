-- name: GetSshKey :one
SELECT sk.id, BIN_TO_UUID(sk.public_id) AS public_id,
       BIN_TO_UUID(a.public_id) AS account_public_id,
       sk.public_key, sk.`name`, sk.fingerprint,
       sk.created_at, sk.updated_at
FROM ssh_keys sk
JOIN accounts a ON sk.account_id = a.id
WHERE sk.public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: CreateSshKey :execresult
INSERT INTO ssh_keys (
  public_id, account_id, public_key, `name`, fingerprint, created_at, updated_at
) VALUES (
  UUID_TO_BIN(sqlc.arg(public_id)),
  (SELECT id FROM accounts WHERE accounts.public_id = UUID_TO_BIN(sqlc.arg(account_public_id))),
  ?, ?, ?,
  CURRENT_TIMESTAMP, CURRENT_TIMESTAMP
);


-- name: UpdateSshKey :execresult
UPDATE ssh_keys SET
  `name` = ?,
  updated_at = CURRENT_TIMESTAMP
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: DeleteSshKey :exec
DELETE FROM ssh_keys WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: GetSshAccess :one
SELECT id, account_id, site_id, created_at, updated_at, created_by, updated_by
FROM ssh_access WHERE account_id = ? AND site_id = ?;


-- name: CreateSshAccess :exec
INSERT INTO ssh_access (
  account_id, site_id, created_at, updated_at, created_by, updated_by
) VALUES (?, ?, NOW(), NOW(), ?, ?);


-- name: DeleteSshAccess :exec
DELETE FROM ssh_access WHERE account_id = ? AND site_id = ?;
