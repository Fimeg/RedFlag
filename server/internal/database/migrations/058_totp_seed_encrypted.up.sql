-- SEC-025 rework: store the fleet-join TOTP seed encrypted, not hashed.
-- A hash-only server can never verify a time code without the host
-- disclosing the seed in the join request itself — which reduced the
-- "2FA" to a second cleartext shared secret. The seed now crosses the
-- wire once, at token creation over the admin-authenticated channel,
-- is stored AES-256-GCM encrypted (same key as token_encrypted), and
-- the join request carries only the 6-digit code.

ALTER TABLE registration_tokens
    DROP COLUMN IF EXISTS totp_seed_hash;

DROP INDEX IF EXISTS idx_registration_tokens_totp_seed_hash;

ALTER TABLE registration_tokens
    ADD COLUMN IF NOT EXISTS totp_seed_encrypted BYTEA;
