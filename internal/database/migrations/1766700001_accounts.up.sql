CREATE TABLE IF NOT EXISTS accounts (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,
    public_id BINARY(16) NOT NULL UNIQUE,
    email VARCHAR(255) NOT NULL UNIQUE,
    name VARCHAR(255),
    github_username VARCHAR(255),
    vault_entity_id VARCHAR(255) UNIQUE DEFAULT NULL,

    -- Authentication method: 'google', 'userpass', 'gcloud' (service account), or future OIDC providers
    auth_method ENUM('google', 'userpass', 'gcloud', 'github', 'okta', 'azure_ad') NOT NULL DEFAULT 'userpass',

    -- Email verification status
    verified BOOLEAN NOT NULL DEFAULT FALSE,
    verified_at TIMESTAMP NULL,

    failed_login_attempts INT NOT NULL DEFAULT 0,
    last_failed_login_at TIMESTAMP NULL,

    -- Onboarding tracking
    onboarding_completed BOOLEAN NOT NULL DEFAULT FALSE,
    onboarding_session_id VARCHAR(255) NULL,

    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,

    INDEX idx_github (github_username),
    INDEX idx_vault_entity (vault_entity_id),
    INDEX idx_verified (verified),
    INDEX idx_auth_method (auth_method),
    INDEX idx_onboarding_completed (onboarding_completed)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
