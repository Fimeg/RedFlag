-- Migration 026 rollback: Remove expires_at column from agent_commands
DROP INDEX IF EXISTS idx_agent_commands_expires_at;
ALTER TABLE agent_commands DROP COLUMN IF EXISTS expires_at;
