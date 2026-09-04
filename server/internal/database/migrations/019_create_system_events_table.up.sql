-- Migration: Create system_events table for unified event logging
-- Reference: docs/ERROR_FLOW_AUDIT.md

CREATE TABLE IF NOT EXISTS system_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID REFERENCES agents(id) ON DELETE CASCADE,
    event_type VARCHAR(50) NOT NULL,        -- 'agent_update', 'agent_startup', 'agent_scan', 'server_build', etc.
    event_subtype VARCHAR(50) NOT NULL,     -- 'success', 'failed', 'info', 'warning', 'critical'
    severity VARCHAR(20) NOT NULL,          -- 'info', 'warning', 'error', 'critical'
    component VARCHAR(50) NOT NULL,         -- 'agent', 'server', 'build', 'download', 'config', etc.
    message TEXT,
    metadata JSONB DEFAULT '{}',           -- Structured event data (stack traces, HTTP codes, etc.)
    created_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

-- Performance indexes for common query patterns
CREATE INDEX IF NOT EXISTS idx_system_events_agent_id ON system_events(agent_id);
CREATE INDEX IF NOT EXISTS idx_system_events_type_subtype ON system_events(event_type, event_subtype);
CREATE INDEX IF NOT EXISTS idx_system_events_created_at ON system_events(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_system_events_severity ON system_events(severity);
CREATE INDEX IF NOT EXISTS idx_system_events_component ON system_events(component);

-- Composite index for agent timeline queries (agent + time range)
CREATE INDEX IF NOT EXISTS idx_system_events_agent_timeline ON system_events(agent_id, created_at DESC);

-- Partial index for error events (faster error dashboard queries)
CREATE INDEX IF NOT EXISTS idx_system_events_errors ON system_events(severity, created_at DESC)
WHERE severity IN ('error', 'critical');

-- GIN index for metadata JSONB queries (allows searching event metadata)
CREATE INDEX IF NOT EXISTS idx_system_events_metadata_gin ON system_events USING GIN(metadata);

-- Comment for documentation
COMMENT ON TABLE system_events IS 'Unified event logging table for all system events (agent + server)';
COMMENT ON COLUMN system_events.event_type IS 'High-level event category (e.g., agent_update, agent_startup)';
COMMENT ON COLUMN system_events.event_subtype IS 'Event outcome/status (e.g., success, failed, info, warning)';
COMMENT ON COLUMN system_events.severity IS 'Event severity level for filtering and alerting';
COMMENT ON COLUMN system_events.component IS 'System component that generated the event';
COMMENT ON COLUMN system_events.metadata IS 'JSONB field for structured event data (stack traces, HTTP codes, etc.)';