-- Add 'archived_failed' status to agent_commands status constraint
-- This allows archiving failed/timed_out commands to clean up the active list

-- Drop the existing constraint
ALTER TABLE agent_commands DROP CONSTRAINT IF EXISTS agent_commands_status_check;

-- Add the new constraint with 'archived_failed' included
ALTER TABLE agent_commands ADD CONSTRAINT agent_commands_status_check
  CHECK (status IN ('pending', 'sent', 'running', 'completed', 'failed', 'timed_out', 'cancelled', 'archived_failed'));
