-- 008_create_refresh_tokens_table.sql
-- Create refresh tokens table for secure token renewal

CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    token_hash VARCHAR(64) NOT NULL,  -- SHA-256 hash of the refresh token
    expires_at TIMESTAMP NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    last_used_at TIMESTAMP,
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT unique_token_hash UNIQUE(token_hash)
);

-- Index for fast agent lookup
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_agent_id ON refresh_tokens(agent_id);

-- Index for expiration cleanup
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_expires_at ON refresh_tokens(expires_at);

-- Index for token validation
CREATE INDEX IF NOT EXISTS idx_refresh_tokens_hash_not_revoked
    ON refresh_tokens(token_hash) WHERE NOT revoked;

COMMENT ON TABLE refresh_tokens IS 'Stores long-lived refresh tokens for agent token renewal without re-registration';
COMMENT ON COLUMN refresh_tokens.token_hash IS 'SHA-256 hash of the refresh token for secure storage';
COMMENT ON COLUMN refresh_tokens.expires_at IS 'Refresh token expiration (default: 90 days from creation)';
COMMENT ON COLUMN refresh_tokens.last_used_at IS 'Timestamp of last successful token renewal';
COMMENT ON COLUMN refresh_tokens.revoked IS 'Flag to revoke token before expiration';
