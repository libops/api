CREATE TABLE IF NOT EXISTS onboarding_sessions (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    account_id BIGINT NOT NULL,

    -- Step 1: Organization
    org_name VARCHAR(255),
    organization_public_id BINARY(16) NULL,

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
