DROP INDEX IF EXISTS idx_refresh_tokens_family;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS consumed_at;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS superseded_by;
ALTER TABLE refresh_tokens DROP COLUMN IF EXISTS family_id;
