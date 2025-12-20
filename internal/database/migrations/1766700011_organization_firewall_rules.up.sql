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
