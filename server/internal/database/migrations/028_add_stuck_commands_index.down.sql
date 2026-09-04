-- Migration 028 rollback
DROP INDEX IF EXISTS idx_agent_commands_status_sent_at;
