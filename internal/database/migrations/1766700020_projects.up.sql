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
    os VARCHAR(255) DEFAULT 'cos-125-19216-104-74',
    disk_type VARCHAR(255) DEFAULT 'hyperdisk-balanced',

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
