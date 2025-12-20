-- name: GetMachineType :one
SELECT id, machine_type, display_name, vcpu, memory_gib, stripe_price_id, monthly_price_cents, active, created_at, updated_at
FROM machine_types
WHERE machine_type = ? AND active = TRUE;


-- name: GetMachineTypeByStripePriceID :one
SELECT id, machine_type, display_name, vcpu, memory_gib, stripe_price_id, monthly_price_cents, active, created_at, updated_at
FROM machine_types
WHERE stripe_price_id = ? AND active = TRUE;


-- name: ListMachineTypes :many
SELECT id, machine_type, display_name, vcpu, memory_gib, stripe_price_id, monthly_price_cents, active, created_at, updated_at
FROM machine_types
WHERE active = TRUE
ORDER BY vcpu ASC, memory_gib ASC;


-- name: ListAllMachineTypes :many
SELECT id, machine_type, display_name, vcpu, memory_gib, stripe_price_id, monthly_price_cents, active, created_at, updated_at
FROM machine_types
ORDER BY vcpu ASC, memory_gib ASC;


-- name: CreateMachineType :exec
INSERT INTO machine_types (machine_type, display_name, vcpu, memory_gib, stripe_price_id, monthly_price_cents, active)
VALUES (?, ?, ?, ?, ?, ?, ?);


-- name: UpdateMachineType :exec
UPDATE machine_types
SET display_name = ?, vcpu = ?, memory_gib = ?, stripe_price_id = ?, monthly_price_cents = ?, active = ?, updated_at = NOW()
WHERE machine_type = ?;


-- name: GetStorageConfig :one
SELECT id, config_key, stripe_price_id, price_per_gb_cents, min_size_gb, max_size_gb, active, created_at, updated_at
FROM storage_config
WHERE config_key = 'disk_storage' AND active = TRUE;

-- =============================================================================
-- PROJECTS
-- =============================================================================


-- name: GetOnboardingSessionByStripeCheckoutID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, account_id, org_name, BIN_TO_UUID(organization_public_id) AS organization_public_id,
       machine_type, machine_price_id, disk_size_gb,
       stripe_checkout_session_id, stripe_checkout_url, stripe_subscription_id, organization_id,
       project_name, gcp_country, gcp_region, site_name, github_repo_url, port, firewall_ip,
       current_step, completed, expires_at, created_at, updated_at
FROM onboarding_sessions WHERE stripe_checkout_session_id = ?;


-- name: CreateStripeSubscription :execresult
INSERT INTO stripe_subscriptions (
  public_id, organization_id, stripe_subscription_id, stripe_customer_id, stripe_checkout_session_id,
  status, current_period_start, current_period_end, trial_start, trial_end,
  cancel_at_period_end, canceled_at, machine_type, disk_size_gb, created_at, updated_at
) VALUES (UUID_TO_BIN(UUID_V7()), ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW());


-- name: GetStripeSubscription :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, stripe_subscription_id, stripe_customer_id, stripe_checkout_session_id,
       status, current_period_start, current_period_end, trial_start, trial_end,
       cancel_at_period_end, canceled_at, machine_type, disk_size_gb, created_at, updated_at
FROM stripe_subscriptions WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: GetStripeSubscriptionByStripeID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, organization_id, stripe_subscription_id, stripe_customer_id, stripe_checkout_session_id,
       status, current_period_start, current_period_end, trial_start, trial_end,
       cancel_at_period_end, canceled_at, machine_type, disk_size_gb, created_at, updated_at
FROM stripe_subscriptions WHERE stripe_subscription_id = ?;


-- name: UpdateStripeSubscription :exec
UPDATE stripe_subscriptions SET
  status = ?,
  current_period_start = ?,
  current_period_end = ?,
  trial_start = ?,
  trial_end = ?,
  cancel_at_period_end = ?,
  canceled_at = ?,
  machine_type = ?,
  disk_size_gb = ?,
  updated_at = NOW()
WHERE stripe_subscription_id = ?;


-- name: DeleteStripeSubscription :exec
DELETE FROM stripe_subscriptions WHERE stripe_subscription_id = ?;

-- =============================================================================
-- VM RECONCILIATION ADMIN API
-- =============================================================================


