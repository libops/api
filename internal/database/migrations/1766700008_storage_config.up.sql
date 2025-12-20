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
