-- Migration 036: Rollback - Drop token_seats and restore TIMESTAMP columns

BEGIN;

-- Drop token_seats table
DROP TABLE IF EXISTS token_seats CASCADE;

-- Revert TIMESTAMP columns back to TIMESTAMP

-- agents
ALTER TABLE agents
    ALTER COLUMN last_seen           TYPE TIMESTAMP USING last_seen           AT TIME ZONE 'UTC',
    ALTER COLUMN created_at          TYPE TIMESTAMP USING created_at          AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at          TYPE TIMESTAMP USING updated_at          AT TIME ZONE 'UTC',
    ALTER COLUMN last_version_check  TYPE TIMESTAMP USING last_version_check  AT TIME ZONE 'UTC',
    ALTER COLUMN last_reboot_at      TYPE TIMESTAMP USING last_reboot_at      AT TIME ZONE 'UTC',
    ALTER COLUMN update_initiated_at TYPE TIMESTAMP USING update_initiated_at AT TIME ZONE 'UTC';

-- agent_specs
ALTER TABLE agent_specs
    ALTER COLUMN collected_at TYPE TIMESTAMP USING collected_at AT TIME ZONE 'UTC';

-- update_packages
ALTER TABLE update_packages
    ALTER COLUMN discovered_at TYPE TIMESTAMP USING discovered_at AT TIME ZONE 'UTC',
    ALTER COLUMN approved_at   TYPE TIMESTAMP USING approved_at   AT TIME ZONE 'UTC',
    ALTER COLUMN scheduled_for TYPE TIMESTAMP USING scheduled_for AT TIME ZONE 'UTC',
    ALTER COLUMN installed_at  TYPE TIMESTAMP USING installed_at  AT TIME ZONE 'UTC';

-- update_logs
ALTER TABLE update_logs
    ALTER COLUMN executed_at TYPE TIMESTAMP USING executed_at AT TIME ZONE 'UTC';

-- users
ALTER TABLE users
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN last_login TYPE TIMESTAMP USING last_login AT TIME ZONE 'UTC';

-- agent_commands
ALTER TABLE agent_commands
    ALTER COLUMN created_at   TYPE TIMESTAMP USING created_at   AT TIME ZONE 'UTC',
    ALTER COLUMN sent_at      TYPE TIMESTAMP USING sent_at      AT TIME ZONE 'UTC',
    ALTER COLUMN completed_at TYPE TIMESTAMP USING completed_at AT TIME ZONE 'UTC',
    ALTER COLUMN signed_at    TYPE TIMESTAMP USING signed_at    AT TIME ZONE 'UTC',
    ALTER COLUMN expires_at   TYPE TIMESTAMP USING expires_at   AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at   TYPE TIMESTAMP USING updated_at   AT TIME ZONE 'UTC';

-- refresh_tokens
ALTER TABLE refresh_tokens
    ALTER COLUMN expires_at   TYPE TIMESTAMP USING expires_at   AT TIME ZONE 'UTC',
    ALTER COLUMN created_at   TYPE TIMESTAMP USING created_at   AT TIME ZONE 'UTC',
    ALTER COLUMN last_used_at TYPE TIMESTAMP USING last_used_at AT TIME ZONE 'UTC';

-- registration_tokens
ALTER TABLE registration_tokens
    ALTER COLUMN expires_at TYPE TIMESTAMP USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN used_at    TYPE TIMESTAMP USING used_at    AT TIME ZONE 'UTC',
    ALTER COLUMN revoked_at TYPE TIMESTAMP USING revoked_at AT TIME ZONE 'UTC';

-- agent_subsystems
ALTER TABLE agent_subsystems
    ALTER COLUMN last_run_at TYPE TIMESTAMP USING last_run_at AT TIME ZONE 'UTC',
    ALTER COLUMN next_run_at TYPE TIMESTAMP USING next_run_at AT TIME ZONE 'UTC',
    ALTER COLUMN created_at  TYPE TIMESTAMP USING created_at  AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at  TYPE TIMESTAMP USING updated_at  AT TIME ZONE 'UTC';

-- agent_update_packages
ALTER TABLE agent_update_packages
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC';

-- security_settings
ALTER TABLE security_settings
    ALTER COLUMN updated_at TYPE TIMESTAMP USING updated_at AT TIME ZONE 'UTC',
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC';

-- security_settings_audit
ALTER TABLE security_settings_audit
    ALTER COLUMN changed_at TYPE TIMESTAMP USING changed_at AT TIME ZONE 'UTC';

-- security_incidents
ALTER TABLE security_incidents
    ALTER COLUMN resolved_at TYPE TIMESTAMP USING resolved_at AT TIME ZONE 'UTC',
    ALTER COLUMN created_at  TYPE TIMESTAMP USING created_at  AT TIME ZONE 'UTC';

-- signing_keys
ALTER TABLE signing_keys
    ALTER COLUMN created_at    TYPE TIMESTAMP USING created_at    AT TIME ZONE 'UTC',
    ALTER COLUMN deprecated_at TYPE TIMESTAMP USING deprecated_at AT TIME ZONE 'UTC';

-- storage_metrics
ALTER TABLE storage_metrics
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC';

-- client_errors
ALTER TABLE client_errors
    ALTER COLUMN created_at TYPE TIMESTAMP USING created_at AT TIME ZONE 'UTC';

-- scanner_config
ALTER TABLE scanner_config
    ALTER COLUMN updated_at TYPE TIMESTAMP USING updated_at AT TIME ZONE 'UTC';

-- tracked_software
ALTER TABLE tracked_software
    ALTER COLUMN latest_at       TYPE TIMESTAMP USING latest_at       AT TIME ZONE 'UTC',
    ALTER COLUMN eol_at          TYPE TIMESTAMP USING eol_at          AT TIME ZONE 'UTC',
    ALTER COLUMN last_checked_at TYPE TIMESTAMP USING last_checked_at AT TIME ZONE 'UTC',
    ALTER COLUMN last_synced_at  TYPE TIMESTAMP USING last_synced_at  AT TIME ZONE 'UTC',
    ALTER COLUMN created_at      TYPE TIMESTAMP USING created_at      AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at      TYPE TIMESTAMP USING updated_at      AT TIME ZONE 'UTC';

-- upstream_drift_events
ALTER TABLE upstream_drift_events
    ALTER COLUMN observed_at TYPE TIMESTAMP USING observed_at AT TIME ZONE 'UTC';

COMMIT;
