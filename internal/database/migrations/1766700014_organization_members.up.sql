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
    INDEX idx_status (status),
    INDEX idx_org_role_status (organization_id, role, status)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
