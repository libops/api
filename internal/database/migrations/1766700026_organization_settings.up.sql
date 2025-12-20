CREATE TABLE IF NOT EXISTS organization_settings (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    organization_id BIGINT NOT NULL,

    -- Setting configuration
    setting_key VARCHAR(255) NOT NULL,
    setting_value TEXT NOT NULL,  -- Store as string, app handles conversion
    editable BOOLEAN DEFAULT TRUE,  -- If false, users cannot modify

    -- Metadata
    description VARCHAR(500) DEFAULT NULL,  -- Optional human-readable description

    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'active',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    UNIQUE KEY unique_org_setting_key (organization_id, setting_key),
    INDEX idx_organization (organization_id),
    INDEX idx_setting_key (setting_key),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
