-- Create dedicated storage_metrics table for proper storage tracking
-- This replaces the misuse of metrics table for storage data

CREATE TABLE IF NOT EXISTS storage_metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    mountpoint VARCHAR(255) NOT NULL,
    device VARCHAR(255),
    disk_type VARCHAR(50),
    filesystem VARCHAR(50),
    total_bytes BIGINT,
    used_bytes BIGINT,
    available_bytes BIGINT,
    used_percent FLOAT,
    severity VARCHAR(20),
    metadata JSONB,
    created_at TIMESTAMP NOT NULL DEFAULT NOW()
);

-- Indexes for performance
CREATE INDEX IF NOT EXISTS idx_storage_metrics_agent_id ON storage_metrics(agent_id);
CREATE INDEX IF NOT EXISTS idx_storage_metrics_created_at ON storage_metrics(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_storage_metrics_mountpoint ON storage_metrics(mountpoint);
CREATE INDEX IF NOT EXISTS idx_storage_metrics_agent_mount ON storage_metrics(agent_id, mountpoint, created_at DESC);
