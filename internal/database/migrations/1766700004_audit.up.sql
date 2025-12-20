CREATE TABLE IF NOT EXISTS audit (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    account_id BIGINT NOT NULL,
    entity_id BIGINT NOT NULL,
    entity_type ENUM('accounts', 'organizations', 'projects', 'sites', 'ssh_keys', 'api_keys') NOT NULL,
    event_name VARCHAR(255) NOT NULL,
    event_data BLOB NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    INDEX idx_audit_account (account_id),
    INDEX idx_audit_entity (entity_id, entity_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
