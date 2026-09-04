-- Migration: 013_agent_subsystems (down)
-- Purpose: Rollback agent subsystems table
-- Version: 0.1.20
-- Date: 2025-11-01

-- Drop trigger and function
DROP TRIGGER IF EXISTS trigger_create_default_subsystems ON agents;
DROP FUNCTION IF EXISTS create_default_subsystems();

-- Drop indexes
DROP INDEX IF EXISTS idx_agent_subsystems_lookup;
DROP INDEX IF EXISTS idx_agent_subsystems_subsystem;
DROP INDEX IF EXISTS idx_agent_subsystems_next_run;
DROP INDEX IF EXISTS idx_agent_subsystems_agent;

-- Drop table
DROP TABLE IF EXISTS agent_subsystems;
