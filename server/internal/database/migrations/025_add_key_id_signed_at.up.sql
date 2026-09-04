-- Add key_id and signed_at to agent_commands for key-rotation-aware verification
ALTER TABLE agent_commands ADD COLUMN IF NOT EXISTS key_id VARCHAR(64);
ALTER TABLE agent_commands ADD COLUMN IF NOT EXISTS signed_at TIMESTAMP;
CREATE INDEX IF NOT EXISTS idx_agent_commands_key_id ON agent_commands(key_id);
COMMENT ON COLUMN agent_commands.key_id IS 'Fingerprint of the signing key used to sign this command';
COMMENT ON COLUMN agent_commands.signed_at IS 'Timestamp when command was signed, used for replay protection';
