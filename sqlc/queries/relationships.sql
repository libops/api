-- name: GetRelationship :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, source_organization_id, target_organization_id,
       relationship_type, `status`, created_at, resolved_at, resolved_by
FROM relationships WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: CreateRelationship :execresult
INSERT INTO relationships (
  public_id, source_organization_id, target_organization_id, relationship_type, `status`, created_at
) VALUES (
  UUID_TO_BIN(UUID_V7()), ?, ?, ?, 'pending', CURRENT_TIMESTAMP
);


-- name: ApproveRelationship :execresult
UPDATE relationships SET
  `status` = 'approved',
  resolved_at = CURRENT_TIMESTAMP,
  resolved_by = ?
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id)) AND `status` = 'pending';


-- name: RejectRelationship :execresult
UPDATE relationships SET
  `status` = 'rejected',
  resolved_at = CURRENT_TIMESTAMP,
  resolved_by = ?
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id)) AND `status` = 'pending';


