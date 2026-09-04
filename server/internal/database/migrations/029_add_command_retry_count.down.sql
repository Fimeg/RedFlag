-- Migration 029 rollback
DROP INDEX IF EXISTS idx_agent_commands_retry_count;
ALTER TABLE agent_commands DROP COLUMN IF EXISTS retry_count;
