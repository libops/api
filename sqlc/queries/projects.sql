-- name: ListSshKeysByProject :many
SELECT DISTINCT sk.public_key
FROM ssh_keys sk
JOIN accounts a ON sk.account_id = a.id
JOIN (
    SELECT DISTINCT pm.account_id
    FROM project_members pm
    WHERE pm.project_id = (SELECT id FROM projects WHERE public_id = UUID_TO_BIN(sqlc.arg(project_public_id)))
      AND pm.status = 'active'
      AND pm.role IN ('owner', 'developer')
    UNION
    SELECT DISTINCT om.account_id
    FROM organization_members om
    JOIN projects p ON om.organization_id = p.organization_id
    WHERE p.public_id = UUID_TO_BIN(sqlc.arg(project_public_id))
      AND om.status = 'active'
      AND om.role IN ('owner', 'developer')
    UNION
    -- Include members from related organizations with approved relationships
    SELECT DISTINCT om_related.account_id
    FROM organization_members om_related
    JOIN relationships r ON om_related.organization_id = r.target_organization_id
    JOIN projects p ON r.source_organization_id = p.organization_id
    WHERE p.public_id = UUID_TO_BIN(sqlc.arg(project_public_id))
      AND r.status = 'approved'
      AND om_related.status = 'active'
      AND om_related.role IN ('owner', 'developer')
) AS authorized_accounts ON a.id = authorized_accounts.account_id
ORDER BY sk.created_at DESC;


-- name: GetProject :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, `name`,
       gcp_region, gcp_zone, machine_type, disk_size_gb, os, disk_type, stripe_subscription_item_id,
       promote_strategy,
       monitoring_enabled, monitoring_log_level, monitoring_metrics_enabled, monitoring_health_check_path,
       gcp_project_id, gcp_project_number, create_branch_sites, `status`,
       created_at, updated_at, created_by, updated_by
FROM projects WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: GetProjectByID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, `name`,
       gcp_region, gcp_zone, machine_type, disk_size_gb, os, disk_type, stripe_subscription_item_id,
       promote_strategy,
       monitoring_enabled, monitoring_log_level, monitoring_metrics_enabled, monitoring_health_check_path,
       gcp_project_id, gcp_project_number, create_branch_sites, `status`,
       created_at, updated_at, created_by, updated_by
FROM projects WHERE id = ?;


-- name: GetProjectByGCPProjectID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, `name`,
       gcp_region, gcp_zone, machine_type, disk_size_gb, os, disk_type, stripe_subscription_item_id,
       promote_strategy,
       monitoring_enabled, monitoring_log_level, monitoring_metrics_enabled, monitoring_health_check_path,
       gcp_project_id, gcp_project_number, create_branch_sites, `status`,
       created_at, updated_at, created_by, updated_by
FROM projects WHERE gcp_project_id = ?;


-- name: CreateProject :exec
INSERT INTO projects (
  public_id, organization_id, `name`,
  gcp_region, gcp_zone, machine_type, disk_size_gb, os, disk_type, stripe_subscription_item_id,
  monitoring_enabled, monitoring_log_level, monitoring_metrics_enabled, monitoring_health_check_path,
  gcp_project_id, gcp_project_number, create_branch_sites, `status`,
  created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(sqlc.arg(public_id)), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?, ?);


-- name: UpdateProject :exec
UPDATE projects SET
  `name` = ?,
  gcp_region = ?,
  gcp_zone = ?,
  machine_type = ?,
  disk_size_gb = ?,
  os = ?,
  disk_type = ?,
  stripe_subscription_item_id = ?,
  monitoring_enabled = ?,
  monitoring_log_level = ?,
  monitoring_metrics_enabled = ?,
  monitoring_health_check_path = ?,
  gcp_project_id = ?,
  gcp_project_number = ?,
  create_branch_sites = ?,
  `status` = ?,
  updated_at = NOW(),
  updated_by = ?
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: DeleteProject :exec
DELETE FROM projects WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: ListProjects :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, name, gcp_region, gcp_zone, machine_type, disk_size_gb, stripe_subscription_item_id, promote_strategy, monitoring_enabled, monitoring_log_level, monitoring_metrics_enabled, monitoring_health_check_path, gcp_project_id, gcp_project_number, organization_project, create_branch_sites, status, created_at, updated_at, created_by, updated_by
FROM projects
ORDER BY created_at DESC
LIMIT ? OFFSET ?;


-- name: ListUserProjects :many
WITH RECURSIVE user_orgs AS (
    SELECT organization_id FROM organization_members WHERE organization_members.account_id = sqlc.arg(account_id) AND organization_members.status = 'active'
    UNION DISTINCT
    SELECT r.target_organization_id
    FROM relationships r
    INNER JOIN user_orgs uo ON r.source_organization_id = uo.organization_id
    WHERE r.status = 'approved'
)
SELECT DISTINCT p.id, BIN_TO_UUID(p.public_id) AS public_id, p.organization_id, BIN_TO_UUID(o.public_id) AS organization_public_id, p.name, p.gcp_region, p.gcp_zone, p.machine_type, p.disk_size_gb, p.os, p.disk_type, p.stripe_subscription_item_id, p.promote_strategy, p.monitoring_enabled, p.monitoring_log_level, p.monitoring_metrics_enabled, p.monitoring_health_check_path, p.gcp_project_id, p.gcp_project_number, p.organization_project, p.create_branch_sites, p.status, p.created_at, p.updated_at, p.created_by, p.updated_by
FROM projects p
JOIN organizations o ON p.organization_id = o.id
LEFT JOIN project_members pm ON p.id = pm.project_id AND pm.account_id = sqlc.arg(account_id) AND pm.status = 'active'
LEFT JOIN user_orgs uo ON p.organization_id = uo.organization_id
WHERE (pm.id IS NOT NULL OR uo.organization_id IS NOT NULL)
AND (p.organization_id = sqlc.narg(filter_organization_id) OR sqlc.narg(filter_organization_id) IS NULL)
ORDER BY p.created_at DESC
LIMIT ? OFFSET ?;

-- =============================================================================
-- SITES
-- =============================================================================


-- name: GetSiteByProjectAndName :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, `name`, github_repository, github_ref, github_team_id, compose_path, compose_file, port, application_type, up_cmd, init_cmd, rollout_cmd, overlay_volumes, os, is_production, gcp_external_ip, `status`,
       created_at, updated_at, created_by, updated_by
FROM sites WHERE project_id = ? AND `name` = ?;


-- name: ListProjectSites :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, name, github_repository, github_ref, github_team_id, compose_path, compose_file, port, application_type, up_cmd, init_cmd, rollout_cmd, overlay_volumes, os, is_production, gcp_external_ip, status, created_at, updated_at, created_by, updated_by
FROM sites
WHERE project_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;


-- name: ListUserProjectsWithOrg :many
WITH RECURSIVE user_orgs AS (
    SELECT organization_id FROM organization_members WHERE organization_members.account_id = sqlc.arg(account_id) AND organization_members.status = 'active'
    UNION DISTINCT
    SELECT r.target_organization_id
    FROM relationships r
    INNER JOIN user_orgs uo ON r.source_organization_id = uo.organization_id
    WHERE r.status = 'approved'
)
SELECT DISTINCT p.id, BIN_TO_UUID(p.public_id) AS public_id, p.organization_id, BIN_TO_UUID(o.public_id) AS organization_public_id, o.name AS organization_name, p.name, p.gcp_region, p.gcp_zone, p.machine_type, p.disk_size_gb, p.stripe_subscription_item_id, p.promote_strategy, p.monitoring_enabled, p.monitoring_log_level, p.monitoring_metrics_enabled, p.monitoring_health_check_path, p.gcp_project_id, p.gcp_project_number, p.organization_project, p.create_branch_sites, p.status, p.created_at, p.updated_at, p.created_by, p.updated_by
FROM projects p
JOIN organizations o ON p.organization_id = o.id
LEFT JOIN project_members pm ON p.id = pm.project_id AND pm.account_id = sqlc.arg(account_id) AND pm.status = 'active'
LEFT JOIN user_orgs uo ON p.organization_id = uo.organization_id
WHERE (pm.id IS NOT NULL OR uo.organization_id IS NOT NULL)
AND (p.organization_id = sqlc.narg(filter_organization_id) OR sqlc.narg(filter_organization_id) IS NULL)
ORDER BY p.created_at DESC
LIMIT ? OFFSET ?;


-- name: ListUserSitesWithProject :many
WITH RECURSIVE user_orgs AS (
    SELECT organization_id FROM organization_members WHERE organization_members.account_id = sqlc.arg(account_id) AND organization_members.status = 'active'
    UNION DISTINCT
    SELECT r.target_organization_id
    FROM relationships r
    INNER JOIN user_orgs uo ON r.source_organization_id = uo.organization_id
    WHERE r.status = 'approved'
)
SELECT DISTINCT s.id, BIN_TO_UUID(s.public_id) AS public_id, s.project_id, BIN_TO_UUID(p.public_id) AS project_public_id, p.name AS project_name, BIN_TO_UUID(o.public_id) AS organization_public_id, s.name, s.github_repository, s.github_ref, s.github_team_id, s.compose_path, s.compose_file, s.port, s.application_type, s.up_cmd, s.init_cmd, s.rollout_cmd, s.gcp_external_ip, s.status, s.created_at, s.updated_at, s.created_by, s.updated_by
FROM sites s
JOIN projects p ON s.project_id = p.id
JOIN organizations o ON p.organization_id = o.id
LEFT JOIN site_members sm ON s.id = sm.site_id AND sm.account_id = sqlc.arg(account_id) AND sm.status = 'active'
LEFT JOIN project_members pm ON s.project_id = pm.project_id AND pm.account_id = sqlc.arg(account_id) AND pm.status = 'active'
LEFT JOIN user_orgs uo ON p.organization_id = uo.organization_id
WHERE (sm.id IS NOT NULL OR pm.id IS NOT NULL OR uo.organization_id IS NOT NULL)
AND (p.organization_id = sqlc.narg(filter_organization_id) OR sqlc.narg(filter_organization_id) IS NULL)
AND (s.project_id = sqlc.narg(filter_project_id) OR sqlc.narg(filter_project_id) IS NULL)
ORDER BY s.created_at DESC
LIMIT ? OFFSET ?;


-- name: GetProjectFirewallRuleByPublicID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, rule_type, cidr, name, status, created_at, updated_at, created_by, updated_by
FROM project_firewall_rules WHERE public_id = UUID_TO_BIN(?);


-- name: CreateProjectFirewallRule :exec
INSERT INTO project_firewall_rules (
  public_id, project_id, name, rule_type, cidr, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?);


-- name: DeleteProjectFirewallRule :exec
DELETE FROM project_firewall_rules WHERE id = ?;


-- name: DeleteProjectFirewallRuleByPublicID :exec
UPDATE project_firewall_rules SET status = 'deleted', updated_at = CURRENT_TIMESTAMP WHERE public_id = UUID_TO_BIN(?);


-- name: ListProjectFirewallRules :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, rule_type, cidr, name, status, created_at, updated_at, created_by, updated_by
FROM project_firewall_rules
WHERE project_id = ? AND status != 'deleted'
ORDER BY created_at DESC;

-- =============================================================================
-- SITE FIREWALL RULES
-- =============================================================================


-- name: CreateProjectSecret :execresult
INSERT INTO project_secrets (
    public_id, project_id, name, vault_path, status, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(sqlc.arg(public_id)), ?, ?, ?, ?, ?, ?, ?, ?);


-- name: GetProjectSecretByID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, name, vault_path, status,
       created_at, updated_at, created_by, updated_by
FROM project_secrets WHERE id = ? AND status != 'deleted';


-- name: GetProjectSecretByPublicID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, name, vault_path, status,
       created_at, updated_at, created_by, updated_by
FROM project_secrets WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id)) AND status != 'deleted';


-- name: GetProjectSecretByName :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, name, vault_path, status,
       created_at, updated_at, created_by, updated_by
FROM project_secrets
WHERE project_id = ? AND name = ? AND status != 'deleted';


-- name: ListProjectSecrets :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, name, vault_path, status,
       created_at, updated_at, created_by, updated_by
FROM project_secrets
WHERE project_id = ? AND status != 'deleted'
ORDER BY name ASC
LIMIT ? OFFSET ?;


-- name: CountProjectSecrets :one
SELECT COUNT(*) FROM project_secrets
WHERE project_id = ? AND status != 'deleted';


-- name: UpdateProjectSecret :exec
UPDATE project_secrets
SET vault_path = ?, updated_by = ?, updated_at = ?
WHERE id = ?;


-- name: DeleteProjectSecret :exec
UPDATE project_secrets
SET status = 'deleted', updated_by = ?, updated_at = ?
WHERE id = ?;

-- =============================================================================
-- SITE SECRETS
-- =============================================================================


-- name: HasUserSiteAccessInProject :one
SELECT EXISTS (
    SELECT 1 FROM site_members sm
    JOIN sites s ON sm.site_id = s.id
    JOIN projects p ON s.project_id = p.id
    JOIN projects p_target ON p_target.id = ?
    LEFT JOIN relationships r ON (
        r.source_organization_id = p.organization_id AND r.target_organization_id = p_target.organization_id
    )
    WHERE sm.account_id = ?
      AND (s.project_id = ? OR (r.status = 'approved' AND r.id IS NOT NULL))
      AND sm.status = 'active'
    LIMIT 1
);


-- name: CountOrganizationProjects :one
SELECT COUNT(*) as count
FROM projects
WHERE organization_id = ? AND status != 'deleted';


