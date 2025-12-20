CREATE TABLE IF NOT EXISTS site_settings (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    site_id BIGINT NOT NULL,

    -- Setting configuration
    setting_key VARCHAR(255) NOT NULL,
    setting_value TEXT NOT NULL,
    editable BOOLEAN DEFAULT TRUE,

    -- Metadata
    description VARCHAR(500) DEFAULT NULL,

    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'active',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    UNIQUE KEY unique_site_setting_key (site_id, setting_key),
    INDEX idx_site (site_id),
    INDEX idx_setting_key (setting_key),
    INDEX idx_status (status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
