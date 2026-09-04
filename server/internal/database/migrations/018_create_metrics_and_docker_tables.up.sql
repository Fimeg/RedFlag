-- Migration: Create separate tables for metrics and docker images
-- Purpose: Fix data classification issue where storage/system metrics were incorrectly stored as package updates

-- Create metrics table for system and storage metrics
CREATE TABLE IF NOT EXISTS metrics (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    package_type VARCHAR(50) NOT NULL,  -- "storage", "system", "cpu", "memory"
    package_name VARCHAR(255) NOT NULL,
    current_version TEXT NOT NULL,     -- current usage, value
    available_version TEXT NOT NULL, -- available space, threshold
    severity VARCHAR(20) NOT NULL DEFAULT 'low', -- "low", "moderate", "high", "critical"
    repository_source VARCHAR(255),
    metadata JSONB DEFAULT '{}',
    event_type VARCHAR(50) NOT NULL DEFAULT 'discovered',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Unique constraint to prevent duplicate entries
    UNIQUE (agent_id, package_name, package_type, created_at)
);

-- Create docker_images table for Docker image information
CREATE TABLE IF NOT EXISTS docker_images (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    package_type VARCHAR(50) NOT NULL DEFAULT 'docker_image',
    package_name VARCHAR(500) NOT NULL,  -- image name:tag
    current_version VARCHAR(255) NOT NULL,  -- current image ID
    available_version VARCHAR(255),      -- latest image ID
    severity VARCHAR(20) NOT NULL DEFAULT 'low', -- "low", "moderate", "high", "critical"
    repository_source VARCHAR(500),        -- registry URL
    metadata JSONB DEFAULT '{}',
    event_type VARCHAR(50) NOT NULL DEFAULT 'discovered',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),

    -- Unique constraint to prevent duplicate entries
    UNIQUE (agent_id, package_name, package_type, created_at)
);

-- Create indexes for better performance
CREATE INDEX IF NOT EXISTS idx_metrics_agent_id ON metrics(agent_id);
CREATE INDEX IF NOT EXISTS idx_metrics_package_type ON metrics(package_type);
CREATE INDEX IF NOT EXISTS idx_metrics_created_at ON metrics(created_at);
CREATE INDEX IF NOT EXISTS idx_metrics_severity ON metrics(severity);

CREATE INDEX IF NOT EXISTS idx_docker_images_agent_id ON docker_images(agent_id);
CREATE INDEX IF NOT EXISTS idx_docker_images_package_type ON docker_images(package_type);
CREATE INDEX IF NOT EXISTS idx_docker_images_created_at ON docker_images(created_at);
CREATE INDEX IF NOT EXISTS idx_docker_images_severity ON docker_images(severity);
CREATE INDEX IF NOT EXISTS idx_docker_images_has_updates ON docker_images(current_version, available_version) WHERE current_version != available_version;

-- Add comments for documentation
COMMENT ON TABLE metrics IS 'Stores system and storage metrics collected from agents, separate from package updates';
COMMENT ON TABLE docker_images IS 'Stores Docker image information and update availability, separate from package updates';

COMMENT ON COLUMN metrics.package_type IS 'Type of metric: storage, system, cpu, memory, etc.';
COMMENT ON COLUMN metrics.package_name IS 'Name of the metric (mount point, metric name, etc.)';
COMMENT ON COLUMN metrics.current_version IS 'Current value or usage';
COMMENT ON COLUMN metrics.available_version IS 'Available space or threshold';
COMMENT ON COLUMN metrics.severity IS 'Severity level: low, moderate, high, critical';

COMMENT ON COLUMN docker_images.package_name IS 'Docker image name with tag (e.g., nginx:latest)';
COMMENT ON COLUMN docker_images.current_version IS 'Current image ID';
COMMENT ON COLUMN docker_images.available_version IS 'Latest image ID';
COMMENT ON COLUMN docker_images.severity IS 'Update severity: low, moderate, high, critical';

-- Create or replace function to clean old data (optional)
CREATE OR REPLACE FUNCTION clean_misclassified_data()
RETURNS INTEGER AS $$
DECLARE
    deleted_count INTEGER := 0;
BEGIN
    -- This function can be called to clean up any storage/system metrics that were
    -- incorrectly stored in the update_events table before migration

    -- For now, just return 0 as we're keeping the old data for audit purposes
    RETURN deleted_count;
END;
$$ LANGUAGE plpgsql;

-- Grant permissions (adjust as needed for your setup)
-- GRANT ALL PRIVILEGES ON TABLE metrics TO redflag_user;
-- GRANT ALL PRIVILEGES ON TABLE docker_images TO redflag_user;
-- GRANT USAGE ON SCHEMA public TO redflag_user;