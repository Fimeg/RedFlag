-- Migration 032: Add updated_at to agent_commands
-- The Go struct defines UpdatedAt with db:"updated_at" tag but no
-- migration ever added the column. Any SELECT * on agent_commands
-- would attempt to scan into a non-existent column. This closes
-- the schema gap.

ALTER TABLE agent_commands
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMP NOT NULL DEFAULT NOW();

-- Backfill existing rows that still have NULL updated_at
UPDATE agent_commands SET updated_at = created_at WHERE updated_at IS NULL;

-- Keep updated_at in sync with status changes via trigger
CREATE OR REPLACE FUNCTION update_agent_commands_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trigger_agent_commands_updated_at ON agent_commands;
CREATE TRIGGER trigger_agent_commands_updated_at
    BEFORE UPDATE ON agent_commands
    FOR EACH ROW
    EXECUTE FUNCTION update_agent_commands_updated_at();
