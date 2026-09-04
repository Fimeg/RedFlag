DROP INDEX IF EXISTS idx_agent_commands_key_id;
ALTER TABLE agent_commands DROP COLUMN IF EXISTS key_id;
ALTER TABLE agent_commands DROP COLUMN IF EXISTS signed_at;
