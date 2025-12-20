-- ============================================================================
-- ORGANIZATION SETTINGS
-- ============================================================================

-- name: CreateOrganizationSetting :exec
INSERT INTO organization_settings (
    public_id, organization_id, setting_key, setting_value, editable, description, status, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(sqlc.arg(public_id)), ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?, ?);

-- name: GetOrganizationSetting :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, setting_key, setting_value, editable, description, status, created_at, updated_at, created_by, updated_by
FROM organization_settings
WHERE organization_id = ? AND setting_key = ? AND status != 'deleted';

-- name: GetOrganizationSettingByPublicID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, setting_key, setting_value, editable, description, status, created_at, updated_at, created_by, updated_by
FROM organization_settings
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id)) AND status != 'deleted';

-- name: ListOrganizationSettings :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, setting_key, setting_value, editable, description, status, created_at, updated_at, created_by, updated_by
FROM organization_settings
WHERE organization_id = ? AND status != 'deleted'
ORDER BY setting_key ASC
LIMIT ? OFFSET ?;

-- name: UpdateOrganizationSetting :exec
UPDATE organization_settings
SET setting_value = ?, updated_at = NOW(), updated_by = ?
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: DeleteOrganizationSetting :exec
UPDATE organization_settings
SET status = 'deleted', updated_at = NOW(), updated_by = ?
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- ============================================================================
-- PROJECT SETTINGS
-- ============================================================================

-- name: CreateProjectSetting :exec
INSERT INTO project_settings (
    public_id, project_id, setting_key, setting_value, editable, description, status, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(sqlc.arg(public_id)), ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?, ?);

-- name: GetProjectSetting :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, setting_key, setting_value, editable, description, status, created_at, updated_at, created_by, updated_by
FROM project_settings
WHERE project_id = ? AND setting_key = ? AND status != 'deleted';

-- name: GetProjectSettingByPublicID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, setting_key, setting_value, editable, description, status, created_at, updated_at, created_by, updated_by
FROM project_settings
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id)) AND status != 'deleted';

-- name: ListProjectSettings :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, setting_key, setting_value, editable, description, status, created_at, updated_at, created_by, updated_by
FROM project_settings
WHERE project_id = ? AND status != 'deleted'
ORDER BY setting_key ASC
LIMIT ? OFFSET ?;

-- name: UpdateProjectSetting :exec
UPDATE project_settings
SET setting_value = ?, updated_at = NOW(), updated_by = ?
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: DeleteProjectSetting :exec
UPDATE project_settings
SET status = 'deleted', updated_at = NOW(), updated_by = ?
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- ============================================================================
-- SITE SETTINGS
-- ============================================================================

-- name: CreateSiteSetting :exec
INSERT INTO site_settings (
    public_id, site_id, setting_key, setting_value, editable, description, status, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(sqlc.arg(public_id)), ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?, ?);

-- name: GetSiteSetting :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, site_id, setting_key, setting_value, editable, description, status, created_at, updated_at, created_by, updated_by
FROM site_settings
WHERE site_id = ? AND setting_key = ? AND status != 'deleted';

-- name: GetSiteSettingByPublicID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, site_id, setting_key, setting_value, editable, description, status, created_at, updated_at, created_by, updated_by
FROM site_settings
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id)) AND status != 'deleted';

-- name: ListSiteSettings :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, site_id, setting_key, setting_value, editable, description, status, created_at, updated_at, created_by, updated_by
FROM site_settings
WHERE site_id = ? AND status != 'deleted'
ORDER BY setting_key ASC
LIMIT ? OFFSET ?;

-- name: UpdateSiteSetting :exec
UPDATE site_settings
SET setting_value = ?, updated_at = NOW(), updated_by = ?
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: DeleteSiteSetting :exec
UPDATE site_settings
SET status = 'deleted', updated_at = NOW(), updated_by = ?
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- ============================================================================
-- USER SETTINGS (Cross-scope query for dashboard)
-- ============================================================================

-- name: ListUserSettings :many
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
        os.id, BIN_TO_UUID(os.public_id) AS public_id, os.setting_key, os.setting_value, os.editable, os.description, os.status, os.created_at, os.updated_at,
        'organization' AS parent_type,
        o.name AS parent_name,
        BIN_TO_UUID(o.public_id) AS parent_public_id
    FROM organization_settings os
    JOIN organizations o ON os.organization_id = o.id
    JOIN user_orgs uo ON o.id = uo.organization_id
    WHERE os.status != 'deleted'

    UNION ALL

    SELECT
        ps.id, BIN_TO_UUID(ps.public_id) AS public_id, ps.setting_key, ps.setting_value, ps.editable, ps.description, ps.status, ps.created_at, ps.updated_at,
        'project' AS parent_type,
        p.name AS parent_name,
        BIN_TO_UUID(p.public_id) AS parent_public_id
    FROM project_settings ps
    JOIN projects p ON ps.project_id = p.id
    LEFT JOIN project_members pm ON p.id = pm.project_id AND pm.account_id = sqlc.arg(account_id) AND pm.status = 'active'
    LEFT JOIN user_orgs uo ON p.organization_id = uo.organization_id
    WHERE ps.status != 'deleted'
    AND (pm.id IS NOT NULL OR uo.organization_id IS NOT NULL)

    UNION ALL

    SELECT
        ss.id, BIN_TO_UUID(ss.public_id) AS public_id, ss.setting_key, ss.setting_value, ss.editable, ss.description, ss.status, ss.created_at, ss.updated_at,
        'site' AS parent_type,
        s.name AS parent_name,
        BIN_TO_UUID(s.public_id) AS parent_public_id
    FROM site_settings ss
    JOIN sites s ON ss.site_id = s.id
    JOIN projects p ON s.project_id = p.id
    LEFT JOIN site_members sm ON s.id = sm.site_id AND sm.account_id = sqlc.arg(account_id) AND sm.status = 'active'
    LEFT JOIN project_members pm ON p.id = pm.project_id AND pm.account_id = sqlc.arg(account_id) AND pm.status = 'active'
    LEFT JOIN user_orgs uo ON p.organization_id = uo.organization_id
    WHERE ss.status != 'deleted'
    AND (sm.id IS NOT NULL OR pm.id IS NOT NULL OR uo.organization_id IS NOT NULL)
) AS all_settings
ORDER BY created_at DESC
LIMIT ? OFFSET ?;
