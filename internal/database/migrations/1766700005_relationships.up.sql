CREATE TABLE IF NOT EXISTS relationships (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    source_organization_id BIGINT NOT NULL,
    target_organization_id BIGINT NOT NULL,
    relationship_type ENUM('access', 'merge') NOT NULL,
    status ENUM('pending', 'approved', 'rejected') NOT NULL DEFAULT 'pending',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    resolved_at TIMESTAMP NULL,
    resolved_by BIGINT NULL,  -- Account ID who approved/rejected

    INDEX idx_source_organization (source_organization_id),
    INDEX idx_target_organization (target_organization_id),
    INDEX idx_status (status),
    UNIQUE KEY unique_relationship (source_organization_id, target_organization_id, relationship_type)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
