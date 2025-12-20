CREATE TABLE IF NOT EXISTS sites (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,

    project_id BIGINT NOT NULL,
    name VARCHAR(255) NOT NULL,

    -- GitHub configuration
    github_repository VARCHAR(255) NOT NULL,
    -- GitHub reference: heads/{branch_name}, tags/{tag_name}, or "release"
    github_ref VARCHAR(255) NOT NULL DEFAULT 'heads/main',
    github_team_id VARCHAR(255),
    compose_path VARCHAR(255) DEFAULT '/mnt/disks/data/compose',
    compose_file VARCHAR(255) DEFAULT 'docker-compose.yml',

    -- Application configuration
    port INT DEFAULT 80,
    application_type VARCHAR(255) DEFAULT 'generic',

    -- Docker compose commands
    up_cmd JSON DEFAULT ('["docker compose up --remove-orphans -d"]'),
    init_cmd JSON DEFAULT ('[]'),
    rollout_cmd JSON DEFAULT ('["docker compose pull", "docker compose up --remove-orphans -d"]'),
    overlay_volumes JSON DEFAULT ('[]'),

    -- GCP deployment configuration
    os VARCHAR(255) DEFAULT 'cos-125-19216-104-74',
    is_production BOOLEAN DEFAULT FALSE,

    -- GCP resources (populated after terraform creates them)
    gcp_external_ip VARCHAR(255),

    -- VM check-in timestamp (updated by VM controller to indicate liveness)
    checkin_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- State materialization (control plane reconciliation)
    target_state_hash VARCHAR(64) NULL COMMENT 'SHA-256 hash of materialized state (ssh-keys + secrets + firewall)',
    last_state_materialized_at TIMESTAMP NULL COMMENT 'Last time state was materialized to GCS',

    -- Provisioning status
    status ENUM('unspecified', 'active', 'provisioning', 'failed', 'suspended', 'deleted') DEFAULT 'unspecified',

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    created_by BIGINT NULL,
    updated_by BIGINT NULL,

    UNIQUE KEY unique_site_env (project_id, name),
    INDEX idx_site (project_id),
    INDEX idx_repository (github_repository),
    INDEX idx_status (status),
    INDEX idx_port (port),
    INDEX idx_state_hash (id, target_state_hash)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
