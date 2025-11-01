-- Add source field to agent_commands table to track command origin
-- 'manual' = user-initiated via UI
-- 'system' = automatically triggered by system operations (scans, installs, etc)

ALTER TABLE agent_commands
ADD COLUMN source VARCHAR(20) DEFAULT 'manual' NOT NULL;

-- Add check constraint to ensure valid source values
ALTER TABLE agent_commands
ADD CONSTRAINT agent_commands_source_check
CHECK (source IN ('manual', 'system'));

-- Add index for filtering commands by source
CREATE INDEX idx_agent_commands_source ON agent_commands(source);

-- Update comment
COMMENT ON COLUMN agent_commands.source IS 'Command origin: manual (user-initiated) or system (auto-triggered)';
