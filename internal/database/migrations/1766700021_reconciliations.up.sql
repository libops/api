-- Platform service database schema
-- This adds platform-specific tables to the main API database

-- Reconciliations table (Kubernetes-style naming)
-- Handles both terraform infrastructure runs and VM configuration reconciliation
CREATE TABLE IF NOT EXISTS reconciliations (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    run_id VARCHAR(255) NOT NULL UNIQUE,

    -- Resource identifiers (references to main API tables)
    organization_id BIGINT NULL,
    project_id BIGINT NULL,
    site_id BIGINT NULL,

    -- Run type: terraform infrastructure or VM config reconciliation
    run_type ENUM('terraform', 'reconciliation') NOT NULL,
    reconciliation_type ENUM('ssh_keys', 'secrets', 'firewall', 'general') NULL,

    -- Terraform: modules to run; Reconciliation: target site IDs
    modules JSON NULL COMMENT 'For terraform: ["organization", "project", "site"]',
    target_site_ids JSON NULL COMMENT 'For reconciliation: array of site IDs',

    -- Event aggregation
    event_ids JSON NOT NULL COMMENT 'Array of event IDs that triggered this run',
    first_event_at TIMESTAMP NOT NULL,
    last_event_at TIMESTAMP NOT NULL,

    -- Execution tracking
    status ENUM('pending', 'triggered', 'running', 'completed', 'failed') DEFAULT 'pending',
    error_message TEXT NULL,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    triggered_at TIMESTAMP NULL,
    started_at TIMESTAMP NULL,
    completed_at TIMESTAMP NULL,

    INDEX idx_status (status),
    INDEX idx_run_type (run_type),
    INDEX idx_reconciliation_type (reconciliation_type),
    INDEX idx_org_id (organization_id),
    INDEX idx_project_id (project_id),
    INDEX idx_site_id (site_id),
    INDEX idx_created_at (created_at)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
