-- Migration 029: Add retry_count to agent_commands (F-B2-10 fix)
-- Caps stuck command re-delivery at 5 attempts

ALTER TABLE agent_commands
    ADD COLUMN IF NOT EXISTS retry_count INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_agent_commands_retry_count
    ON agent_commands(retry_count)
    WHERE status IN ('pending', 'sent');
