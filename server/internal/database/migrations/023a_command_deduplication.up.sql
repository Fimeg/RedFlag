-- Migration 023a: Command Deduplication Schema
-- Prevents multiple pending scan commands per subsystem per agent

-- Add unique constraint to enforce single pending command per subsystem
CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_pending_subsystem
ON agent_commands(agent_id, command_type, status)
WHERE status = 'pending';

-- Add idempotency key support for retry scenarios
ALTER TABLE agent_commands ADD COLUMN IF NOT EXISTS idempotency_key VARCHAR(64) UNIQUE NULL;
CREATE INDEX IF NOT EXISTS idx_agent_commands_idempotency_key ON agent_commands(idempotency_key);

-- Comments for documentation
COMMENT ON TABLE agent_commands IS 'Commands sent to agents for execution';
COMMENT ON COLUMN agent_commands.idempotency_key IS
  'Prevents duplicate command creation from retry logic. Based on agent_id + subsystem + timestamp window.';
