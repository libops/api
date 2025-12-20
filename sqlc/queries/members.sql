-- name: GetProjectMember :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, account_id, `role`, created_at, updated_at, created_by, updated_by
FROM project_members WHERE project_id = ? AND account_id = ?;


-- name: CreateProjectMember :exec
INSERT INTO project_members (
  public_id, project_id, account_id, `role`, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, NOW(), NOW(), ?, ?);


-- name: UpdateProjectMember :exec
UPDATE project_members SET
  `role` = ?,
  updated_at = NOW(),
  updated_by = ?
WHERE project_id = ? AND account_id = ?;


-- name: DeleteProjectMember :exec
DELETE FROM project_members WHERE project_id = ? AND account_id = ?;


-- name: ListProjectMembers :many
SELECT pm.id, BIN_TO_UUID(pm.public_id) AS public_id, pm.project_id, pm.account_id, pm.`role`, pm.status, pm.created_at, pm.updated_at,
       BIN_TO_UUID(a.public_id) AS account_public_id, a.email, a.`name`, a.github_username
FROM project_members pm
JOIN accounts a ON pm.account_id = a.id
WHERE pm.project_id = ?
ORDER BY pm.created_at DESC
LIMIT ? OFFSET ?;


-- name: GetSiteMember :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, site_id, account_id, `role`, created_at, updated_at, created_by, updated_by
FROM site_members WHERE site_id = ? AND account_id = ?;


-- name: CreateSiteMember :exec
INSERT INTO site_members (
  public_id, site_id, account_id, `role`, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, NOW(), NOW(), ?, ?);


-- name: UpdateSiteMember :exec
UPDATE site_members SET
  `role` = ?,
  updated_at = NOW(),
  updated_by = ?
WHERE site_id = ? AND account_id = ?;


-- name: DeleteSiteMember :exec
DELETE FROM site_members WHERE site_id = ? AND account_id = ?;


-- name: ListSiteMembers :many
SELECT sm.id, BIN_TO_UUID(sm.public_id) AS public_id, sm.site_id, sm.account_id, sm.`role`, sm.status, sm.created_at, sm.updated_at,
       BIN_TO_UUID(a.public_id) AS account_public_id, a.email, a.`name`, a.github_username
FROM site_members sm
JOIN accounts a ON sm.account_id = a.id
WHERE sm.site_id = ?
ORDER BY sm.created_at DESC
LIMIT ? OFFSET ?;


-- name: ListUserMemberships :many
WITH RECURSIVE user_orgs AS (
    SELECT organization_id FROM organization_members WHERE organization_members.account_id = sqlc.arg(account_id) AND organization_members.status = 'active'
    UNION DISTINCT
    SELECT r.target_organization_id
    FROM relationships r
    INNER JOIN user_orgs uo ON r.source_organization_id = uo.organization_id
    WHERE r.status = 'approved'
)
SELECT * FROM (
    SELECT
        om.id, BIN_TO_UUID(om.public_id) AS public_id, om.status, om.created_at, om.updated_at, om.role,
        a.email, a.name AS user_name, BIN_TO_UUID(a.public_id) AS account_public_id,
        'organization' AS parent_type,
        o.name AS parent_name,
        BIN_TO_UUID(o.public_id) AS parent_public_id
    FROM organization_members om
    JOIN organizations o ON om.organization_id = o.id
    JOIN accounts a ON om.account_id = a.id
    JOIN user_orgs uo ON o.id = uo.organization_id
    WHERE om.status != 'deleted'

    UNION ALL

    SELECT
        pm.id, BIN_TO_UUID(pm.public_id) AS public_id, pm.status, pm.created_at, pm.updated_at, pm.role,
        a.email, a.name AS user_name, BIN_TO_UUID(a.public_id) AS account_public_id,
        'project' AS parent_type,
        p.name AS parent_name,
        BIN_TO_UUID(p.public_id) AS parent_public_id
    FROM project_members pm
    JOIN projects p ON pm.project_id = p.id
    JOIN accounts a ON pm.account_id = a.id
    LEFT JOIN project_members pm_auth ON p.id = pm_auth.project_id AND pm_auth.account_id = sqlc.arg(account_id) AND pm_auth.status = 'active'
    LEFT JOIN user_orgs uo ON p.organization_id = uo.organization_id
    WHERE pm.status != 'deleted'
    AND (pm_auth.id IS NOT NULL OR uo.organization_id IS NOT NULL)

    UNION ALL

    SELECT
        sm.id, BIN_TO_UUID(sm.public_id) AS public_id, sm.status, sm.created_at, sm.updated_at, sm.role,
        a.email, a.name AS user_name, BIN_TO_UUID(a.public_id) AS account_public_id,
        'site' AS parent_type,
        s.name AS parent_name,
        BIN_TO_UUID(s.public_id) AS parent_public_id
    FROM site_members sm
    JOIN sites s ON sm.site_id = s.id
    JOIN projects p ON s.project_id = p.id
    JOIN accounts a ON sm.account_id = a.id
    LEFT JOIN site_members sm_auth ON s.id = sm_auth.site_id AND sm_auth.account_id = sqlc.arg(account_id) AND sm_auth.status = 'active'
    LEFT JOIN project_members pm_auth ON p.id = pm_auth.project_id AND pm_auth.account_id = sqlc.arg(account_id) AND pm_auth.status = 'active'
    LEFT JOIN user_orgs uo ON p.organization_id = uo.organization_id
    WHERE sm.status != 'deleted'
    AND (sm_auth.id IS NOT NULL OR pm_auth.id IS NOT NULL OR uo.organization_id IS NOT NULL)
) AS all_members
ORDER BY created_at DESC
LIMIT ? OFFSET ?;


-- name: UpdateProjectMemberStatus :exec
-- Updates project member status (e.g., provisioning → active)
UPDATE project_members
SET status = ?, updated_at = NOW()
WHERE public_id = UUID_TO_BIN(?);


-- name: UpdateSiteMemberStatus :exec
-- Updates site member status (e.g., provisioning → active)
UPDATE site_members
SET status = ?, updated_at = NOW()
WHERE public_id = UUID_TO_BIN(?);

-- name: GetOrganizationsByAccountID :many
SELECT DISTINCT o.id
FROM organizations o
WHERE o.id IN (
    SELECT om.organization_id FROM organization_members om WHERE om.account_id = sqlc.arg(account_id) AND om.status = 'active'
    UNION
    SELECT p.organization_id FROM project_members pm JOIN projects p ON pm.project_id = p.id WHERE pm.account_id = sqlc.arg(account_id) AND pm.status = 'active'
    UNION
    SELECT p.organization_id FROM site_members sm JOIN sites s ON sm.site_id = s.id JOIN projects p ON s.project_id = p.id WHERE sm.account_id = sqlc.arg(account_id) AND sm.status = 'active'
);