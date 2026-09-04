ALTER TABLE registration_tokens
    DROP COLUMN IF EXISTS totp_seed_encrypted;

ALTER TABLE registration_tokens
    ADD COLUMN IF NOT EXISTS totp_seed_hash TEXT;

CREATE INDEX IF NOT EXISTS idx_registration_tokens_totp_seed_hash
    ON registration_tokens (totp_seed_hash)
    WHERE totp_seed_hash IS NOT NULL;
