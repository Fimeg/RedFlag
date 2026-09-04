-- Ensure proper UNIQUE constraint on machine_id for hardware fingerprint binding
-- This prevents config file copying attacks by validating hardware identity
-- NOTE: Migration 016 already added the machine_id column, this ensures proper unique constraint

-- Drop the old non-unique index if it exists
DROP INDEX IF EXISTS idx_agents_machine_id;

-- Create unique index to prevent duplicate machine IDs (allows multiple NULLs)
-- Note: CONCURRENTLY removed to allow transaction-based migration
CREATE UNIQUE INDEX IF NOT EXISTS idx_agents_machine_id_unique ON agents(machine_id) WHERE machine_id IS NOT NULL;

-- Add comment for documentation
COMMENT ON COLUMN agents.machine_id IS 'SHA-256 hash of hardware fingerprint (prevents agent impersonation via config copying)';
