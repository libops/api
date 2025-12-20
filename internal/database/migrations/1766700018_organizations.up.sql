CREATE TABLE IF NOT EXISTS organizations (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    name VARCHAR(255) NOT NULL,

    -- GCP configuration (provided at creation)
    gcp_org_id VARCHAR(255) NOT NULL,
    gcp_billing_account VARCHAR(255) NOT NULL,
    -- Parent resource: folders/{folder_id} or organizations/{org_id}
    gcp_parent VARCHAR(255) NOT NULL,
    -- Organization's preferred Google Cloud location and region
    location ENUM('unspecified', 'asia', 'au', 'ca', 'de', 'eu', 'in', 'it', 'us') DEFAULT 'unspecified',
    region VARCHAR(255) DEFAULT '',

    -- GCP resources (populated after terraform creates them)
    gcp_folder_id VARCHAR(255) UNIQUE,

    -- Provisioning status
    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    -- GCP resources (populated after terraform creates them)
    gcp_project_id VARCHAR(63) UNIQUE,
    gcp_project_number VARCHAR(255),

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    INDEX idx_gcp_folder (gcp_folder_id),
    INDEX idx_public_id (public_id),
    INDEX idx_status (status),
    INDEX idx_location (location)
 ) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
