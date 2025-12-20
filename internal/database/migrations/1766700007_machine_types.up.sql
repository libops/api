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
