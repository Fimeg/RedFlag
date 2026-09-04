-- Down for migration 033

DROP INDEX IF EXISTS idx_agent_commands_status_received_at;

-- Any rows currently in 'received' must be folded back to 'sent' before the
-- CHECK constraint can be tightened, otherwise the ALTER will fail.
UPDATE agent_commands SET status = 'sent' WHERE status = 'received';

ALTER TABLE agent_commands DROP CONSTRAINT IF EXISTS agent_commands_status_check;
ALTER TABLE agent_commands ADD CONSTRAINT agent_commands_status_check
  CHECK (status IN ('pending', 'sent', 'running', 'completed', 'failed', 'timed_out', 'cancelled', 'archived_failed'));

ALTER TABLE agent_commands DROP COLUMN IF EXISTS received_at;
