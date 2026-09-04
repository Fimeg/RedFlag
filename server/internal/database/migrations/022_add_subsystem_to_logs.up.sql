-- Migration: Add subsystem column to update_logs table
-- Purpose: Make subsystem context explicit (not parsed from action field)

-- Add subsystem column
ALTER TABLE update_logs
ADD COLUMN IF NOT EXISTS subsystem VARCHAR(50);

-- Create indexes for subsystem filtering
CREATE INDEX IF NOT EXISTS idx_logs_subsystem ON update_logs(subsystem);
CREATE INDEX IF NOT EXISTS idx_logs_agent_subsystem ON update_logs(agent_id, subsystem);

-- Backfill subsystem from action field for existing scan entries
UPDATE update_logs
SET subsystem = CASE
    WHEN action = 'scan_docker' THEN 'docker'
    WHEN action = 'scan_storage' THEN 'storage'
    WHEN action = 'scan_system' THEN 'system'
    WHEN action = 'scan_apt' THEN 'apt'
    WHEN action = 'scan_dnf' THEN 'dnf'
    WHEN action = 'scan_winget' THEN 'winget'
    WHEN action = 'scan_updates' THEN 'updates'
    ELSE NULL
END
WHERE action LIKE 'scan_%' AND subsystem IS NULL;

-- Add check constraint for valid subsystem values
ALTER TABLE update_logs
ADD CONSTRAINT chk_update_logs_subsystem
CHECK (subsystem IS NULL OR subsystem IN (
    'docker', 'storage', 'system', 'apt', 'dnf', 'winget', 'updates',
    'agent', 'security', 'network', 'heartbeat'
));

-- Add comment for documentation
COMMENT ON COLUMN update_logs.subsystem IS 'Subsystem that generated this log entry (e.g., docker, storage, system)';

-- Grant permissions (adjust as needed for your setup)
-- GRANT ALL PRIVILEGES ON TABLE update_logs TO redflag_user;
