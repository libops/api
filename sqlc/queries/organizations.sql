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


-- name: ListAllOrganizations :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, `name`, gcp_org_id, gcp_billing_account, gcp_parent, gcp_folder_id, `status`, gcp_project_id, gcp_project_number, created_at, updated_at, created_by, updated_by
FROM organizations
ORDER BY created_at DESC;


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


-- name: GetOrganizationMember :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, account_id, `role`, created_at, updated_at, created_by, updated_by
FROM organization_members WHERE organization_id = ? AND account_id = ?;


-- name: CreateOrganizationMember :exec
INSERT INTO organization_members (
  public_id, organization_id, account_id, `role`, status, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, ?, NOW(), NOW(), ?, ?);


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


-- name: GetProjectWithOrganization :one
SELECT
    p.id, BIN_TO_UUID(p.public_id) AS public_id, p.organization_id, p.name,
    p.gcp_region, p.gcp_zone, p.machine_type, p.disk_size_gb, p.os, p.disk_type, p.stripe_subscription_item_id,
    p.promote_strategy,
    p.monitoring_enabled, p.monitoring_log_level, p.monitoring_metrics_enabled, p.monitoring_health_check_path,
    p.gcp_project_id, p.gcp_project_number, p.create_branch_sites, p.status,
    p.created_at, p.updated_at, p.created_by, p.updated_by,
    c.gcp_billing_account
FROM projects p
JOIN organizations c ON p.organization_id = c.id
WHERE p.public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: GetOrganizationProjectByOrganizationID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, `name`,
       gcp_region, gcp_zone, machine_type, disk_size_gb, os, disk_type, stripe_subscription_item_id,
       promote_strategy,
       monitoring_enabled, monitoring_log_level, monitoring_metrics_enabled, monitoring_health_check_path,
       gcp_project_id, gcp_project_number, create_branch_sites, organization_project, `status`,
       created_at, updated_at, created_by, updated_by
FROM projects
WHERE organization_id = ? AND organization_project = TRUE
LIMIT 1;


-- name: ListOrganizationProjects :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, name, gcp_region, gcp_zone, machine_type, disk_size_gb, os, disk_type, stripe_subscription_item_id, promote_strategy, monitoring_enabled, monitoring_log_level, monitoring_metrics_enabled, monitoring_health_check_path, gcp_project_id, gcp_project_number, organization_project, create_branch_sites, status, created_at, updated_at, created_by, updated_by
FROM projects
WHERE organization_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;


-- name: GetOrganizationFirewallRuleByPublicID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, rule_type, cidr, name, status, created_at, updated_at, created_by, updated_by
FROM organization_firewall_rules WHERE public_id = UUID_TO_BIN(?);


-- name: CreateOrganizationFirewallRule :exec
INSERT INTO organization_firewall_rules (
  public_id, organization_id, name, rule_type, cidr, created_at, updated_at, created_by, updated_by
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, ?, ?);


-- name: DeleteOrganizationFirewallRule :exec
DELETE FROM organization_firewall_rules WHERE id = ?;


-- name: DeleteOrganizationFirewallRuleByPublicID :exec
UPDATE organization_firewall_rules SET status = 'deleted', updated_at = CURRENT_TIMESTAMP WHERE public_id = UUID_TO_BIN(?);


-- name: ListOrganizationFirewallRules :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, rule_type, cidr, name, status, created_at, updated_at, created_by, updated_by
FROM organization_firewall_rules
WHERE organization_id = ? AND status != 'deleted'
ORDER BY created_at DESC;

-- =============================================================================
-- PROJECT FIREWALL RULES
-- =============================================================================


-- name: ListOrganizationRelationships :many
SELECT r.id, BIN_TO_UUID(r.public_id) AS public_id, r.source_organization_id, r.target_organization_id,
       r.relationship_type, r.`status`, r.created_at, r.resolved_at, r.resolved_by
FROM relationships r
WHERE r.source_organization_id = ? OR r.target_organization_id = ?
ORDER BY r.created_at DESC;


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


-- name: GetOrganizationMemberByAccountAndOrganization :one
SELECT * FROM organization_members
WHERE account_id = ? AND organization_id = ? AND status = 'active';


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

-- =============================================================================
-- ONBOARDING
-- =============================================================================


-- name: GetStripeSubscriptionByOrganizationID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, stripe_subscription_id, stripe_customer_id, stripe_checkout_session_id,
       status, current_period_start, current_period_end, trial_start, trial_end,
       cancel_at_period_end, canceled_at, machine_type, disk_size_gb, created_at, updated_at
FROM stripe_subscriptions WHERE organization_id = ?;


-- name: UpdateOrganizationMemberStatus :exec
-- Updates organization member status (e.g., provisioning → active)
UPDATE organization_members
SET status = ?, updated_at = NOW()
WHERE public_id = UUID_TO_BIN(?);


-- name: CountUserOrganizations :one
SELECT COUNT(DISTINCT om.organization_id) as count
FROM organization_members om
WHERE om.account_id = ? AND om.status = 'active';


