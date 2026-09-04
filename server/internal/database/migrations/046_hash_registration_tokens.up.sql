-- SEC-001: Hash registration tokens at rest.
-- Registration tokens are now stored as SHA-256 hashes, matching the refresh
-- token model. The plaintext column is dropped; all queries use token_hash.
--
-- Idempotent (ETHOS #4 / F-B1-15): every step guards for re-run. The backfill is
-- wrapped in a column-existence check because the plaintext `token` column is
-- dropped at the end — a second run must not reference a column that's gone.

-- Add the hash column
ALTER TABLE registration_tokens ADD COLUMN IF NOT EXISTS token_hash VARCHAR(64);

-- Backfill: SHA-256 hash every existing plaintext token, but only while the
-- plaintext column still exists (skipped on a re-run after it's been dropped).
DO $$
BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_name = 'registration_tokens' AND column_name = 'token'
    ) THEN
        UPDATE registration_tokens
        SET token_hash = encode(sha256(token::bytea), 'hex')
        WHERE token_hash IS NULL;
    END IF;
END $$;

-- Now enforce NOT NULL + uniqueness (SET NOT NULL is a no-op when already set)
ALTER TABLE registration_tokens ALTER COLUMN token_hash SET NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_registration_tokens_hash ON registration_tokens(token_hash);

-- Drop the plaintext column and its old unique constraint
ALTER TABLE registration_tokens DROP COLUMN IF EXISTS token;

-- Rebuild is_registration_token_valid to use token_hash
DROP FUNCTION IF EXISTS is_registration_token_valid(VARCHAR);
CREATE FUNCTION is_registration_token_valid(token_hash_input VARCHAR)
RETURNS BOOLEAN AS $$
DECLARE
    token_valid BOOLEAN;
BEGIN
    SELECT (status = 'active' AND expires_at > NOW() AND seats_used < max_seats) INTO token_valid
    FROM registration_tokens
    WHERE token_hash = token_hash_input;

    RETURN COALESCE(token_valid, FALSE);
END;
$$ LANGUAGE plpgsql;

-- Rebuild mark_registration_token_used to use token_hash
DROP FUNCTION IF EXISTS mark_registration_token_used(VARCHAR, UUID);
CREATE FUNCTION mark_registration_token_used(token_hash_input VARCHAR, agent_id_param UUID)
RETURNS BOOLEAN AS $$
DECLARE
    rows_updated INTEGER;
    token_id_val UUID;
    new_seats_used INT;
    token_max_seats INT;
BEGIN
    SELECT id, seats_used + 1, max_seats INTO token_id_val, new_seats_used, token_max_seats
    FROM registration_tokens
    WHERE token_hash = token_hash_input
      AND status = 'active'
      AND expires_at > NOW()
      AND seats_used < max_seats;

    IF token_id_val IS NULL THEN
        RETURN FALSE;
    END IF;

    UPDATE registration_tokens
    SET seats_used = new_seats_used,
        used_at = CASE
            WHEN used_at IS NULL THEN NOW()
            ELSE used_at
        END,
        status = CASE
            WHEN new_seats_used >= token_max_seats THEN 'used'
            ELSE 'active'
        END
    WHERE id = token_id_val
      AND status = 'active';

    GET DIAGNOSTICS rows_updated = ROW_COUNT;

    IF rows_updated > 0 THEN
        INSERT INTO registration_token_usage (token_id, agent_id, used_at)
        VALUES (token_id_val, agent_id_param, NOW())
        ON CONFLICT (token_id, agent_id) DO NOTHING;
    END IF;

    RETURN rows_updated > 0;
END;
$$ LANGUAGE plpgsql;
