-- SETTINGS-001: reversible at-rest encryption for registration tokens.
--
-- SEC-001 reduced tokens to a one-way SHA-256 (token_hash), which is correct for
-- install-time validation but destroyed the plaintext the operator UI needs to
-- rebuild the agent install one-liner. token_hash stays the validation/lookup
-- primitive; token_encrypted is the reversible companion (AES-256-GCM, nonce
-- prepended) decrypted only for live tokens. The key lives outside the database
-- (config secret rail), so this column alone is not recoverable from a DB dump.
--
-- No backfill: pre-existing tokens were one-way hashed and cannot be recovered.
-- They simply carry no plaintext; the operator issues a new token if needed.

ALTER TABLE registration_tokens
    ADD COLUMN IF NOT EXISTS token_encrypted BYTEA;
