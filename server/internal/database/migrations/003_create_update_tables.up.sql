-- Event sourcing table for all update events
CREATE TABLE IF NOT EXISTS update_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    package_type VARCHAR(50) NOT NULL,
    package_name TEXT NOT NULL,
    version_from TEXT,
    version_to TEXT NOT NULL,
    severity VARCHAR(20) NOT NULL CHECK (severity IN ('critical', 'important', 'moderate', 'low')),
    repository_source TEXT,
    metadata JSONB DEFAULT '{}',
    event_type VARCHAR(20) NOT NULL CHECK (event_type IN ('discovered', 'updated', 'failed', 'ignored')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Current state table for optimized queries
CREATE TABLE IF NOT EXISTS current_package_state (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    package_type VARCHAR(50) NOT NULL,
    package_name TEXT NOT NULL,
    current_version TEXT NOT NULL,
    available_version TEXT,
    severity VARCHAR(20) NOT NULL CHECK (severity IN ('critical', 'important', 'moderate', 'low')),
    repository_source TEXT,
    metadata JSONB DEFAULT '{}',
    last_discovered_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    last_updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    status VARCHAR(20) NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'updated', 'failed', 'ignored', 'installing')),
    UNIQUE(agent_id, package_type, package_name)
);

-- Version history table for audit trails
CREATE TABLE IF NOT EXISTS update_version_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    package_type VARCHAR(50) NOT NULL,
    package_name TEXT NOT NULL,
    version_from TEXT NOT NULL,
    version_to TEXT NOT NULL,
    severity VARCHAR(20) NOT NULL CHECK (severity IN ('critical', 'important', 'moderate', 'low')),
    repository_source TEXT,
    metadata JSONB DEFAULT '{}',
    update_initiated_at TIMESTAMP WITH TIME ZONE,
    update_completed_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    update_status VARCHAR(20) NOT NULL CHECK (update_status IN ('success', 'failed', 'rollback')),
    failure_reason TEXT
);

-- Batch processing tracking
CREATE TABLE IF NOT EXISTS update_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    batch_size INTEGER NOT NULL,
    processed_count INTEGER DEFAULT 0,
    failed_count INTEGER DEFAULT 0,
    status VARCHAR(20) NOT NULL DEFAULT 'processing' CHECK (status IN ('processing', 'completed', 'failed', 'cancelled')),
    error_details JSONB DEFAULT '{}',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW(),
    completed_at TIMESTAMP WITH TIME ZONE
);

-- Create indexes for performance
CREATE INDEX IF NOT EXISTS idx_agent_events ON update_events(agent_id);
CREATE INDEX IF NOT EXISTS idx_package_events ON update_events(package_name, package_type);
CREATE INDEX IF NOT EXISTS idx_severity_events ON update_events(severity);
CREATE INDEX IF NOT EXISTS idx_created_events ON update_events(created_at);

CREATE INDEX IF NOT EXISTS idx_agent_state ON current_package_state(agent_id);
CREATE INDEX IF NOT EXISTS idx_package_state ON current_package_state(package_name, package_type);
CREATE INDEX IF NOT EXISTS idx_severity_state ON current_package_state(severity);
CREATE INDEX IF NOT EXISTS idx_status_state ON current_package_state(status);

CREATE INDEX IF NOT EXISTS idx_agent_history ON update_version_history(agent_id);
CREATE INDEX IF NOT EXISTS idx_package_history ON update_version_history(package_name, package_type);
CREATE INDEX IF NOT EXISTS idx_completed_history ON update_version_history(update_completed_at);

CREATE INDEX IF NOT EXISTS idx_agent_batches ON update_batches(agent_id);
CREATE INDEX IF NOT EXISTS idx_batch_status ON update_batches(status);
CREATE INDEX IF NOT EXISTS idx_created_batches ON update_batches(created_at);