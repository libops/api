CREATE TABLE IF NOT EXISTS deployments (
  id VARCHAR(255) PRIMARY KEY,
  site_id VARCHAR(255) NOT NULL,
  status ENUM('pending', 'in_progress', 'success', 'failed') NOT NULL DEFAULT 'pending',
  github_run_id VARCHAR(255),
  github_run_url TEXT,
  started_at BIGINT NOT NULL,
  completed_at BIGINT,
  error_message TEXT,
  created_at BIGINT NOT NULL,
  INDEX idx_deployments_site (site_id),
  INDEX idx_deployments_status (status)
);
