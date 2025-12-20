-- name: ListUserSecrets :many

-- name: ListUserSecrets :many
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
        os.id, BIN_TO_UUID(os.public_id) AS public_id, os.name, os.status, os.created_at, os.updated_at,
        'organization' AS parent_type,
        o.name AS parent_name,
        BIN_TO_UUID(o.public_id) AS parent_public_id
    FROM organization_secrets os
    JOIN organizations o ON os.organization_id = o.id
    JOIN user_orgs uo ON o.id = uo.organization_id
    WHERE os.status != 'deleted'

    UNION ALL

    SELECT
        ps.id, BIN_TO_UUID(ps.public_id) AS public_id, ps.name, ps.status, ps.created_at, ps.updated_at,
        'project' AS parent_type,
        p.name AS parent_name,
        BIN_TO_UUID(p.public_id) AS parent_public_id
    FROM project_secrets ps
    JOIN projects p ON ps.project_id = p.id
    LEFT JOIN project_members pm ON p.id = pm.project_id AND pm.account_id = sqlc.arg(account_id) AND pm.status = 'active'
    LEFT JOIN user_orgs uo ON p.organization_id = uo.organization_id
    WHERE ps.status != 'deleted'
    AND (pm.id IS NOT NULL OR uo.organization_id IS NOT NULL)

    UNION ALL

    SELECT
        ss.id, BIN_TO_UUID(ss.public_id) AS public_id, ss.name, ss.status, ss.created_at, ss.updated_at,
        'site' AS parent_type,
        s.name AS parent_name,
        BIN_TO_UUID(s.public_id) AS parent_public_id
    FROM site_secrets ss
    JOIN sites s ON ss.site_id = s.id
    JOIN projects p ON s.project_id = p.id
    LEFT JOIN site_members sm ON s.id = sm.site_id AND sm.account_id = sqlc.arg(account_id) AND sm.status = 'active'
    LEFT JOIN project_members pm ON p.id = pm.project_id AND pm.account_id = sqlc.arg(account_id) AND pm.status = 'active'
    LEFT JOIN user_orgs uo ON p.organization_id = uo.organization_id
    WHERE ss.status != 'deleted'
    AND (sm.id IS NOT NULL OR pm.id IS NOT NULL OR uo.organization_id IS NOT NULL)
) AS all_secrets
ORDER BY created_at DESC
LIMIT ? OFFSET ?;


