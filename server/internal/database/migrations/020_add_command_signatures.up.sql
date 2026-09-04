-- Migration: Add security features for RedFlag v0.2.x
-- Purpose: Add command signatures, security settings, audit trail, incidents tracking, and signing keys

-- Add signature column to agent_commands table
ALTER TABLE agent_commands ADD COLUMN IF NOT EXISTS signature VARCHAR(128);

-- Create security_settings table for user-configurable settings
CREATE TABLE IF NOT EXISTS security_settings (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    category VARCHAR(50) NOT NULL,
    key VARCHAR(100) NOT NULL,
    value JSONB NOT NULL,
    value_type VARCHAR(20) NOT NULL,
    requires_restart BOOLEAN DEFAULT false,
    updated_at TIMESTAMP DEFAULT NOW(),
    updated_by UUID REFERENCES users(id),
    is_encrypted BOOLEAN DEFAULT false,
    description TEXT,
    validation_rules JSONB,
    UNIQUE(category, key)
);

-- Create security_settings_audit table for audit trail
CREATE TABLE IF NOT EXISTS security_settings_audit (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    setting_id UUID REFERENCES security_settings(id),
    previous_value JSONB,
    new_value JSONB,
    changed_by UUID REFERENCES users(id),
    changed_at TIMESTAMP DEFAULT NOW(),
    ip_address INET,
    user_agent TEXT,
    reason TEXT
);

-- Create security_incidents table for tracking security events
CREATE TABLE IF NOT EXISTS security_incidents (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    incident_type VARCHAR(50) NOT NULL,
    severity VARCHAR(20) NOT NULL,
    agent_id UUID REFERENCES agents(id),
    description TEXT NOT NULL,
    metadata JSONB,
    resolved BOOLEAN DEFAULT false,
    resolved_at TIMESTAMP,
    resolved_by UUID REFERENCES users(id),
    created_at TIMESTAMP DEFAULT NOW()
);

-- Create signing_keys table for public key rotation
CREATE TABLE IF NOT EXISTS signing_keys (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    key_id VARCHAR(64) UNIQUE NOT NULL,
    public_key TEXT NOT NULL,
    algorithm VARCHAR(20) DEFAULT 'ed25519',
    is_active BOOLEAN DEFAULT true,
    is_primary BOOLEAN DEFAULT false,
    created_at TIMESTAMP DEFAULT NOW(),
    deprecated_at TIMESTAMP,
    version INTEGER DEFAULT 1
);

-- Create indexes for security_settings
CREATE INDEX IF NOT EXISTS idx_security_settings_category ON security_settings(category);
CREATE INDEX IF NOT EXISTS idx_security_settings_restart ON security_settings(requires_restart);

-- Create indexes for security_settings_audit
CREATE INDEX IF NOT EXISTS idx_security_audit_timestamp ON security_settings_audit(changed_at DESC);

-- Create indexes for security_incidents
CREATE INDEX IF NOT EXISTS idx_security_incidents_type ON security_incidents(incident_type);
CREATE INDEX IF NOT EXISTS idx_security_incidents_severity ON security_incidents(severity);
CREATE INDEX IF NOT EXISTS idx_security_incidents_resolved ON security_incidents(resolved);

-- Create indexes for signing_keys
CREATE INDEX IF NOT EXISTS idx_signing_keys_active ON signing_keys(is_active, is_primary);
CREATE INDEX IF NOT EXISTS idx_signing_keys_algorithm ON signing_keys(algorithm);

-- Add comments for documentation
COMMENT ON TABLE security_settings IS 'Stores user-configurable security settings for the RedFlag system';
COMMENT ON TABLE security_settings_audit IS 'Audit trail for all changes to security settings';
COMMENT ON TABLE security_incidents IS 'Tracks security incidents and events in the system';
COMMENT ON TABLE signing_keys IS 'Stores public signing keys with support for key rotation';

COMMENT ON COLUMN agent_commands.signature IS 'Digital signature of the command for verification';
COMMENT ON COLUMN security_settings.is_encrypted IS 'Indicates if the setting value should be encrypted at rest';
COMMENT ON COLUMN security_settings.validation_rules IS 'JSON schema for validating the setting value';
COMMENT ON COLUMN security_settings_audit.ip_address IS 'IP address of the user who made the change';
COMMENT ON COLUMN security_settings_audit.reason IS 'Optional reason for the configuration change';
COMMENT ON COLUMN security_incidents.metadata IS 'Additional structured data about the incident';
COMMENT ON COLUMN signing_keys.key_id IS 'Unique identifier for the signing key (e.g., fingerprint)';
COMMENT ON COLUMN signing_keys.version IS 'Version number for tracking key iterations';

-- Add check constraints for data integrity
ALTER TABLE security_settings ADD CONSTRAINT chk_value_type CHECK (value_type IN ('string', 'number', 'boolean', 'array', 'object'));

ALTER TABLE security_incidents ADD CONSTRAINT chk_incident_severity CHECK (severity IN ('low', 'medium', 'high', 'critical'));

ALTER TABLE signing_keys ADD CONSTRAINT chk_algorithm CHECK (algorithm IN ('ed25519', 'rsa', 'ecdsa', 'rsa-pss'));

-- Grant permissions (adjust as needed for your setup)
-- GRANT ALL PRIVILEGES ON TABLE security_settings TO redflag_user;
-- GRANT ALL PRIVILEGES ON TABLE security_settings_audit TO redflag_user;
-- GRANT ALL PRIVILEGES ON TABLE security_incidents TO redflag_user;
-- GRANT ALL PRIVILEGES ON TABLE signing_keys TO redflag_user;
-- GRANT USAGE ON SCHEMA public TO redflag_user;