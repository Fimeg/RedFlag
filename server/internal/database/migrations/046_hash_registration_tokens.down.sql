-- Reverse SEC-001: restore plaintext token column.
-- WARNING: hash is one-way — rolled-back tokens will be empty strings.

ALTER TABLE registration_tokens ADD COLUMN token VARCHAR(64);
UPDATE registration_tokens SET token = '';
ALTER TABLE registration_tokens ALTER COLUMN token SET NOT NULL;
ALTER TABLE registration_tokens ADD CONSTRAINT registration_tokens_token_key UNIQUE (token);

ALTER TABLE registration_tokens DROP COLUMN token_hash;

DROP FUNCTION IF EXISTS mark_registration_token_used(VARCHAR, UUID);
CREATE FUNCTION mark_registration_token_used(token_input VARCHAR, agent_id_param UUID)
RETURNS BOOLEAN AS $$
DECLARE
    rows_updated INTEGER;
    token_id_val UUID;
    new_seats_used INT;
    token_max_seats INT;
BEGIN
    SELECT id, seats_used + 1, max_seats INTO token_id_val, new_seats_used, token_max_seats
    FROM registration_tokens
    WHERE token = token_input
      AND status = 'active'
      AND expires_at > NOW()
      AND seats_used < max_seats;

    IF token_id_val IS NULL THEN
        RETURN FALSE;
    END IF;

    UPDATE registration_tokens
    SET seats_used = new_seats_used,
        used_at = CASE WHEN used_at IS NULL THEN NOW() ELSE used_at END,
        status = CASE WHEN new_seats_used >= token_max_seats THEN 'used' ELSE 'active' END
    WHERE token = token_input AND status = 'active';

    GET DIAGNOSTICS rows_updated = ROW_COUNT;

    IF rows_updated > 0 THEN
        INSERT INTO registration_token_usage (token_id, agent_id, used_at)
        VALUES (token_id_val, agent_id_param, NOW())
        ON CONFLICT (token_id, agent_id) DO NOTHING;
    END IF;

    RETURN rows_updated > 0;
END;
$$ LANGUAGE plpgsql;

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
