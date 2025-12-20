CREATE TABLE IF NOT EXISTS event_queue (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    event_id VARCHAR(255) NOT NULL UNIQUE,
    event_type VARCHAR(255) NOT NULL,
    event_source VARCHAR(255) NOT NULL,
    event_subject VARCHAR(255),
    event_data BLOB NOT NULL,
    content_type VARCHAR(100) NOT NULL DEFAULT 'application/protobuf',

    -- Resource IDs for event collapsing
    organization_id BIGINT NULL,
    project_id BIGINT NULL,
    site_id BIGINT NULL,

    -- Queue processing status
    status ENUM('pending', 'processing', 'sent', 'dead_letter', 'executed', 'collapsed') NOT NULL DEFAULT 'pending',
    retry_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    processing_by VARCHAR(255) NULL,
    processing_at TIMESTAMP NULL,

    -- Reconciliation tracking
    -- If status='collapsed', the run_id it was collapsed into
    collapsed_into_run_id VARCHAR(255) NULL,
    -- If status='executed', the run_id that was created
    created_run_id VARCHAR(255) NULL,

    -- Timestamps
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    last_retry_at TIMESTAMP NULL,
    sent_at TIMESTAMP NULL,
    processed_at TIMESTAMP NULL,

    INDEX idx_status_created (status, created_at),
    INDEX idx_event_type (event_type),
    INDEX idx_created_at (created_at),
    INDEX idx_processing (status, processing_by, processing_at),
    INDEX idx_collapsed_into (collapsed_into_run_id),
    INDEX idx_created_run (created_run_id),
    INDEX idx_processed_at (processed_at),
    INDEX idx_organization_id (organization_id),
    INDEX idx_project_id (project_id),
    INDEX idx_site_id (site_id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
