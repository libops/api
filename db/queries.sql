-- name: GetOrganization :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, `name`, gcp_org_id, gcp_billing_account, gcp_parent, gcp_folder_id, `status`, gcp_project_id, gcp_project_number, created_at, updated_at, created_by, updated_by
FROM organizations WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: GetOrganizationByID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, `name`, gcp_org_id, gcp_billing_account, gcp_parent, gcp_folder_id, `status`, gcp_project_id, gcp_project_number, created_at, updated_at, created_by, updated_by
FROM organizations WHERE id = ?;

-- name: GetOrganizationByGCPProjectID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, `name`, gcp_org_id, gcp_billing_account, gcp_parent, gcp_folder_id, `status`, gcp_project_id, gcp_project_number, created_at, updated_at, created_by, updated_by
FROM organizations WHERE gcp_project_id = ?;

-- name: CreateOrganization :exec
INSERT INTO organizations (
  public_id, `name`, gcp_org_id, gcp_billing_account, gcp_parent, gcp_folder_id, `status`, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(sqlc.arg(public_id)), ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?, ?);

-- name: UpdateOrganization :exec
UPDATE organizations SET
  `name` = ?,
  gcp_org_id = ?,
  gcp_billing_account = ?,
  gcp_parent = ?,
  gcp_folder_id = ?,
  `status` = ?,
  updated_at = NOW(),
  updated_by = ?
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: DeleteOrganization :exec
DELETE FROM organizations WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: ListOrganizations :many
WITH RECURSIVE user_orgs AS (
    SELECT organization_id FROM organization_members WHERE account_id = ? AND status = 'active'
    UNION DISTINCT
    SELECT r.target_organization_id
    FROM relationships r
    INNER JOIN user_orgs uo ON r.source_organization_id = uo.organization_id
    WHERE r.status = 'approved'
)
SELECT DISTINCT o.id, BIN_TO_UUID(o.public_id) AS public_id, o.name, o.gcp_org_id, o.gcp_billing_account, o.gcp_parent, o.location, o.region, o.gcp_folder_id, o.status, o.gcp_project_id, o.gcp_project_number, o.created_at, o.updated_at, o.created_by, o.updated_by
FROM organizations o
INNER JOIN user_orgs uo ON o.id = uo.organization_id
ORDER BY o.created_at DESC
LIMIT ? OFFSET ?;

-- =============================================================================
-- ACCOUNTS
-- =============================================================================

-- name: GetAccount :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, email, `name`, github_username, vault_entity_id,
       auth_method, verified, verified_at, created_at, updated_at
FROM accounts WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: GetAccountByID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, email, `name`, github_username, vault_entity_id,
       auth_method, verified, verified_at, created_at, updated_at
FROM accounts WHERE id = ?;

-- name: GetAccountByEmail :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, email, `name`, github_username, vault_entity_id,
       auth_method, verified, verified_at, failed_login_attempts, last_failed_login_at, created_at, updated_at
FROM accounts WHERE email = ?;

-- name: GetAccountByVaultEntityID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, email, `name`, github_username, vault_entity_id,
       auth_method, verified, verified_at, created_at, updated_at
FROM accounts WHERE vault_entity_id = ?;

-- name: CreateAccount :exec
INSERT INTO accounts (
  public_id, email, `name`, github_username, vault_entity_id, auth_method, verified, verified_at, created_at, updated_at
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, ?, ?, ?, ?, NOW(), NOW());

-- name: UpdateAccount :exec
UPDATE accounts SET
  email = ?,
  `name` = ?,
  github_username = ?,
  vault_entity_id = ?,
  auth_method = ?,
  verified = ?,
  verified_at = ?,
  updated_at = NOW()
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: IncrementFailedLoginAttempts :exec
UPDATE accounts SET
  failed_login_attempts = failed_login_attempts + 1,
  last_failed_login_at = NOW()
WHERE id = ?;

-- name: ResetFailedLoginAttempts :exec
UPDATE accounts SET
  failed_login_attempts = 0
WHERE id = ?;

-- name: DeleteAccount :exec
DELETE FROM accounts WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: ListAccounts :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, email, name, github_username, vault_entity_id, auth_method, verified, verified_at, failed_login_attempts, last_failed_login_at, created_at, updated_at
FROM accounts
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- =============================================================================
-- API KEYS
-- =============================================================================

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

-- name: ListAPIKeysByAccount :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, account_id, `name`, description,
       COALESCE(scopes, '[]') as scopes,
       created_at, last_used_at, expires_at, active, created_by
FROM api_keys
WHERE account_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

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

-- name: GetOrganizationMember :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, account_id, `role`, created_at, updated_at, created_by, updated_by
FROM organization_members WHERE organization_id = ? AND account_id = ?;

-- name: CreateOrganizationMember :exec
INSERT INTO organization_members (
  public_id, organization_id, account_id, `role`, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, NOW(), NOW(), ?, ?);

-- name: UpdateOrganizationMember :exec
UPDATE organization_members SET
  `role` = ?,
  updated_at = NOW(),
  updated_by = ?
WHERE organization_id = ? AND account_id = ?;

-- name: DeleteOrganizationMember :exec
DELETE FROM organization_members WHERE organization_id = ? AND account_id = ?;

-- name: ListOrganizationMembers :many
SELECT cm.id, BIN_TO_UUID(cm.public_id) AS public_id, cm.organization_id, cm.account_id, cm.`role`, cm.status, cm.created_at, cm.updated_at,
       BIN_TO_UUID(a.public_id) AS account_public_id, a.email, a.`name`, a.github_username, a.verified, a.auth_method
FROM organization_members cm
JOIN accounts a ON cm.account_id = a.id
WHERE cm.organization_id = ?
ORDER BY cm.created_at DESC
LIMIT ? OFFSET ?;

-- name: ListAccountOrganizations :many
SELECT c.id, BIN_TO_UUID(c.public_id) AS public_id, c.`name`, cm.`role`
FROM organization_members cm
JOIN organizations c ON cm.organization_id = c.id
WHERE cm.account_id = ?
ORDER BY c.created_at DESC
LIMIT ? OFFSET ?;

-- =============================================================================
-- Ssh KEYS
-- =============================================================================

-- name: GetSshKey :one
SELECT sk.id, BIN_TO_UUID(sk.public_id) AS public_id,
       BIN_TO_UUID(a.public_id) AS account_public_id,
       sk.public_key, sk.`name`, sk.fingerprint,
       sk.created_at, sk.updated_at
FROM ssh_keys sk
JOIN accounts a ON sk.account_id = a.id
WHERE sk.public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: ListSshKeysByAccount :many
SELECT sk.id, BIN_TO_UUID(sk.public_id) AS public_id,
       BIN_TO_UUID(a.public_id) AS account_public_id,
       sk.public_key, sk.`name`, sk.fingerprint,
       sk.created_at, sk.updated_at
FROM ssh_keys sk
JOIN accounts a ON sk.account_id = a.id
WHERE a.public_id = UUID_TO_BIN(sqlc.arg(public_id))
ORDER BY sk.created_at DESC;

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

-- name: ListSshKeysBySite :many
SELECT DISTINCT sk.public_key
FROM ssh_keys sk
JOIN accounts a ON sk.account_id = a.id
JOIN (
    SELECT DISTINCT sm.account_id
    FROM site_members sm
    WHERE sm.site_id = (SELECT id FROM sites WHERE public_id = UUID_TO_BIN(sqlc.arg(site_public_id)))
      AND sm.status = 'active'
      AND sm.role IN ('owner', 'developer')
    UNION
    SELECT DISTINCT pm.account_id
    FROM project_members pm
    JOIN sites s ON pm.project_id = s.project_id
    WHERE s.public_id = UUID_TO_BIN(sqlc.arg(site_public_id))
      AND pm.status = 'active'
      AND pm.role IN ('owner', 'developer')
    UNION
    SELECT DISTINCT om.account_id
    FROM organization_members om
    JOIN projects p ON om.organization_id = p.organization_id
    JOIN sites s ON s.project_id = p.id
    WHERE s.public_id = UUID_TO_BIN(sqlc.arg(site_public_id))
      AND om.status = 'active'
      AND om.role IN ('owner', 'developer')
    UNION
    -- Include members from related organizations with approved relationships
    SELECT DISTINCT om_related.account_id
    FROM organization_members om_related
    JOIN relationships r ON om_related.organization_id = r.target_organization_id
    JOIN projects p ON r.source_organization_id = p.organization_id
    JOIN sites s ON s.project_id = p.id
    WHERE s.public_id = UUID_TO_BIN(sqlc.arg(site_public_id))
      AND r.status = 'approved'
      AND om_related.status = 'active'
      AND om_related.role IN ('owner', 'developer')
) AS authorized_accounts ON a.id = authorized_accounts.account_id
ORDER BY sk.created_at DESC;

-- =============================================================================
-- PROJECT MEMBERS
-- =============================================================================

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

-- name: ListAccountProjects :many
SELECT p.id, BIN_TO_UUID(p.public_id) AS public_id, p.`name`, pm.`role`
FROM project_members pm
JOIN projects p ON pm.project_id = p.id
WHERE pm.account_id = ?
ORDER BY p.created_at DESC
LIMIT ? OFFSET ?;

-- =============================================================================
-- PROJECTS
-- =============================================================================

-- name: GetProject :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, `name`,
       github_repository, github_branch, compose_path,
       gcp_region, gcp_zone, machine_type, disk_size_gb, compose_file, application_type,
       promote_strategy,
       monitoring_enabled, monitoring_log_level, monitoring_metrics_enabled, monitoring_health_check_path,
       gcp_project_id, gcp_project_number, github_team_id, create_branch_sites, `status`,
       created_at, updated_at, created_by, updated_by
FROM projects WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: GetProjectByID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, `name`,
       github_repository, github_branch, compose_path,
       gcp_region, gcp_zone, machine_type, disk_size_gb, compose_file, application_type,
       promote_strategy,
       monitoring_enabled, monitoring_log_level, monitoring_metrics_enabled, monitoring_health_check_path,
       gcp_project_id, gcp_project_number, github_team_id, create_branch_sites, `status`,
       created_at, updated_at, created_by, updated_by
FROM projects WHERE id = ?;

-- name: GetProjectByGCPProjectID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, `name`,
       github_repository, github_branch, compose_path,
       gcp_region, gcp_zone, machine_type, disk_size_gb, compose_file, application_type,
       promote_strategy,
       monitoring_enabled, monitoring_log_level, monitoring_metrics_enabled, monitoring_health_check_path,
       gcp_project_id, gcp_project_number, github_team_id, create_branch_sites, `status`,
       created_at, updated_at, created_by, updated_by
FROM projects WHERE gcp_project_id = ?;

-- name: GetProjectWithOrganization :one
SELECT
    p.id, BIN_TO_UUID(p.public_id) AS public_id, p.organization_id, p.name,
    p.github_repository, p.github_branch, p.compose_path,
    p.gcp_region, p.gcp_zone, p.machine_type, p.disk_size_gb, p.compose_file, p.application_type,
    p.promote_strategy,
    p.monitoring_enabled, p.monitoring_log_level, p.monitoring_metrics_enabled, p.monitoring_health_check_path,
    p.gcp_project_id, p.gcp_project_number, p.github_team_id, p.create_branch_sites, p.status,
    p.created_at, p.updated_at, p.created_by, p.updated_by,
    c.gcp_billing_account
FROM projects p
JOIN organizations c ON p.organization_id = c.id
WHERE p.public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: GetOrganizationProjectByOrganizationID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, `name`,
       github_repository, github_branch, compose_path,
       gcp_region, gcp_zone, machine_type, disk_size_gb, compose_file, application_type,
       promote_strategy,
       monitoring_enabled, monitoring_log_level, monitoring_metrics_enabled, monitoring_health_check_path,
       gcp_project_id, gcp_project_number, github_team_id, create_branch_sites, organization_project, `status`,
       created_at, updated_at, created_by, updated_by
FROM projects
WHERE organization_id = ? AND organization_project = TRUE
LIMIT 1;

-- name: CreateProject :exec
INSERT INTO projects (
  public_id, organization_id, `name`, github_repository, github_branch, compose_path,
  gcp_region, gcp_zone, machine_type, disk_size_gb, compose_file, application_type,
  monitoring_enabled, monitoring_log_level, monitoring_metrics_enabled, monitoring_health_check_path,
  gcp_project_id, gcp_project_number, github_team_id, create_branch_sites, `status`,
  created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?, ?);

-- name: UpdateProject :exec
UPDATE projects SET
  `name` = ?,
  github_repository = ?,
  github_branch = ?,
  compose_path = ?,
  gcp_region = ?,
  gcp_zone = ?,
  machine_type = ?,
  disk_size_gb = ?,
  compose_file = ?,
  application_type = ?,
  monitoring_enabled = ?,
  monitoring_log_level = ?,
  monitoring_metrics_enabled = ?,
  monitoring_health_check_path = ?,
  gcp_project_id = ?,
  gcp_project_number = ?,
  github_team_id = ?,
  create_branch_sites = ?,
  `status` = ?,
  updated_at = NOW(),
  updated_by = ?
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: DeleteProject :exec
DELETE FROM projects WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: ListProjects :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, name, github_repository, github_repository_template, github_branch, compose_path, gcp_region, gcp_zone, machine_type, disk_size_gb, compose_file, application_type, promote_strategy, monitoring_enabled, monitoring_log_level, monitoring_metrics_enabled, monitoring_health_check_path, gcp_project_id, gcp_project_number, github_team_id, organization_project, create_branch_sites, up_cmd, init_cmd, rollout_cmd, status, created_at, updated_at, created_by, updated_by
FROM projects
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: ListOrganizationProjects :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, name, github_repository, github_repository_template, github_branch, compose_path, gcp_region, gcp_zone, machine_type, disk_size_gb, compose_file, application_type, promote_strategy, monitoring_enabled, monitoring_log_level, monitoring_metrics_enabled, monitoring_health_check_path, gcp_project_id, gcp_project_number, github_team_id, organization_project, create_branch_sites, up_cmd, init_cmd, rollout_cmd, status, created_at, updated_at, created_by, updated_by
FROM projects
WHERE organization_id = ?
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
SELECT DISTINCT p.id, BIN_TO_UUID(p.public_id) AS public_id, p.organization_id, BIN_TO_UUID(o.public_id) AS organization_public_id, p.name, p.github_repository, p.github_repository_template, p.github_branch, p.compose_path, p.gcp_region, p.gcp_zone, p.machine_type, p.disk_size_gb, p.compose_file, p.application_type, p.promote_strategy, p.monitoring_enabled, p.monitoring_log_level, p.monitoring_metrics_enabled, p.monitoring_health_check_path, p.gcp_project_id, p.gcp_project_number, p.github_team_id, p.organization_project, p.create_branch_sites, p.up_cmd, p.init_cmd, p.rollout_cmd, p.status, p.created_at, p.updated_at, p.created_by, p.updated_by
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

-- name: GetSite :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, `name`, github_ref, gcp_external_ip, `status`,
       created_at, updated_at, created_by, updated_by
FROM sites WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: GetSiteByID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, `name`, github_ref, gcp_external_ip, `status`,
       created_at, updated_at, created_by, updated_by
FROM sites WHERE id = ?;

-- name: GetSiteByProjectAndName :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, `name`, github_ref, gcp_external_ip, `status`,
       created_at, updated_at, created_by, updated_by
FROM sites WHERE project_id = ? AND `name` = ?;

-- name: CreateSite :exec
INSERT INTO sites (
  public_id, project_id, `name`, github_ref, gcp_external_ip, `status`, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, ?, ?, NOW(), NOW(), ?, ?);

-- name: UpdateSite :exec
UPDATE sites SET
  `name` = ?,
  github_ref = ?,
  gcp_external_ip = ?,
  `status` = ?,
  updated_at = NOW(),
  updated_by = ?
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: DeleteSite :exec
DELETE FROM sites WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: ListSites :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, name, github_ref, gcp_external_ip, status, created_at, updated_at, created_by, updated_by
FROM sites
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: ListProjectSites :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, name, github_ref, gcp_external_ip, status, created_at, updated_at, created_by, updated_by
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
SELECT DISTINCT p.id, BIN_TO_UUID(p.public_id) AS public_id, p.organization_id, BIN_TO_UUID(o.public_id) AS organization_public_id, o.name AS organization_name, p.name, p.github_repository, p.github_repository_template, p.github_branch, p.compose_path, p.gcp_region, p.gcp_zone, p.machine_type, p.disk_size_gb, p.compose_file, p.application_type, p.promote_strategy, p.monitoring_enabled, p.monitoring_log_level, p.monitoring_metrics_enabled, p.monitoring_health_check_path, p.gcp_project_id, p.gcp_project_number, p.github_team_id, p.organization_project, p.create_branch_sites, p.up_cmd, p.init_cmd, p.rollout_cmd, p.status, p.created_at, p.updated_at, p.created_by, p.updated_by
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
SELECT DISTINCT s.id, BIN_TO_UUID(s.public_id) AS public_id, s.project_id, BIN_TO_UUID(p.public_id) AS project_public_id, p.name AS project_name, BIN_TO_UUID(o.public_id) AS organization_public_id, s.name, s.github_ref, s.gcp_external_ip, s.status, s.created_at, s.updated_at, s.created_by, s.updated_by
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

-- name: ListUserSites :many
WITH RECURSIVE user_orgs AS (
    SELECT organization_id FROM organization_members WHERE organization_members.account_id = sqlc.arg(account_id) AND organization_members.status = 'active'
    UNION DISTINCT
    SELECT r.target_organization_id
    FROM relationships r
    INNER JOIN user_orgs uo ON r.source_organization_id = uo.organization_id
    WHERE r.status = 'approved'
)
SELECT DISTINCT s.id, BIN_TO_UUID(s.public_id) AS public_id, s.project_id, BIN_TO_UUID(p.public_id) AS project_public_id, BIN_TO_UUID(o.public_id) AS organization_public_id, s.name, s.github_ref, s.gcp_external_ip, s.status, s.created_at, s.updated_at, s.created_by, s.updated_by
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

-- =============================================================================
-- SITE MEMBERS
-- =============================================================================

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

-- name: ListAccountSites :many
SELECT s.id, BIN_TO_UUID(s.public_id) AS public_id, s.`name`, sm.`role`
FROM site_members sm
JOIN sites s ON sm.site_id = s.id
WHERE sm.account_id = ?
ORDER BY s.created_at DESC
LIMIT ? OFFSET ?;

-- =============================================================================
-- DOMAINS
-- =============================================================================

-- name: GetDomain :one
SELECT id, site_id, domain, created_at
FROM domains WHERE id = ?;

-- name: GetDomainByName :one
SELECT id, site_id, domain, created_at
FROM domains WHERE domain = ?;

-- name: CreateDomain :exec
INSERT INTO domains (
  site_id, domain, created_at
) VALUES (?, ?, NOW());

-- name: DeleteDomain :exec
DELETE FROM domains WHERE id = ?;

-- name: ListSiteDomains :many
SELECT * FROM domains
WHERE site_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- =============================================================================
-- ORGANIZATION FIREWALL RULES
-- =============================================================================

-- name: GetOrganizationFirewallRule :one
SELECT id, organization_id, rule_type, cidr, created_at, updated_at, created_by, updated_by
FROM organization_firewall_rules WHERE id = ?;

-- name: CreateOrganizationFirewallRule :exec
INSERT INTO organization_firewall_rules (
  public_id, organization_id, name, rule_type, cidr, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?);

-- name: DeleteOrganizationFirewallRule :exec
DELETE FROM organization_firewall_rules WHERE id = ?;

-- name: DeleteOrganizationFirewallRuleByPublicID :exec
DELETE FROM organization_firewall_rules WHERE public_id = UUID_TO_BIN(?);

-- name: ListOrganizationFirewallRules :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, rule_type, cidr, name, status, created_at, updated_at, created_by, updated_by
FROM organization_firewall_rules
WHERE organization_id = ?
ORDER BY created_at DESC;

-- =============================================================================
-- PROJECT FIREWALL RULES
-- =============================================================================

-- name: GetProjectFirewallRule :one
SELECT id, project_id, rule_type, cidr, created_at, updated_at, created_by, updated_by
FROM project_firewall_rules WHERE id = ?;

-- name: CreateProjectFirewallRule :exec
INSERT INTO project_firewall_rules (
  public_id, project_id, name, rule_type, cidr, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?);

-- name: DeleteProjectFirewallRule :exec
DELETE FROM project_firewall_rules WHERE id = ?;

-- name: DeleteProjectFirewallRuleByPublicID :exec
DELETE FROM project_firewall_rules WHERE public_id = UUID_TO_BIN(?);

-- name: ListProjectFirewallRules :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, rule_type, cidr, name, status, created_at, updated_at, created_by, updated_by
FROM project_firewall_rules
WHERE project_id = ?
ORDER BY created_at DESC;

-- =============================================================================
-- SITE FIREWALL RULES
-- =============================================================================

-- name: GetSiteFirewallRule :one
SELECT id, site_id, rule_type, cidr, created_at, updated_at, created_by, updated_by
FROM site_firewall_rules WHERE id = ?;

-- name: CreateSiteFirewallRule :exec
INSERT INTO site_firewall_rules (
  public_id, site_id, name, rule_type, cidr, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?);

-- name: DeleteSiteFirewallRule :exec
DELETE FROM site_firewall_rules WHERE id = ?;

-- name: DeleteSiteFirewallRuleByPublicID :exec
DELETE FROM site_firewall_rules WHERE public_id = UUID_TO_BIN(?);

-- name: ListSiteFirewallRules :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, site_id, rule_type, cidr, name, status, created_at, updated_at, created_by, updated_by
FROM site_firewall_rules
WHERE site_id = ?
ORDER BY created_at DESC;

-- =============================================================================
-- Ssh ACCESS
-- =============================================================================

-- name: GetSshAccess :one
SELECT id, account_id, site_id, created_at, updated_at, created_by, updated_by
FROM ssh_access WHERE account_id = ? AND site_id = ?;

-- name: CreateSshAccess :exec
INSERT INTO ssh_access (
  account_id, site_id, created_at, updated_at, created_by, updated_by
) VALUES (?, ?, NOW(), NOW(), ?, ?);

-- name: DeleteSshAccess :exec
DELETE FROM ssh_access WHERE account_id = ? AND site_id = ?;

-- name: ListAccountSshAccess :many
SELECT * FROM ssh_access
WHERE account_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;

-- name: ListSiteSshAccess :many
SELECT sa.id, sa.account_id, sa.site_id, sa.created_at, sa.updated_at,
       a.email, a.`name`, a.github_username
FROM ssh_access sa
JOIN accounts a ON sa.account_id = a.id
WHERE sa.site_id = ?
ORDER BY sa.created_at DESC
LIMIT ? OFFSET ?;

-- =============================================================================
-- DEPLOYMENTS
-- =============================================================================

-- name: GetDeployment :one
SELECT deployment_id, site_id, `status`, github_run_id, github_run_url, started_at, completed_at, error_message, created_at
FROM deployments WHERE deployment_id = ?;

-- name: CreateDeployment :exec
INSERT INTO deployments (
  deployment_id, site_id, `status`, github_run_id, github_run_url, started_at, completed_at, error_message, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW());

-- name: UpdateDeployment :exec
UPDATE deployments SET
  `status` = ?,
  github_run_id = ?,
  github_run_url = ?,
  completed_at = ?,
  error_message = ?
WHERE deployment_id = ?;

-- name: DeleteDeployment :exec
DELETE FROM deployments WHERE deployment_id = ?;

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

-- =============================================================================
-- RELATIONSHIPS
-- =============================================================================

-- name: GetRelationship :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, source_organization_id, target_organization_id,
       relationship_type, `status`, created_at, resolved_at, resolved_by
FROM relationships WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));

-- name: ListOrganizationRelationships :many
SELECT r.id, BIN_TO_UUID(r.public_id) AS public_id, r.source_organization_id, r.target_organization_id,
       r.relationship_type, r.`status`, r.created_at, r.resolved_at, r.resolved_by
FROM relationships r
WHERE r.source_organization_id = ? OR r.target_organization_id = ?
ORDER BY r.created_at DESC;

-- name: ListPendingApprovals :many
SELECT r.id, BIN_TO_UUID(r.public_id) AS public_id, r.source_organization_id, r.target_organization_id,
       r.relationship_type, r.`status`, r.created_at, r.resolved_at, r.resolved_by
FROM relationships r
WHERE r.target_organization_id = ? AND r.`status` = 'pending'
ORDER BY r.created_at DESC;

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

-- name: ListApprovedRelatedOrganizationsForAccount :many
-- Get all approved relationships for a source org where the account has access to the target org
WITH user_orgs AS (
    SELECT organization_id FROM organization_members WHERE account_id = ? AND status = 'active'
)
SELECT r.id, BIN_TO_UUID(r.public_id) AS public_id, r.source_organization_id, r.target_organization_id,
       r.relationship_type, r.`status`, r.created_at, r.resolved_at, r.resolved_by,
       BIN_TO_UUID(target_org.public_id) AS target_org_public_id
FROM relationships r
INNER JOIN organizations target_org ON r.target_organization_id = target_org.id
WHERE r.source_organization_id = ?
  AND r.`status` = 'approved'
  AND r.target_organization_id IN (SELECT organization_id FROM user_orgs)
ORDER BY r.created_at DESC;

-- =============================================================================
-- EVENT QUEUE
-- =============================================================================

-- name: EnqueueEvent :exec
INSERT INTO event_queue (
    event_id,
    event_type,
    event_source,
    event_subject,
    event_data,
    content_type,
    created_at
) VALUES (?, ?, ?, ?, ?, ?, NOW());

-- name: ClaimPendingEvents :execresult
UPDATE event_queue
SET status = 'processing',
    processing_by = ?,
    processing_at = NOW()
WHERE status = 'pending'
  AND retry_count < ?
ORDER BY created_at ASC
LIMIT ?;

-- name: GetClaimedEvents :many
SELECT
    id,
    event_id,
    event_type,
    event_source,
    event_subject,
    event_data,
    content_type,
    retry_count,
    created_at,
    last_retry_at
FROM event_queue
WHERE status = 'processing'
  AND processing_by = ?
ORDER BY created_at ASC;

-- name: MarkEventSent :exec
UPDATE event_queue
SET status = 'sent',
    sent_at = NOW()
WHERE id = ?;

-- name: MarkEventFailed :exec
UPDATE event_queue
SET retry_count = retry_count + 1,
    last_retry_at = NOW(),
    last_error = ?,
    status = 'pending',
    processing_by = NULL,
    processing_at = NULL
WHERE id = ?;

-- name: MarkEventDeadLetter :exec
UPDATE event_queue
SET status = 'dead_letter',
    last_error = ?
WHERE id = ?;

-- name: GetQueueStats :one
SELECT
    COUNT(*) as total_events,
    SUM(CASE WHEN status = 'pending' THEN 1 ELSE 0 END) as pending_events,
    SUM(CASE WHEN status = 'sent' THEN 1 ELSE 0 END) as sent_events,
    SUM(CASE WHEN status = 'dead_letter' THEN 1 ELSE 0 END) as dead_letter_events
FROM event_queue;

-- name: CleanupOldEvents :exec
DELETE FROM event_queue
WHERE status = 'sent'
  AND sent_at < DATE_SUB(NOW(), INTERVAL ? DAY);

-- name: RecoverStaleProcessing :exec
UPDATE event_queue
SET status = 'pending',
    processing_by = NULL,
    processing_at = NULL
WHERE status = 'processing'
  AND processing_at < DATE_SUB(NOW(), INTERVAL ? MINUTE);

-- name: CreateAuditEvent :exec
INSERT INTO audit (
  account_id, entity_id, entity_type, event_name, event_data
) VALUES (?, ?, ?, ?, ?);

-- =============================================================================
-- EMAIL VERIFICATION TOKENS
-- =============================================================================

-- name: CreateEmailVerificationToken :exec
INSERT INTO email_verification_tokens (
    email,
    token,
    password_hash,
    expires_at
) VALUES (?, ?, ?, ?);

-- name: GetEmailVerificationToken :one
SELECT id, email, token, password_hash, created_at, expires_at
FROM email_verification_tokens
WHERE email = ? AND token = ?
  AND expires_at > NOW();

-- name: GetEmailVerificationTokenByEmail :one
SELECT id, email, token, password_hash, created_at, expires_at
FROM email_verification_tokens
WHERE email = ?
  AND expires_at > NOW()
LIMIT 1;

-- name: DeleteEmailVerificationToken :exec
DELETE FROM email_verification_tokens
WHERE email = ?;

-- name: CleanupExpiredVerificationTokens :exec
DELETE FROM email_verification_tokens
WHERE expires_at < NOW();

-- =============================================================================
-- ORGANIZATION SECRETS
-- =============================================================================

-- name: CreateOrganizationSecret :execresult
INSERT INTO organization_secrets (
    public_id, organization_id, name, vault_path, status, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(sqlc.arg(public_id)), ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetOrganizationSecretByID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, name, vault_path, status,
       created_at, updated_at, created_by, updated_by
FROM organization_secrets WHERE id = ? AND status != 'deleted';

-- name: GetOrganizationSecretByPublicID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, name, vault_path, status,
       created_at, updated_at, created_by, updated_by
FROM organization_secrets WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id)) AND status != 'deleted';

-- name: GetOrganizationSecretByName :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, name, vault_path, status,
       created_at, updated_at, created_by, updated_by
FROM organization_secrets
WHERE organization_id = ? AND name = ? AND status != 'deleted';

-- name: ListOrganizationSecrets :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, name, vault_path, status,
       created_at, updated_at, created_by, updated_by
FROM organization_secrets
WHERE organization_id = ? AND status != 'deleted'
ORDER BY name ASC
LIMIT ? OFFSET ?;

-- name: CountOrganizationSecrets :one
SELECT COUNT(*) FROM organization_secrets
WHERE organization_id = ? AND status != 'deleted';

-- name: UpdateOrganizationSecret :exec
UPDATE organization_secrets
SET vault_path = ?, updated_by = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteOrganizationSecret :exec
UPDATE organization_secrets
SET status = 'deleted', updated_by = ?, updated_at = ?
WHERE id = ?;

-- =============================================================================
-- PROJECT SECRETS
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

-- name: CreateSiteSecret :execresult
INSERT INTO site_secrets (
    public_id, site_id, name, vault_path, status, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(sqlc.arg(public_id)), ?, ?, ?, ?, ?, ?, ?, ?);

-- name: GetSiteSecretByID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, site_id, name, vault_path, status,
       created_at, updated_at, created_by, updated_by
FROM site_secrets WHERE id = ? AND status != 'deleted';

-- name: GetSiteSecretByPublicID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, site_id, name, vault_path, status,
       created_at, updated_at, created_by, updated_by
FROM site_secrets WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id)) AND status != 'deleted';

-- name: GetSiteSecretByName :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, site_id, name, vault_path, status,
       created_at, updated_at, created_by, updated_by
FROM site_secrets
WHERE site_id = ? AND name = ? AND status != 'deleted';

-- name: ListSiteSecrets :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, site_id, name, vault_path, status,
       created_at, updated_at, created_by, updated_by
FROM site_secrets
WHERE site_id = ? AND status != 'deleted'
ORDER BY name ASC
LIMIT ? OFFSET ?;

-- name: CountSiteSecrets :one
SELECT COUNT(*) FROM site_secrets
WHERE site_id = ? AND status != 'deleted';

-- name: UpdateSiteSecret :exec
UPDATE site_secrets
SET vault_path = ?, updated_by = ?, updated_at = ?
WHERE id = ?;

-- name: DeleteSiteSecret :exec
UPDATE site_secrets
SET status = 'deleted', updated_by = ?, updated_at = ?
WHERE id = ?;

-- =============================================================================
-- MEMBERSHIP QUERIES FOR AUTHORIZATION
-- =============================================================================

-- name: GetOrganizationMemberByAccountAndOrganization :one
SELECT * FROM organization_members
WHERE account_id = ? AND organization_id = ? AND status = 'active';

-- name: GetProjectMemberByAccountAndProject :one
SELECT * FROM project_members
WHERE account_id = ? AND project_id = ? AND status = 'active';

-- name: GetSiteMemberByAccountAndSite :one
SELECT * FROM site_members
WHERE account_id = ? AND site_id = ? AND status = 'active';

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

-- name: HasUserRelationshipAccessToOrganization :one
WITH RECURSIVE user_access AS (
    SELECT organization_id
    FROM organization_members
    WHERE account_id = ? AND status = 'active'
    UNION DISTINCT
    SELECT r.target_organization_id
    FROM relationships r
    INNER JOIN user_access ua ON r.source_organization_id = ua.organization_id
    WHERE r.status = 'approved'
)
SELECT EXISTS (
    SELECT 1 FROM user_access WHERE organization_id = ?
);

-- name: HasUserProjectAccessInOrganization :one
SELECT EXISTS (
    SELECT 1 FROM project_members pm
    JOIN projects p ON pm.project_id = p.id
    LEFT JOIN relationships r ON (
        r.source_organization_id = p.organization_id AND r.target_organization_id = ?
    )
    WHERE pm.account_id = ?
      AND (p.organization_id = ? OR (r.status = 'approved' AND r.id IS NOT NULL))
      AND pm.status = 'active'
    LIMIT 1
);

-- name: HasUserSiteAccessInOrganization :one
SELECT EXISTS (
    SELECT 1 FROM site_members sm
    JOIN sites s ON sm.site_id = s.id
    JOIN projects p ON s.project_id = p.id
    LEFT JOIN relationships r ON (
        r.source_organization_id = p.organization_id AND r.target_organization_id = ?
    )
    WHERE sm.account_id = ?
      AND (p.organization_id = ? OR (r.status = 'approved' AND r.id IS NOT NULL))
      AND sm.status = 'active'
    LIMIT 1
);
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

-- name: ListUserFirewallRules :many
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
        ofr.id, BIN_TO_UUID(ofr.public_id) AS public_id, ofr.name, ofr.status, ofr.created_at, ofr.updated_at, ofr.rule_type, ofr.cidr,
        'organization' AS parent_type,
        o.name AS parent_name,
        BIN_TO_UUID(o.public_id) AS parent_public_id
    FROM organization_firewall_rules ofr
    JOIN organizations o ON ofr.organization_id = o.id
    JOIN user_orgs uo ON o.id = uo.organization_id
    WHERE ofr.status != 'deleted'

    UNION ALL

    SELECT
        pfr.id, BIN_TO_UUID(pfr.public_id) AS public_id, pfr.name, pfr.status, pfr.created_at, pfr.updated_at, pfr.rule_type, pfr.cidr,
        'project' AS parent_type,
        p.name AS parent_name,
        BIN_TO_UUID(p.public_id) AS parent_public_id
    FROM project_firewall_rules pfr
    JOIN projects p ON pfr.project_id = p.id
    LEFT JOIN project_members pm ON p.id = pm.project_id AND pm.account_id = sqlc.arg(account_id) AND pm.status = 'active'
    LEFT JOIN user_orgs uo ON p.organization_id = uo.organization_id
    WHERE pfr.status != 'deleted'
    AND (pm.id IS NOT NULL OR uo.organization_id IS NOT NULL)

    UNION ALL

    SELECT
        sfr.id, BIN_TO_UUID(sfr.public_id) AS public_id, sfr.name, sfr.status, sfr.created_at, sfr.updated_at, sfr.rule_type, sfr.cidr,
        'site' AS parent_type,
        s.name AS parent_name,
        BIN_TO_UUID(s.public_id) AS parent_public_id
    FROM site_firewall_rules sfr
    JOIN sites s ON sfr.site_id = s.id
    JOIN projects p ON s.project_id = p.id
    LEFT JOIN site_members sm ON s.id = sm.site_id AND sm.account_id = sqlc.arg(account_id) AND sm.status = 'active'
    LEFT JOIN project_members pm ON p.id = pm.project_id AND pm.account_id = sqlc.arg(account_id) AND pm.status = 'active'
    LEFT JOIN user_orgs uo ON p.organization_id = uo.organization_id
    WHERE sfr.status != 'deleted'
    AND (sm.id IS NOT NULL OR pm.id IS NOT NULL OR uo.organization_id IS NOT NULL)
) AS all_rules
ORDER BY created_at DESC
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

-- name: ListUserOrganizations :many
WITH RECURSIVE user_orgs AS (
    SELECT organization_id FROM organization_members WHERE organization_members.account_id = sqlc.arg(account_id) AND status = 'active'
    UNION DISTINCT
    SELECT r.target_organization_id
    FROM relationships r
    INNER JOIN user_orgs uo ON r.source_organization_id = uo.organization_id
    WHERE r.status = 'approved'
)
SELECT DISTINCT o.id, BIN_TO_UUID(o.public_id) AS public_id, o.name,
       COALESCE(om.role, 'read') AS role
FROM organizations o
JOIN user_orgs uo ON o.id = uo.organization_id
LEFT JOIN organization_members om ON o.id = om.organization_id AND om.account_id = sqlc.arg(account_id)
ORDER BY o.created_at DESC
LIMIT ? OFFSET ?;
