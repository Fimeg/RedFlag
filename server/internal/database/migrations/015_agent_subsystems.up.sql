-- Migration: 013_agent_subsystems
-- Purpose: Add agent subsystems table for granular command scheduling and management
-- Version: 0.1.20
-- Date: 2025-11-01

-- Create agent_subsystems table for tracking individual subsystem configurations per agent
CREATE TABLE IF NOT EXISTS agent_subsystems (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    subsystem VARCHAR(50) NOT NULL,
    enabled BOOLEAN DEFAULT true,
    interval_minutes INTEGER DEFAULT 15,
    auto_run BOOLEAN DEFAULT false,
    last_run_at TIMESTAMP,
    next_run_at TIMESTAMP,
    created_at TIMESTAMP DEFAULT NOW(),
    updated_at TIMESTAMP DEFAULT NOW(),
    UNIQUE(agent_id, subsystem)
);

-- Create indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_agent_subsystems_agent ON agent_subsystems(agent_id);
CREATE INDEX IF NOT EXISTS idx_agent_subsystems_next_run ON agent_subsystems(next_run_at)
    WHERE enabled = true AND auto_run = true;
CREATE INDEX IF NOT EXISTS idx_agent_subsystems_subsystem ON agent_subsystems(subsystem);

-- Create a composite index for common queries (agent + subsystem)
CREATE INDEX IF NOT EXISTS idx_agent_subsystems_lookup ON agent_subsystems(agent_id, subsystem, enabled);

-- Default subsystems for existing agents
-- Only insert for agents that don't already have subsystems configured
INSERT INTO agent_subsystems (agent_id, subsystem, enabled, interval_minutes, auto_run)
SELECT id, 'updates', true, 15, false FROM agents
WHERE NOT EXISTS (
    SELECT 1 FROM agent_subsystems WHERE agent_subsystems.agent_id = agents.id AND subsystem = 'updates'
)
UNION ALL
SELECT id, 'storage', true, 15, false FROM agents
WHERE NOT EXISTS (
    SELECT 1 FROM agent_subsystems WHERE agent_subsystems.agent_id = agents.id AND subsystem = 'storage'
)
UNION ALL
SELECT id, 'system', true, 30, false FROM agents
WHERE NOT EXISTS (
    SELECT 1 FROM agent_subsystems WHERE agent_subsystems.agent_id = agents.id AND subsystem = 'system'
)
UNION ALL
SELECT id, 'docker', false, 15, false FROM agents
WHERE NOT EXISTS (
    SELECT 1 FROM agent_subsystems WHERE agent_subsystems.agent_id = agents.id AND subsystem = 'docker'
);

-- Create trigger to automatically insert default subsystems for new agents
CREATE OR REPLACE FUNCTION create_default_subsystems()
RETURNS TRIGGER AS $$
BEGIN
    -- Insert default subsystems for new agent
    INSERT INTO agent_subsystems (agent_id, subsystem, enabled, interval_minutes, auto_run)
    VALUES
        (NEW.id, 'updates', true, 15, false),
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

-- Add comment for documentation
COMMENT ON TABLE agent_subsystems IS 'Per-agent subsystem configurations for granular command scheduling';
COMMENT ON COLUMN agent_subsystems.subsystem IS 'Subsystem name: updates, storage, system, docker';
COMMENT ON COLUMN agent_subsystems.enabled IS 'Whether this subsystem is enabled for the agent';
COMMENT ON COLUMN agent_subsystems.interval_minutes IS 'How often to run this subsystem (in minutes)';
COMMENT ON COLUMN agent_subsystems.auto_run IS 'Whether the server should auto-schedule this subsystem';
COMMENT ON COLUMN agent_subsystems.last_run_at IS 'Last time this subsystem was executed';
COMMENT ON COLUMN agent_subsystems.next_run_at IS 'Next scheduled run time for auto-run subsystems';
