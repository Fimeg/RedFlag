-- SEC-025: Fleet-join 2FA. Adds a TOTP seed hash to registration tokens so
-- that standalone hosts can require a second factor when joining a fleet.
-- The seed hash is nullable — only set for fleet-join tokens that require 2FA.

ALTER TABLE registration_tokens
    ADD COLUMN IF NOT EXISTS totp_seed_hash TEXT;

-- Index for fleet-join lookups (rare, but keeps the join path fast).
CREATE INDEX IF NOT EXISTS idx_registration_tokens_totp_seed_hash
    ON registration_tokens (totp_seed_hash)
    WHERE totp_seed_hash IS NOT NULL;
