-- name: GetAccount :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, email, `name`, github_username, vault_entity_id,
       auth_method, verified, verified_at, onboarding_completed, onboarding_session_id, created_at, updated_at
FROM accounts WHERE public_id = UUID_TO_BIN(sqlc.arg(public_id));


-- name: GetAccountByID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, email, `name`, github_username, vault_entity_id,
       auth_method, verified, verified_at, onboarding_completed, onboarding_session_id, created_at, updated_at
FROM accounts WHERE id = ?;


-- name: GetAccountByEmail :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, email, `name`, github_username, vault_entity_id,
       auth_method, verified, verified_at, failed_login_attempts, last_failed_login_at,
       onboarding_completed, onboarding_session_id, created_at, updated_at
FROM accounts WHERE email = ?;


-- name: GetAccountByVaultEntityID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, email, `name`, github_username, vault_entity_id,
       auth_method, verified, verified_at, onboarding_completed, onboarding_session_id, created_at, updated_at
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


-- name: ListAPIKeysByAccount :many
SELECT id, BIN_TO_UUID(public_id) AS public_id, account_id, `name`, description,
       COALESCE(scopes, '[]') as scopes,
       created_at, last_used_at, expires_at, active, created_by
FROM api_keys
WHERE account_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;


-- name: ListSshKeysByAccount :many
SELECT sk.id, BIN_TO_UUID(sk.public_id) AS public_id,
       BIN_TO_UUID(a.public_id) AS account_public_id,
       sk.public_key, sk.`name`, sk.fingerprint,
       sk.created_at, sk.updated_at
FROM ssh_keys sk
JOIN accounts a ON sk.account_id = a.id
WHERE a.public_id = UUID_TO_BIN(sqlc.arg(public_id))
ORDER BY sk.created_at DESC;


-- name: ListAccountProjects :many
SELECT p.id, BIN_TO_UUID(p.public_id) AS public_id, p.`name`, pm.`role`
FROM project_members pm
JOIN projects p ON pm.project_id = p.id
WHERE pm.account_id = ?
ORDER BY p.created_at DESC
LIMIT ? OFFSET ?;

-- =============================================================================
-- MACHINE TYPES
-- =============================================================================


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


-- name: ListAccountSshAccess :many
SELECT * FROM ssh_access
WHERE account_id = ?
ORDER BY created_at DESC
LIMIT ? OFFSET ?;


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


-- name: GetProjectMemberByAccountAndProject :one
SELECT * FROM project_members
WHERE account_id = ? AND project_id = ? AND status = 'active';


-- name: GetSiteMemberByAccountAndSite :one
SELECT * FROM site_members
WHERE account_id = ? AND site_id = ? AND status = 'active';


-- name: UpdateAccountOnboarding :exec
UPDATE accounts SET
  onboarding_completed = ?,
  onboarding_session_id = ?,
  updated_at = NOW()
WHERE id = ?;


-- name: GetOnboardingSessionByAccountID :one
SELECT id, BIN_TO_UUID(public_id) AS public_id, account_id, org_name,
       CASE WHEN organization_public_id IS NULL THEN NULL ELSE BIN_TO_UUID(organization_public_id) END AS organization_public_id,
       machine_type, machine_price_id, disk_size_gb,
       stripe_checkout_session_id, stripe_checkout_url, stripe_subscription_id, organization_id,
       project_name, gcp_country, gcp_region, site_name, github_repo_url, port, firewall_ip,
       current_step, completed, expires_at, created_at, updated_at
FROM onboarding_sessions WHERE account_id = ? AND completed = FALSE ORDER BY created_at DESC LIMIT 1;


