CREATE TABLE IF NOT EXISTS domains (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    site_id BIGINT NOT NULL,
    domain VARCHAR(255) NOT NULL,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    UNIQUE KEY unique_domain (domain),
    INDEX idx_environment (site_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
