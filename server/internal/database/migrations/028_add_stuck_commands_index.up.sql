-- Migration 028: Add index for GetStuckCommands query (F-B1-5 fix)
-- Covers the (status, sent_at) pattern used by the timeout service

CREATE INDEX IF NOT EXISTS idx_agent_commands_status_sent_at
ON agent_commands(status, sent_at)
WHERE status IN ('pending', 'sent');
