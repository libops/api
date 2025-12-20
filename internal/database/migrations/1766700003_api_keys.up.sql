CREATE TABLE IF NOT EXISTS api_keys (
    id BIGINT AUTO_INCREMENT PRIMARY KEY,

    -- UUID for this API key (stored as path in Vault)
    public_id BINARY(16) NOT NULL UNIQUE,

    -- Account that owns this key
    account_id BIGINT NOT NULL,

    -- Human-readable name for the key
    name VARCHAR(255) NOT NULL,

    -- Optional description
    description TEXT,

    -- OAuth scope strings that restrict what this API key can access
    -- Empty/null scopes means no restrictions - authorization based on user's membership/roles
    scopes JSON DEFAULT NULL,

    -- When the key was created
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,

    -- When the key was last used for API access
    last_used_at TIMESTAMP NULL,

    -- When the key expires (NULL = never)
    expires_at TIMESTAMP NULL,

    -- Whether the key is active
    active BOOLEAN NOT NULL DEFAULT TRUE,

    -- Who created this key
    created_by BIGINT NULL,

    INDEX idx_account (account_id),
    INDEX idx_public_id (public_id),
    INDEX idx_active (active),
    INDEX idx_expires (expires_at)

) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
