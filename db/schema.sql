CREATE TABLE IF NOT EXISTS organizations (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,

    -- GCP configuration (provided at creation)
    gcp_org_id VARCHAR(255) NOT NULL,
    gcp_billing_account VARCHAR(255) NOT NULL,
    -- Parent resource: folders/{folder_id} or organizations/{org_id}
    gcp_parent VARCHAR(255) NOT NULL,
    -- Organization's preferred Google Cloud location and region
    location ENUM('unspecified', 'asia', 'au', 'ca', 'de', 'eu', 'in', 'it', 'us') DEFAULT 'unspecified',
    region VARCHAR(255) DEFAULT '',

    -- GCP resources (populated after terraform creates them)
    gcp_folder_id VARCHAR(255) UNIQUE,

    -- Provisioning status
    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    -- GCP resources (populated after terraform creates them)
    gcp_project_id VARCHAR(63) UNIQUE,
    gcp_project_number VARCHAR(255),

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    INDEX idx_gcp_folder (gcp_folder_id),
    INDEX idx_public_id (public_id),
    INDEX idx_status (status),
    INDEX idx_location (location)
 ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS accounts (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255),
    github_username VARCHAR(255),
    vault_entity_id VARCHAR(255) UNIQUE DEFAULT NULL,

    -- Authentication method: 'google', 'userpass', 'gcloud' (service account), or future OIDC providers
    auth_method ENUM('google', 'userpass', 'gcloud', 'github', 'okta', 'azure_ad') NOT NULL DEFAULT 'userpass',

    -- Email verification status
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    verified_at TIMESTAMP NULL,

    failed_login_attempts INT NOT NULL DEFAULT 0,
    last_failed_login_at TIMESTAMP NULL,

    -- Onboarding tracking
    onboarding_completed BOOLEAN NOT NULL DEFAULT FALSE,
    onboarding_session_id VARCHAR(255) NULL,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_github (github_username),
    INDEX idx_vault_entity (vault_entity_id),
    INDEX idx_verified (verified),
    INDEX idx_auth_method (auth_method),
    INDEX idx_onboarding_completed (onboarding_completed)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS organization_members (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    organization_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    role ENUM('owner', 'developer', 'read') NOT NULL DEFAULT 'read',
    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    UNIQUE KEY unique_organization_account (organization_id, account_id),
    INDEX idx_public_id (public_id),
    INDEX idx_organization (organization_id),
    INDEX idx_user (account_id),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ssh_keys (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,

    account_id BIGINT NOT NULL,
    public_key TEXT NOT NULL,
    name VARCHAR(255),
    fingerprint VARCHAR(255),

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_user (account_id),
    INDEX idx_fingerprint (fingerprint)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS api_keys (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,

    -- UUID for this API key (stored as path in Vault)
    public_id BINARY(16) NOT NULL UNIQUE,

    -- Account that owns this key
    account_id BIGINT NOT NULL,

    -- Human-readable name for the key
    name VARCHAR(255) NOT NULL,

    -- Optional description
    description TEXT,

    -- OAuth scope strings that restrict what this API key can access
    -- Empty/null scopes means no restrictions - authorization based on user's membership/roles
    scopes JSON DEFAULT NULL,

    -- When the key was created
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- When the key was last used for API access
    last_used_at TIMESTAMP NULL,

    -- When the key expires (NULL = never)
    expires_at TIMESTAMP NULL,

    -- Whether the key is active
    active BOOLEAN NOT NULL DEFAULT TRUE,

    -- Who created this key
    created_by BIGINT NULL,

    INDEX idx_account (account_id),
    INDEX idx_public_id (public_id),
    INDEX idx_active (active),
    INDEX idx_expires (expires_at)

) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS projects (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,

    organization_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,

    -- Cloud configuration (provided at creation)
    gcp_region VARCHAR(255) DEFAULT 'us-central1',
    gcp_zone VARCHAR(255) DEFAULT 'us-central1-c',
    machine_type VARCHAR(255) DEFAULT 'e2-medium',
    disk_size_gb INT DEFAULT 20,

    -- Stripe subscription item ID for per-project billing (machine subscription item)
    stripe_subscription_item_id VARCHAR(255) NULL,

    -- Promotion strategy
    promote_strategy ENUM('unspecified', 'github_tag', 'github_release') DEFAULT 'unspecified',

    -- Monitoring configuration
    monitoring_enabled BOOLEAN DEFAULT FALSE,
    monitoring_log_level VARCHAR(50) DEFAULT 'INFO',
    monitoring_metrics_enabled BOOLEAN DEFAULT FALSE,
    monitoring_health_check_path VARCHAR(255) DEFAULT '/',

    -- GCP resources (populated after terraform creates them)
    gcp_project_id VARCHAR(63) UNIQUE,
    gcp_project_number VARCHAR(255),

    -- Flag to identify if this is the organization's libops project (for vault, etc.)
    organization_project BOOLEAN DEFAULT FALSE,

    -- Auto-create sites for new branches
    create_branch_sites BOOLEAN DEFAULT FALSE,

    -- Provisioning status
    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    INDEX idx_project (gcp_project_id),
    INDEX idx_organization (organization_id),
    INDEX idx_status (status),
    INDEX idx_organization_project (organization_project),
    INDEX idx_stripe_subscription_item (stripe_subscription_item_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


CREATE TABLE IF NOT EXISTS project_members (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    project_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    role ENUM('owner', 'developer', 'read') NOT NULL DEFAULT 'read',
    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    UNIQUE KEY unique_project_account (project_id, account_id),
    INDEX idx_public_id (public_id),
    INDEX idx_organization (project_id),
    INDEX idx_user (account_id),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;


CREATE TABLE IF NOT EXISTS sites (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,

    project_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,

    -- GitHub configuration
    github_repository VARCHAR(255) NOT NULL,
    -- GitHub reference: heads/{branch_name}, tags/{tag_name}, or "release"
    github_ref VARCHAR(255) NOT NULL DEFAULT 'heads/main',
    github_team_id VARCHAR(255),
    compose_path VARCHAR(255) DEFAULT '/mnt/disks/data/compose',
    compose_file VARCHAR(255) DEFAULT 'docker-compose.yml',

    -- Application configuration
    port INT DEFAULT 80,
    application_type VARCHAR(255) DEFAULT 'generic',

    -- Docker compose commands
    up_cmd JSON DEFAULT ('["docker compose up --remove-orphans -d"]'),
    init_cmd JSON DEFAULT ('[]'),
    rollout_cmd JSON DEFAULT ('["docker compose pull", "docker compose up --remove-orphans -d"]'),

    -- GCP resources (populated after terraform creates them)
    gcp_external_ip VARCHAR(255),

    -- Provisioning status
    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    UNIQUE KEY unique_site_env (project_id, name),
    INDEX idx_site (project_id),
    INDEX idx_repository (github_repository),
    INDEX idx_status (status),
    INDEX idx_port (port)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_members (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    site_id BIGINT NOT NULL,
    account_id BIGINT NOT NULL,
    role ENUM('owner', 'developer', 'read') NOT NULL DEFAULT 'read',
    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    UNIQUE KEY unique_site_account (site_id, account_id),
    INDEX idx_public_id (public_id),
    INDEX idx_site (site_id),
    INDEX idx_user (account_id),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS domains (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    site_id BIGINT NOT NULL,
    domain VARCHAR(255) NOT NULL,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE KEY unique_domain (domain),
    INDEX idx_environment (site_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS organization_firewall_rules (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    organization_id BIGINT NULL,

    rule_type ENUM('https_allowed', 'ssh_allowed', 'blocked') NOT NULL,
    cidr VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    INDEX idx_organization (organization_id),
    INDEX idx_rule_type (rule_type),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS project_firewall_rules (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    project_id BIGINT NULL,

    rule_type ENUM('https_allowed', 'ssh_allowed', 'blocked') NOT NULL,
    cidr VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    INDEX idx_project (project_id),
    INDEX idx_rule_type (rule_type),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_firewall_rules (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    site_id BIGINT NULL,

    rule_type ENUM('https_allowed', 'ssh_allowed', 'blocked') NOT NULL,
    cidr VARCHAR(255) NOT NULL,
    name VARCHAR(255) NOT NULL,
    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    INDEX idx_environment (site_id),
    INDEX idx_rule_type (rule_type),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS ssh_access (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    account_id BIGINT NOT NULL,
    site_id BIGINT NOT NULL,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    UNIQUE KEY unique_user_environment (account_id, site_id),
    INDEX idx_user (account_id),
    INDEX idx_environment (site_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS relationships (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    source_organization_id BIGINT NOT NULL,
    target_organization_id BIGINT NOT NULL,
    relationship_type ENUM('access', 'merge') NOT NULL,
    status ENUM('pending', 'approved', 'rejected') NOT NULL DEFAULT 'pending',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP NULL,
    resolved_by BIGINT NULL,  -- Account ID who approved/rejected

    INDEX idx_source_organization (source_organization_id),
    INDEX idx_target_organization (target_organization_id),
    INDEX idx_status (status),
    UNIQUE KEY unique_relationship (source_organization_id, target_organization_id, relationship_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS deployments (
  deployment_id VARCHAR(255) PRIMARY KEY,
  site_id VARCHAR(255) NOT NULL,
  status ENUM('pending', 'in_progress', 'success', 'failed') NOT NULL DEFAULT 'pending',
  github_run_id VARCHAR(255),
  github_run_url TEXT,
  started_at BIGINT NOT NULL,
  completed_at BIGINT,
  error_message TEXT,
  created_at BIGINT NOT NULL,
  INDEX idx_deployments_site (site_id),
  INDEX idx_deployments_status (status)
);

CREATE TABLE IF NOT EXISTS event_queue (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    event_id VARCHAR(255) NOT NULL UNIQUE,
    event_type VARCHAR(255) NOT NULL,
    event_source VARCHAR(255) NOT NULL,
    event_subject VARCHAR(255),
    event_data BLOB NOT NULL,
    content_type VARCHAR(100) NOT NULL DEFAULT 'application/protobuf',
    status ENUM('pending', 'processing', 'sent', 'dead_letter') NOT NULL DEFAULT 'pending',
    retry_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    processing_by VARCHAR(255) NULL,
    processing_at TIMESTAMP NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_retry_at TIMESTAMP NULL,
    sent_at TIMESTAMP NULL,
    INDEX idx_status_created (status, created_at),
    INDEX idx_event_type (event_type),
    INDEX idx_created_at (created_at),
    INDEX idx_processing (status, processing_by, processing_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS email_verification_tokens (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    email VARCHAR(255) NOT NULL,
    token VARCHAR(255) NOT NULL,
    password_hash TEXT NOT NULL,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NOT NULL,

    UNIQUE KEY unique_email_verification (email),
    INDEX idx_token (token),
    INDEX idx_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS audit (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    account_id BIGINT NOT NULL,
    entity_id BIGINT NOT NULL,
    entity_type ENUM('accounts', 'organizations', 'projects', 'sites', 'ssh_keys', 'api_keys') NOT NULL,
    event_name VARCHAR(255) NOT NULL,
    event_data BLOB NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_audit_account (account_id),
    INDEX idx_audit_entity (entity_id, entity_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS organization_secrets (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,

    organization_id BIGINT NOT NULL,

    -- Secret identifier (becomes Vault path component)
    name VARCHAR(255) NOT NULL,

    -- Vault path where this secret is stored
    -- Format: secret-global/{name}
    vault_path VARCHAR(512) NOT NULL,

    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    -- Unix timestamps for audit trail
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,

    -- Account IDs (links to accounts table, NO foreign keys)
    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    UNIQUE KEY unique_organization_secret_name (organization_id, name),
    INDEX idx_organization (organization_id),
    INDEX idx_status (status),
    INDEX idx_created_by (created_by),
    INDEX idx_updated_by (updated_by)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS project_secrets (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,

    project_id BIGINT NOT NULL,

    -- Secret identifier (becomes Vault path component)
    name VARCHAR(255) NOT NULL,

    -- Vault path where this secret is stored
    -- Format: secret-project/{project_id}/{name}
    vault_path VARCHAR(512) NOT NULL,

    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    -- Unix timestamps for audit trail
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,

    -- Account IDs (links to accounts table, NO foreign keys)
    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    UNIQUE KEY unique_project_secret_name (project_id, name),
    INDEX idx_project (project_id),
    INDEX idx_status (status),
    INDEX idx_created_by (created_by),
    INDEX idx_updated_by (updated_by)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS site_secrets (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,

    site_id BIGINT NOT NULL,

    -- Secret identifier (becomes Vault path component)
    name VARCHAR(255) NOT NULL,

    -- Vault path where this secret is stored
    -- Format: secret-site/{site_id}/{name}
    vault_path VARCHAR(512) NOT NULL,

    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    -- Unix timestamps for audit trail
    created_at BIGINT NOT NULL,
    updated_at BIGINT NOT NULL,

    -- Account IDs (links to accounts table, NO foreign keys)
    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    UNIQUE KEY unique_site_secret_name (site_id, name),
    INDEX idx_site (site_id),
    INDEX idx_status (status),
    INDEX idx_created_by (created_by),
    INDEX idx_updated_by (updated_by)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS stripe_subscriptions (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    organization_id BIGINT NOT NULL,
    stripe_subscription_id VARCHAR(255) NOT NULL UNIQUE,
    stripe_customer_id VARCHAR(255) NOT NULL,
    stripe_checkout_session_id VARCHAR(255),

    -- Subscription details
    status ENUM('incomplete', 'incomplete_expired', 'trialing', 'active', 'past_due', 'canceled', 'unpaid') NOT NULL DEFAULT 'incomplete',
    current_period_start TIMESTAMP NULL,
    current_period_end TIMESTAMP NULL,
    trial_start TIMESTAMP NULL,
    trial_end TIMESTAMP NULL,
    cancel_at_period_end BOOLEAN DEFAULT FALSE,
    canceled_at TIMESTAMP NULL,

    -- Machine and disk configuration
    machine_type VARCHAR(50),
    disk_size_gb INT,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_organization_id (organization_id),
    INDEX idx_stripe_subscription_id (stripe_subscription_id),
    INDEX idx_stripe_customer_id (stripe_customer_id),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS onboarding_sessions (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    account_id BIGINT NOT NULL,

    -- Step 1: Organization
    org_name VARCHAR(255),

    -- Step 2: Machine & Disk
    machine_type VARCHAR(50),
    machine_price_id VARCHAR(255),
    disk_size_gb INT,

    -- Step 3: Stripe
    stripe_checkout_session_id VARCHAR(255),
    stripe_checkout_url VARCHAR(500),
    stripe_subscription_id VARCHAR(255) NULL,
    organization_id BIGINT NULL,

    -- Step 4-7: Project and Site details
    project_name VARCHAR(255),
    gcp_country VARCHAR(50),
    gcp_region VARCHAR(50),
    site_name VARCHAR(255),
    github_repo_url VARCHAR(500),
    port INT DEFAULT 80,
    firewall_ip VARCHAR(50),

    -- Progress tracking
    current_step INT DEFAULT 1,
    completed BOOLEAN DEFAULT FALSE,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    expires_at TIMESTAMP NULL,

    INDEX idx_account_id (account_id),
    INDEX idx_stripe_checkout_session_id (stripe_checkout_session_id),
    INDEX idx_completed (completed),
    INDEX idx_expires_at (expires_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Create machine types table for managing machine configurations and Stripe pricing
CREATE TABLE IF NOT EXISTS machine_types (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,

    -- Machine configuration
    machine_type VARCHAR(255) NOT NULL UNIQUE COMMENT 'Machine type identifier (e.g., e2-medium, n4-standard-2)',
    display_name VARCHAR(255) NOT NULL COMMENT 'Human-readable display name',

    -- Resources
    vcpu INT NOT NULL COMMENT 'Number of vCPUs',
    memory_gib INT NOT NULL COMMENT 'Memory in GiB',

    -- Stripe pricing
    stripe_price_id VARCHAR(255) NOT NULL UNIQUE COMMENT 'Stripe price ID for this machine type',
    monthly_price_cents INT NOT NULL COMMENT 'Monthly price in cents',

    -- Status
    active BOOLEAN DEFAULT TRUE COMMENT 'Whether this machine type is available for new projects',

    -- Timestamps
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_machine_type (machine_type),
    INDEX idx_active (active),
    INDEX idx_stripe_price_id (stripe_price_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Create disk storage configuration table
CREATE TABLE IF NOT EXISTS storage_config (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,

    -- Configuration key (should always be 'disk_storage')
    config_key VARCHAR(50) NOT NULL UNIQUE,

    -- Stripe pricing
    stripe_price_id VARCHAR(255) NOT NULL COMMENT 'Stripe price ID for disk storage per GB',
    price_per_gb_cents INT NOT NULL COMMENT 'Price per GB per month in cents',

    -- Constraints
    min_size_gb INT NOT NULL DEFAULT 10 COMMENT 'Minimum disk size in GB',
    max_size_gb INT NOT NULL DEFAULT 2000 COMMENT 'Maximum disk size in GB',

    -- Status
    active BOOLEAN DEFAULT TRUE,

    -- Timestamps
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
