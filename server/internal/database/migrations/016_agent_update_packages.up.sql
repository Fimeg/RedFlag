-- Add machine ID and public key fingerprint fields to agents table
-- This enables Ed25519 binary signing and machine binding

ALTER TABLE agents
ADD COLUMN IF NOT EXISTS machine_id VARCHAR(64) UNIQUE,
ADD COLUMN IF NOT EXISTS public_key_fingerprint VARCHAR(16),
ADD COLUMN IF NOT EXISTS is_updating BOOLEAN DEFAULT false,
ADD COLUMN IF NOT EXISTS updating_to_version VARCHAR(50),
ADD COLUMN IF NOT EXISTS update_initiated_at TIMESTAMP;

-- Create index for machine ID lookups
CREATE INDEX IF NOT EXISTS idx_agents_machine_id ON agents(machine_id);
CREATE INDEX IF NOT EXISTS idx_agents_public_key_fingerprint ON agents(public_key_fingerprint);

-- Add comment to document the new fields
COMMENT ON COLUMN agents.machine_id IS 'Unique machine identifier to bind agent binaries to specific hardware';
COMMENT ON COLUMN agents.public_key_fingerprint IS 'Fingerprint of embedded public key for binary signature verification';
COMMENT ON COLUMN agents.is_updating IS 'Whether agent is currently updating';
COMMENT ON COLUMN agents.updating_to_version IS 'Target version for ongoing update';
COMMENT ON COLUMN agents.update_initiated_at IS 'When the update process started';

-- Create table for storing signed update packages
CREATE TABLE IF NOT EXISTS agent_update_packages (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    version VARCHAR(50) NOT NULL,
    platform VARCHAR(50) NOT NULL, -- linux-amd64, linux-arm64, windows-amd64, etc.
    architecture VARCHAR(20) NOT NULL,
    binary_path VARCHAR(500) NOT NULL,
    signature VARCHAR(128) NOT NULL, -- Ed25519 signature (64 bytes hex encoded)
    checksum VARCHAR(64) NOT NULL, -- SHA-256 checksum
    file_size BIGINT NOT NULL,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_by VARCHAR(100) DEFAULT 'system',
    is_active BOOLEAN DEFAULT true
);

-- Add indexes for update packages
CREATE INDEX IF NOT EXISTS idx_agent_update_packages_version ON agent_update_packages(version);
CREATE INDEX IF NOT EXISTS idx_agent_update_packages_platform ON agent_update_packages(platform, architecture);
CREATE INDEX IF NOT EXISTS idx_agent_update_packages_active ON agent_update_packages(is_active);

-- Add comments for update packages table
COMMENT ON TABLE agent_update_packages IS 'Stores signed agent binary packages for secure updates';
COMMENT ON COLUMN agent_update_packages.signature IS 'Ed25519 signature of the binary file';
COMMENT ON COLUMN agent_update_packages.checksum IS 'SHA-256 checksum of the binary file';
COMMENT ON COLUMN agent_update_packages.platform IS 'Target platform (OS-architecture)';
COMMENT ON COLUMN agent_update_packages.is_active IS 'Whether this package is available for updates';