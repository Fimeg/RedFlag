-- SEC-025: Reverse — drop TOTP seed hash from registration tokens.
DROP INDEX IF EXISTS idx_registration_tokens_totp_seed_hash;
ALTER TABLE registration_tokens DROP COLUMN IF EXISTS totp_seed_hash;
