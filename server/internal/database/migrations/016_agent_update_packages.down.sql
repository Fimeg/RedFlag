-- Remove agent update packages table
DROP TABLE IF EXISTS agent_update_packages;

-- Remove new columns from agents table
ALTER TABLE agents
DROP COLUMN IF EXISTS machine_id,
DROP COLUMN IF EXISTS public_key_fingerprint,
DROP COLUMN IF EXISTS is_updating,
DROP COLUMN IF EXISTS updating_to_version,
DROP COLUMN IF EXISTS update_initiated_at;