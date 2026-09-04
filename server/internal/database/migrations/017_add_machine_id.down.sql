-- Rollback machine_id column addition

DROP INDEX IF EXISTS idx_agents_machine_id;
ALTER TABLE agents DROP COLUMN IF EXISTS machine_id;
