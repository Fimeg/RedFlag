-- Add reboot tracking fields to agents table
ALTER TABLE agents
ADD COLUMN IF NOT EXISTS reboot_required BOOLEAN DEFAULT FALSE,
ADD COLUMN IF NOT EXISTS last_reboot_at TIMESTAMP,
ADD COLUMN IF NOT EXISTS reboot_reason TEXT DEFAULT '';

-- Add index for efficient querying of agents needing reboot
CREATE INDEX IF NOT EXISTS idx_agents_reboot_required ON agents(reboot_required) WHERE reboot_required = TRUE;

-- Add comment for documentation
COMMENT ON COLUMN agents.reboot_required IS 'Whether the agent host requires a reboot to complete updates';
COMMENT ON COLUMN agents.last_reboot_at IS 'Timestamp of the last system reboot';
COMMENT ON COLUMN agents.reboot_reason IS 'Reason why reboot is required (e.g., kernel update, library updates)';
