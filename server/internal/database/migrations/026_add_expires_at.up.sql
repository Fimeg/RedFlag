-- Migration 026: Add expires_at column to agent_commands (F-7 fix)
-- Enables TTL-based filtering of expired commands
-- ETHOS #4: Idempotent — safe to run multiple times

ALTER TABLE agent_commands
    ADD COLUMN IF NOT EXISTS expires_at TIMESTAMP;

CREATE INDEX IF NOT EXISTS idx_agent_commands_expires_at
    ON agent_commands(expires_at)
    WHERE expires_at IS NOT NULL;

-- Backfill existing pending commands with 24h expiry from creation time.
-- Uses 24h (not the live 4h default) as a conservative backfill to avoid
-- expiring in-flight commands that may have been created hours ago.
UPDATE agent_commands
    SET expires_at = created_at + INTERVAL '24 hours'
    WHERE expires_at IS NULL
      AND status = 'pending';
