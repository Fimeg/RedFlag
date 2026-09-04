-- Migration 033: Add 'received' state to agent command lifecycle
-- Distinguishes "server sent" (status=sent) from "agent confirmed receipt" (status=received).
-- Closes the gap where stuck-command re-issuance fires blindly because the server has
-- no way to tell a lost-in-flight command from a slow-to-execute one.
-- Doctrine: TODO-full-command-lifecycle.md §1.

-- Drop existing CHECK constraint and re-add with 'received' included
ALTER TABLE agent_commands DROP CONSTRAINT IF EXISTS agent_commands_status_check;
ALTER TABLE agent_commands ADD CONSTRAINT agent_commands_status_check
  CHECK (status IN ('pending', 'sent', 'received', 'running', 'completed', 'failed', 'timed_out', 'cancelled', 'archived_failed'));

-- Timestamp the sent->received transition
ALTER TABLE agent_commands
    ADD COLUMN IF NOT EXISTS received_at TIMESTAMPTZ;

-- Index for the TimeoutService 'received' reconciler (longer threshold than 'sent')
CREATE INDEX IF NOT EXISTS idx_agent_commands_status_received_at
    ON agent_commands(status, received_at)
    WHERE status = 'received';
