-- Reverses migration 034.

ALTER TABLE security_settings
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS created_at;
