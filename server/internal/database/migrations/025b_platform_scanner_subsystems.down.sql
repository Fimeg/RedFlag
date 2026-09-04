-- Migration: 025_platform_scanner_subsystems (down)
-- Purpose: Remove platform-specific package scanner subsystems and restore original trigger
-- Version: 0.1.29
-- Date: 2025-12-23

-- Remove platform-specific subsystems for Linux agents
DELETE FROM agent_subsystems
WHERE subsystem IN ('apt', 'dnf', 'windows', 'winget');

-- Restore original trigger from migration 015
DROP TRIGGER IF EXISTS trigger_create_default_subsystems ON agents;

CREATE OR REPLACE FUNCTION create_default_subsystems()
RETURNS TRIGGER AS $$
BEGIN
    -- Insert default subsystems for new agent (legacy pattern)
    INSERT INTO agent_subsystems (agent_id, subsystem, enabled, interval_minutes, auto_run)
    VALUES
        (NEW.id, 'storage', true, 15, false),
        (NEW.id, 'system', true, 30, false),
        (NEW.id, 'docker', false, 15, false);

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trigger_create_default_subsystems
    AFTER INSERT ON agents
    FOR EACH ROW
    EXECUTE FUNCTION create_default_subsystems();

RAISE NOTICE 'Migration 025_platform_scanner_subsystems down completed: Original trigger restored';
