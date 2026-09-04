-- Down Migration: Remove security features for RedFlag v0.2.x
-- Purpose: Rollback migration 020 - remove security-related tables and columns

-- Drop indexes first
DROP INDEX IF EXISTS idx_security_settings_category;
DROP INDEX IF EXISTS idx_security_settings_restart;
DROP INDEX IF EXISTS idx_security_audit_timestamp;
DROP INDEX IF EXISTS idx_security_incidents_type;
DROP INDEX IF EXISTS idx_security_incidents_severity;
DROP INDEX IF EXISTS idx_security_incidents_resolved;
DROP INDEX IF EXISTS idx_signing_keys_active;
DROP INDEX IF EXISTS idx_signing_keys_algorithm;

-- Drop check constraints
ALTER TABLE security_settings DROP CONSTRAINT IF EXISTS chk_value_type;
ALTER TABLE security_incidents DROP CONSTRAINT IF EXISTS chk_incident_severity;
ALTER TABLE signing_keys DROP CONSTRAINT IF EXISTS chk_algorithm;

-- Drop tables in reverse order to avoid foreign key constraints
DROP TABLE IF EXISTS signing_keys;
DROP TABLE IF EXISTS security_incidents;
DROP TABLE IF EXISTS security_settings_audit;
DROP TABLE IF EXISTS security_settings;

-- Remove signature column from agent_commands table
ALTER TABLE agent_commands DROP COLUMN IF EXISTS signature;