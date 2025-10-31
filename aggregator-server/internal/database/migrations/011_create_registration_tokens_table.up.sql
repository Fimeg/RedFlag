-- Registration tokens for secure agent enrollment
-- Tokens are one-time use and have configurable expiration

CREATE TABLE registration_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token VARCHAR(64) UNIQUE NOT NULL,           -- One-time use token
    label VARCHAR(255),                          -- Optional label for token identification
    expires_at TIMESTAMP NOT NULL,               -- Token expiration time
    created_at TIMESTAMP DEFAULT NOW(),          -- When token was created
    used_at TIMESTAMP NULL,                      -- When token was used (NULL if unused)
    used_by_agent_id UUID NULL,                  -- Which agent used this token (foreign key)
    revoked BOOLEAN DEFAULT FALSE,               -- Manual revocation
    revoked_at TIMESTAMP NULL,                   -- When token was revoked
    revoked_reason VARCHAR(255) NULL,            -- Reason for revocation

    -- Token status tracking
    status VARCHAR(20) DEFAULT 'active'
        CHECK (status IN ('active', 'used', 'expired', 'revoked')),

    -- Additional metadata
    created_by VARCHAR(100) DEFAULT 'setup_wizard', -- Who created the token
    metadata JSONB DEFAULT '{}'::jsonb              -- Additional token metadata
);

-- Indexes for performance
CREATE INDEX idx_registration_tokens_token ON registration_tokens(token);
CREATE INDEX idx_registration_tokens_expires_at ON registration_tokens(expires_at);
CREATE INDEX idx_registration_tokens_status ON registration_tokens(status);
CREATE INDEX idx_registration_tokens_used_by_agent ON registration_tokens(used_by_agent_id) WHERE used_by_agent_id IS NOT NULL;

-- Foreign key constraint for used_by_agent_id
ALTER TABLE registration_tokens
    ADD CONSTRAINT fk_registration_tokens_agent
    FOREIGN KEY (used_by_agent_id) REFERENCES agents(id) ON DELETE SET NULL;

-- Function to clean up expired tokens (called by periodic cleanup job)
CREATE OR REPLACE FUNCTION cleanup_expired_registration_tokens()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER;
BEGIN
    UPDATE registration_tokens
    SET status = 'expired',
        used_at = NOW()
    WHERE status = 'active'
      AND expires_at < NOW()
      AND used_at IS NULL;

    GET DIAGNOSTICS deleted_count = ROW_COUNT;
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Function to check if a token is valid
CREATE OR REPLACE FUNCTION is_registration_token_valid(token_input VARCHAR)
RETURNS BOOLEAN AS $$
DECLARE
    token_valid BOOLEAN;
BEGIN
    SELECT (status = 'active' AND expires_at > NOW()) INTO token_valid
    FROM registration_tokens
    WHERE token = token_input;

    RETURN COALESCE(token_valid, FALSE);
END;
$$ LANGUAGE plpgsql;

-- Function to mark token as used
CREATE OR REPLACE function mark_registration_token_used(token_input VARCHAR, agent_id UUID)
RETURNS BOOLEAN AS $$
DECLARE
    updated BOOLEAN;
BEGIN
    UPDATE registration_tokens
    SET status = 'used',
        used_at = NOW(),
        used_by_agent_id = agent_id
    WHERE token = token_input
      AND status = 'active'
      AND expires_at > NOW();

    GET DIAGNOSTICS updated = ROW_COUNT;
    RETURN updated > 0;
END;
$$ LANGUAGE plpgsql;