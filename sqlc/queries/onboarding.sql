-- name: CreateOnboardingSession :execresult
INSERT INTO onboarding_sessions (
  public_id, account_id, org_name, machine_type, machine_price_id, disk_size_gb,
  stripe_checkout_session_id, stripe_subscription_id, organization_id,
  project_name, gcp_country, gcp_region, site_name, github_repo_url, port, firewall_ip,
  current_step, completed, expires_at, created_at, updated_at
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW());


-- name: GetOnboardingSession :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, account_id, org_name,
       CASE WHEN organization_public_id IS NULL THEN NULL ELSE BIN_TO_UUID(organization_public_id) END AS organization_public_id,
       machine_type, machine_price_id, disk_size_gb,
       stripe_checkout_session_id, stripe_checkout_url, stripe_subscription_id, organization_id,
       project_name, gcp_country, gcp_region, site_name, github_repo_url, port, firewall_ip,
       current_step, completed, expires_at, created_at, updated_at
FROM onboarding_sessions WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: UpdateOnboardingSession :exec
UPDATE onboarding_sessions SET
  org_name = ?,
  organization_public_id = IF(sqlc.arg(org_uuid) = '', NULL, UUID_TO_BIN(sqlc.arg(org_uuid))),
  machine_type = ?,
  machine_price_id = ?,
  disk_size_gb = ?,
  stripe_checkout_session_id = ?,
  stripe_checkout_url = ?,
  stripe_subscription_id = ?,
  organization_id = ?,
  project_name = ?,
  gcp_country = ?,
  gcp_region = ?,
  site_name = ?,
  github_repo_url = ?,
  port = ?,
  firewall_ip = ?,
  current_step = ?,
  completed = ?,
  updated_at = NOW()
WHERE id = ?;


-- name: DeleteExpiredOnboardingSessions :exec
DELETE FROM onboarding_sessions WHERE expires_at < NOW() AND completed = FALSE;

-- =============================================================================
-- STRIPE SUBSCRIPTIONS
-- =============================================================================


