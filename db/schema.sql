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

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_github (github_username),
    INDEX idx_vault_entity (vault_entity_id),
    INDEX idx_verified (verified),
    INDEX idx_auth_method (auth_method)
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

    -- GitHub configuration (provided at creation)
    -- Either github_repository (existing repo) or github_repository_template (new from template)
    github_repository VARCHAR(255),
    github_repository_template VARCHAR(255),
    github_branch VARCHAR(255) DEFAULT 'main',
    compose_path VARCHAR(255) DEFAULT '',

    -- Cloud configuration (provided at creation)
    gcp_region VARCHAR(255) DEFAULT 'us-central1',
    gcp_zone VARCHAR(255) DEFAULT 'us-central1-c',
    machine_type VARCHAR(255) DEFAULT 'e2-medium',
    disk_size_gb INT DEFAULT 20,
    compose_file VARCHAR(255) DEFAULT 'docker-compose.yml',
    application_type VARCHAR(255) DEFAULT 'generic',

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

    -- GitHub resources (populated after terraform creates them)
    github_team_id VARCHAR(255),

    -- Flag to identify if this is the organization's libops project (for vault, etc.)
    organization_project BOOLEAN DEFAULT FALSE,

    -- Auto-create sites for new branches
    create_branch_sites BOOLEAN DEFAULT FALSE,

    -- Docker compose commands
    up_cmd VARCHAR(1024) DEFAULT 'docker compose up --remove-orphans -d',
    init_cmd VARCHAR(1024) DEFAULT '',
    rollout_cmd JSON DEFAULT ('["docker compose pull", "docker compose up --remove-orphans -d"]'),

    -- Provisioning status
    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    INDEX idx_project (gcp_project_id),
    INDEX idx_organization (organization_id),
    INDEX idx_repository (github_repository),
    INDEX idx_status (status),
    INDEX idx_organization_project (organization_project)
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

    -- GitHub reference: heads/{branch_name}, tags/{tag_name}, or "release"
    github_ref VARCHAR(255) NOT NULL DEFAULT 'heads/main',

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
    INDEX idx_status (status)
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
