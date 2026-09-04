-- Rollback migration 023a: Command Deduplication Schema

DROP INDEX IF EXISTS idx_agent_pending_subsystem;
ALTER TABLE agent_commands DROP COLUMN IF EXISTS idempotency_key;
DROP INDEX IF EXISTS idx_agent_commands_idempotency_key;
