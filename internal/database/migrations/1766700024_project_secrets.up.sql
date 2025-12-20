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
