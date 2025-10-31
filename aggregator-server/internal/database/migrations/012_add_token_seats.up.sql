-- Add seat tracking to registration tokens for multi-use support
-- This allows tokens to be used multiple times up to a configured limit

-- Add seats columns
ALTER TABLE registration_tokens
    ADD COLUMN max_seats INT NOT NULL DEFAULT 1,
    ADD COLUMN seats_used INT NOT NULL DEFAULT 0;

-- Backfill existing tokens
-- Tokens with status='used' should have seats_used=1, max_seats=1
UPDATE registration_tokens
SET seats_used = 1,
    max_seats = 1
WHERE status = 'used';

-- Active/expired/revoked tokens get max_seats=1, seats_used=0
UPDATE registration_tokens
SET seats_used = 0,
    max_seats = 1
WHERE status IN ('active', 'expired', 'revoked');

-- Add constraint to ensure seats_used doesn't exceed max_seats
ALTER TABLE registration_tokens
    ADD CONSTRAINT chk_seats_used_within_max
    CHECK (seats_used <= max_seats);

-- Add constraint to ensure positive seat values
ALTER TABLE registration_tokens
    ADD CONSTRAINT chk_seats_positive
    CHECK (max_seats > 0 AND seats_used >= 0);

-- Create table to track all agents that used a token (for audit trail)
CREATE TABLE IF NOT EXISTS registration_token_usage (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_id UUID NOT NULL REFERENCES registration_tokens(id) ON DELETE CASCADE,
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    used_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(token_id, agent_id)
);

CREATE INDEX idx_token_usage_token_id ON registration_token_usage(token_id);
CREATE INDEX idx_token_usage_agent_id ON registration_token_usage(agent_id);

-- Backfill token usage table from existing used_by_agent_id
INSERT INTO registration_token_usage (token_id, agent_id, used_at)
SELECT id, used_by_agent_id, used_at
FROM registration_tokens
WHERE used_by_agent_id IS NOT NULL
ON CONFLICT (token_id, agent_id) DO NOTHING;

-- Update is_registration_token_valid function to check seats
CREATE OR REPLACE FUNCTION is_registration_token_valid(token_input VARCHAR)
RETURNS BOOLEAN AS $$
DECLARE
    token_valid BOOLEAN;
BEGIN
    SELECT (status = 'active' AND expires_at > NOW() AND seats_used < max_seats) INTO token_valid
    FROM registration_tokens
    WHERE token = token_input;

    RETURN COALESCE(token_valid, FALSE);
END;
$$ LANGUAGE plpgsql;

-- Update mark_registration_token_used function to increment seats
DROP FUNCTION IF EXISTS mark_registration_token_used(VARCHAR, UUID);
CREATE FUNCTION mark_registration_token_used(token_input VARCHAR, agent_id_param UUID)
RETURNS BOOLEAN AS $$
DECLARE
    rows_updated INTEGER;  -- Fixed: Changed from BOOLEAN to INTEGER to match ROW_COUNT type
    token_id_val UUID;
    new_seats_used INT;
    token_max_seats INT;
BEGIN
    -- Get token ID and current seat info
    SELECT id, seats_used + 1, max_seats INTO token_id_val, new_seats_used, token_max_seats
    FROM registration_tokens
    WHERE token = token_input
      AND status = 'active'
      AND expires_at > NOW()
      AND seats_used < max_seats;

    -- If no token found or already full, return false
    IF token_id_val IS NULL THEN
        RETURN FALSE;
    END IF;

    -- Increment seats_used
    UPDATE registration_tokens
    SET seats_used = new_seats_used,
        used_at = CASE
            WHEN used_at IS NULL THEN NOW()  -- First use
            ELSE used_at  -- Keep original first use time
        END,
        -- Only mark as 'used' if all seats are now taken
        status = CASE
            WHEN new_seats_used >= token_max_seats THEN 'used'
            ELSE 'active'
        END
    WHERE token = token_input
      AND status = 'active';

    GET DIAGNOSTICS rows_updated = ROW_COUNT;

    -- Record this usage in the audit table
    IF rows_updated > 0 THEN
        INSERT INTO registration_token_usage (token_id, agent_id, used_at)
        VALUES (token_id_val, agent_id_param, NOW())
        ON CONFLICT (token_id, agent_id) DO NOTHING;
    END IF;

    RETURN rows_updated > 0;
END;
$$ LANGUAGE plpgsql;

-- Add comment for documentation
COMMENT ON COLUMN registration_tokens.max_seats IS 'Maximum number of agents that can register with this token';
COMMENT ON COLUMN registration_tokens.seats_used IS 'Number of agents that have registered with this token';
COMMENT ON TABLE registration_token_usage IS 'Audit trail of all agents registered with each token';
