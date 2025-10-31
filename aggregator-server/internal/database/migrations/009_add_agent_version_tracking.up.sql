-- Add version tracking to agents table
-- This enables the hybrid version tracking system

ALTER TABLE agents
ADD COLUMN current_version VARCHAR(50) DEFAULT '0.1.3',
ADD COLUMN update_available BOOLEAN DEFAULT FALSE,
ADD COLUMN last_version_check TIMESTAMP DEFAULT CURRENT_TIMESTAMP;

-- Add index for faster queries on update status
CREATE INDEX idx_agents_update_available ON agents(update_available);
CREATE INDEX idx_agents_current_version ON agents(current_version);

-- Add comment to document the purpose
COMMENT ON COLUMN agents.current_version IS 'The version of the agent currently running';
COMMENT ON COLUMN agents.update_available IS 'Whether an update is available for this agent';
COMMENT ON COLUMN agents.last_version_check IS 'Last time the agent version was checked';