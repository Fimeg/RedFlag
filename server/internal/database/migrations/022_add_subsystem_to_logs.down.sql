-- Migration: Rollback subsystem column addition
-- Purpose: Remove subsystem column and associated indexes

-- Drop indexes
DROP INDEX IF EXISTS idx_logs_agent_subsystem;
DROP INDEX IF EXISTS idx_logs_subsystem;

-- Drop check constraint
ALTER TABLE update_logs
DROP CONSTRAINT IF EXISTS chk_update_logs_subsystem;

-- Remove comment
COMMENT ON COLUMN update_logs.subsystem IS NULL;

-- Drop subsystem column
ALTER TABLE update_logs
DROP COLUMN IF EXISTS subsystem;
