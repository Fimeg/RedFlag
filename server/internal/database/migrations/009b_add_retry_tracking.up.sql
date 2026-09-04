-- Add retry tracking to agent_commands table
-- This allows us to track command retry chains and display retry indicators in the UI

-- Add retried_from_id column to link retries to their original commands
ALTER TABLE agent_commands
ADD COLUMN IF NOT EXISTS retried_from_id UUID REFERENCES agent_commands(id) ON DELETE SET NULL;

-- Add index for efficient retry chain lookups
CREATE INDEX IF NOT EXISTS idx_commands_retried_from ON agent_commands(retried_from_id) WHERE retried_from_id IS NOT NULL;
