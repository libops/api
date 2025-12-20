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
