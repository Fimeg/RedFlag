-- Migration 023: Client Error Logging Schema
-- Implements ETHOS #1: Errors are History, Not /dev/null

CREATE TABLE IF NOT EXISTS client_errors (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    subsystem VARCHAR(50) NOT NULL,
    error_type VARCHAR(50) NOT NULL,
    message TEXT NOT NULL,
    stack_trace TEXT,
    metadata JSONB,
    url TEXT NOT NULL,
    user_agent TEXT,
    created_at TIMESTAMP DEFAULT NOW()
);

-- Indexes for efficient querying
CREATE INDEX IF NOT EXISTS idx_client_errors_agent_time ON client_errors(agent_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_client_errors_subsystem_time ON client_errors(subsystem, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_client_errors_error_type_time ON client_errors(error_type, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_client_errors_created_at ON client_errors(created_at DESC);

-- Comments for documentation
COMMENT ON TABLE client_errors IS 'Frontend error logs for debugging and auditing. Implements ETHOS #1.';
COMMENT ON COLUMN client_errors.agent_id IS 'Agent active when error occurred (NULL for pre-auth errors)';
COMMENT ON COLUMN client_errors.subsystem IS 'RedFlag subsystem being used (storage, system, docker, etc.)';
COMMENT ON COLUMN client_errors.error_type IS 'Error category: javascript_error, api_error, ui_error, validation_error';
COMMENT ON COLUMN client_errors.metadata IS 'Additional context (component name, API response, user actions)';
