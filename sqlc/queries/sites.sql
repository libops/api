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


-- name: GetSite :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, `name`, github_repository, github_ref, github_team_id, compose_path, compose_file, port, application_type, up_cmd, init_cmd, rollout_cmd, overlay_volumes, os, is_production, gcp_external_ip, `status`,
       created_at, updated_at, created_by, updated_by
FROM sites WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: GetSiteByID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, `name`, github_repository, github_ref, github_team_id, compose_path, compose_file, port, application_type, up_cmd, init_cmd, rollout_cmd, overlay_volumes, os, is_production, gcp_external_ip, `status`,
       created_at, updated_at, created_by, updated_by
FROM sites WHERE id = ?;


-- name: GetSiteByShortUUID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, `name`, github_repository, github_ref, github_team_id, compose_path, compose_file, port, application_type, up_cmd, init_cmd, rollout_cmd, overlay_volumes, os, is_production, gcp_external_ip, `status`,
       created_at, updated_at, created_by, updated_by
FROM sites WHERE HEX(public_id) LIKE CONCAT(UPPER(sqlc.arg(short_uuid)), '%') LIMIT 1;


-- name: CreateSite :exec
INSERT INTO sites (
  public_id, project_id, `name`, github_repository, github_ref, github_team_id, compose_path, compose_file, port, application_type, up_cmd, init_cmd, rollout_cmd, overlay_volumes, os, is_production, gcp_external_ip, `status`, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), ?, ?);


-- name: UpdateSite :exec
UPDATE sites SET
  `name` = ?,
  github_repository = ?,
  github_ref = ?,
  github_team_id = ?,
  compose_path = ?,
  compose_file = ?,
  port = ?,
  application_type = ?,
  up_cmd = ?,
  init_cmd = ?,
  rollout_cmd = ?,
  overlay_volumes = ?,
  os = ?,
  is_production = ?,
  gcp_external_ip = ?,
  `status` = ?,
  updated_at = NOW(),
  updated_by = ?
WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: DeleteSite :exec
DELETE FROM sites WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: ListSites :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, project_id, name, github_repository, github_ref, github_team_id, compose_path, compose_file, port, application_type, up_cmd, init_cmd, rollout_cmd, gcp_external_ip, status, created_at, updated_at, created_by, updated_by
FROM sites
ORDER BY created_at DESC
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
SELECT DISTINCT s.id, BIN_TO_UUID(s.public_id) AS public_id, s.project_id, BIN_TO_UUID(p.public_id) AS project_public_id, BIN_TO_UUID(o.public_id) AS organization_public_id, s.name, s.github_repository, s.github_ref, s.github_team_id, s.compose_path, s.compose_file, s.port, s.application_type, s.up_cmd, s.init_cmd, s.rollout_cmd, s.overlay_volumes, s.os, s.is_production, s.gcp_external_ip, s.status, s.created_at, s.updated_at, s.created_by, s.updated_by
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


-- name: GetSiteFirewallRuleByPublicID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, site_id, rule_type, cidr, name, status, created_at, updated_at, created_by, updated_by
FROM site_firewall_rules WHERE public_id = UUID_TO_BIN(?);


-- name: CreateSiteFirewallRule :exec
INSERT INTO site_firewall_rules (
  public_id, site_id, name, rule_type, cidr, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?);


-- name: DeleteSiteFirewallRule :exec
DELETE FROM site_firewall_rules WHERE id = ?;


-- name: DeleteSiteFirewallRuleByPublicID :exec
UPDATE site_firewall_rules SET status = 'deleted', updated_at = CURRENT_TIMESTAMP WHERE public_id = UUID_TO_BIN(?);


-- name: ListSiteFirewallRules :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, site_id, rule_type, cidr, name, status, created_at, updated_at, created_by, updated_by
FROM site_firewall_rules
WHERE site_id = ? AND status != 'deleted'
ORDER BY created_at DESC;

-- =============================================================================
-- Ssh ACCESS
-- =============================================================================


-- name: ListSiteSshAccess :many
SELECT sa.id, sa.account_id, sa.site_id, sa.created_at, sa.updated_at,
       a.email, a.`name`, a.github_username
FROM ssh_access sa
JOIN accounts a ON sa.account_id = a.id
WHERE sa.site_id = ?
ORDER BY sa.created_at DESC
LIMIT ? OFFSET ?;


-- =============================================================================
-- RELATIONSHIPS
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


-- name: GetSiteSSHKeysForVM :many
-- Fetches all SSH keys that should be provisioned to a site VM
-- Includes keys from site members, project members, org members, and relationship members
SELECT DISTINCT sk.public_key, sk.name, sk.fingerprint, a.email, a.name as user_name, a.github_username, BIN_TO_UUID(a.public_id) AS account_public_id
FROM ssh_keys sk
JOIN accounts a ON sk.account_id = a.id
WHERE sk.account_id IN (
    -- Site members (owner/developer with active status)
    SELECT account_id FROM site_members
    WHERE site_id = ? AND role IN ('owner', 'developer') AND status = 'active'

    UNION

    -- Project members (owner/developer with active status)
    SELECT pm.account_id FROM project_members pm
    JOIN sites s ON s.project_id = pm.project_id
    WHERE s.id = ? AND pm.role IN ('owner', 'developer') AND pm.status = 'active'

    UNION

    -- Org members (owner/developer with active status)
    SELECT om.account_id FROM organization_members om
    JOIN projects p ON p.organization_id = om.organization_id
    JOIN sites s ON s.project_id = p.id
    WHERE s.id = ? AND om.role IN ('owner', 'developer') AND om.status = 'active'

    UNION

    -- Members via org relationships (approved relationships)
    SELECT om.account_id FROM organization_members om
    JOIN relationships r ON r.source_organization_id = om.organization_id
    JOIN projects p ON p.organization_id = r.target_organization_id
    JOIN sites s ON s.project_id = p.id
    WHERE s.id = ? AND r.status = 'approved'
      AND om.role IN ('owner', 'developer') AND om.status = 'active'
);


-- name: GetSiteSecretsForVM :many
-- Fetches all secrets that should be provisioned to a site VM
-- Includes secrets from site, project, and org levels
SELECT DISTINCT ss.name as `key`, ss.vault_path as value
FROM site_secrets ss
WHERE ss.site_id = ?
UNION
SELECT DISTINCT ps.name as `key`, ps.vault_path as value
FROM project_secrets ps
JOIN sites s ON s.project_id = ps.project_id
WHERE s.id = ?
UNION
SELECT DISTINCT os.name as `key`, os.vault_path as value
FROM organization_secrets os
JOIN projects p ON p.organization_id = os.organization_id
JOIN sites st ON st.project_id = p.id
WHERE st.id = ?;


-- name: GetSiteFirewallForVM :many
-- Fetches all firewall rules that should be applied to a site VM
-- Includes rules from site, project, and org levels
SELECT DISTINCT sf.rule_type, sf.cidr, sf.name
FROM site_firewall_rules sf
WHERE sf.site_id = ? AND sf.status = 'active'
UNION
SELECT DISTINCT pf.rule_type, pf.cidr, pf.name
FROM project_firewall_rules pf
JOIN sites s ON s.project_id = pf.project_id
WHERE s.id = ? AND pf.status = 'active'
UNION
SELECT DISTINCT orgf.rule_type, orgf.cidr, orgf.name
FROM organization_firewall_rules orgf
JOIN projects p ON p.organization_id = orgf.organization_id
JOIN sites st ON st.project_id = p.id
WHERE st.id = ? AND orgf.status = 'active';


-- name: UpdateSiteCheckIn :exec
-- Updates the site's check-in timestamp (called by VM controller)
UPDATE sites SET checkin_at = NOW() WHERE id = ?;


