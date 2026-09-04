-- Reverse SETTINGS-001 reversible token encryption column.
ALTER TABLE registration_tokens
    DROP COLUMN IF EXISTS token_encrypted;
