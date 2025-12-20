-- Results table for detailed per-module/per-site tracking
CREATE TABLE IF NOT EXISTS reconciliation_results (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    run_id VARCHAR(255) NOT NULL,

    result_type ENUM('terraform_module', 'vm_reconciliation') NOT NULL,
    module_type VARCHAR(50) NULL COMMENT 'For terraform: organization, project, site',
    site_id BIGINT NULL COMMENT 'For VM reconciliation: specific site ID',
    resource_id BIGINT NULL COMMENT 'For terraform: resource being provisioned',

    status ENUM('success', 'failed') NOT NULL,
    output TEXT NULL COMMENT 'Terraform output or reconciliation logs',
    error_message TEXT NULL,

    started_at TIMESTAMP NOT NULL,
    completed_at TIMESTAMP NOT NULL,

    INDEX idx_run_id (run_id),
    INDEX idx_result_type (result_type),
    INDEX idx_site_id (site_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
