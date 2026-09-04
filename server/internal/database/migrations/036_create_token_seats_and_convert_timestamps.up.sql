-- Migration 036: Create token_seats table and convert remaining TIMESTAMP columns to TIMESTAMPTZ.
-- Part 1: Create the missing token_seats table (migration 012 modified registration_tokens but never created token_seats)
-- Part 2: Convert all remaining TIMESTAMP columns to TIMESTAMPTZ.
-- The migration runner (db.go) wraps every file in its own transaction; do not
-- embed BEGIN/COMMIT here — an inner COMMIT closes the runner's tx early and the
-- runner's own Commit() then fails with "unexpected transaction status idle".

-- Part 1: Create the missing token_seats table
CREATE TABLE IF NOT EXISTS token_seats (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    token_id UUID NOT NULL REFERENCES registration_tokens(id) ON DELETE CASCADE,
    seat_number INT NOT NULL,
    used_by_agent_id UUID REFERENCES agents(id) ON DELETE SET NULL,
    used_at TIMESTAMPTZ,
    UNIQUE(token_id, seat_number)
);

CREATE INDEX IF NOT EXISTS idx_token_seats_token_id ON token_seats(token_id);
CREATE INDEX IF NOT EXISTS idx_token_seats_agent_id ON token_seats(used_by_agent_id);

-- Part 2: Convert remaining TIMESTAMP columns to TIMESTAMPTZ

-- agents
ALTER TABLE agents
    ALTER COLUMN last_seen           TYPE TIMESTAMPTZ USING last_seen           AT TIME ZONE 'UTC',
    ALTER COLUMN created_at          TYPE TIMESTAMPTZ USING created_at          AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at          TYPE TIMESTAMPTZ USING updated_at          AT TIME ZONE 'UTC',
    ALTER COLUMN last_version_check  TYPE TIMESTAMPTZ USING last_version_check  AT TIME ZONE 'UTC',
    ALTER COLUMN last_reboot_at      TYPE TIMESTAMPTZ USING last_reboot_at      AT TIME ZONE 'UTC',
    ALTER COLUMN update_initiated_at TYPE TIMESTAMPTZ USING update_initiated_at AT TIME ZONE 'UTC';

-- agent_specs
ALTER TABLE agent_specs
    ALTER COLUMN collected_at TYPE TIMESTAMPTZ USING collected_at AT TIME ZONE 'UTC';

-- update_packages
ALTER TABLE update_packages
    ALTER COLUMN discovered_at TYPE TIMESTAMPTZ USING discovered_at AT TIME ZONE 'UTC',
    ALTER COLUMN approved_at   TYPE TIMESTAMPTZ USING approved_at   AT TIME ZONE 'UTC',
    ALTER COLUMN scheduled_for TYPE TIMESTAMPTZ USING scheduled_for AT TIME ZONE 'UTC',
    ALTER COLUMN installed_at  TYPE TIMESTAMPTZ USING installed_at  AT TIME ZONE 'UTC';

-- update_logs
ALTER TABLE update_logs
    ALTER COLUMN executed_at TYPE TIMESTAMPTZ USING executed_at AT TIME ZONE 'UTC';

-- users
ALTER TABLE users
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN last_login TYPE TIMESTAMPTZ USING last_login AT TIME ZONE 'UTC';

-- agent_commands
ALTER TABLE agent_commands
    ALTER COLUMN created_at   TYPE TIMESTAMPTZ USING created_at   AT TIME ZONE 'UTC',
    ALTER COLUMN sent_at      TYPE TIMESTAMPTZ USING sent_at      AT TIME ZONE 'UTC',
    ALTER COLUMN completed_at TYPE TIMESTAMPTZ USING completed_at AT TIME ZONE 'UTC',
    ALTER COLUMN signed_at    TYPE TIMESTAMPTZ USING signed_at    AT TIME ZONE 'UTC',
    ALTER COLUMN expires_at   TYPE TIMESTAMPTZ USING expires_at   AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at   TYPE TIMESTAMPTZ USING updated_at   AT TIME ZONE 'UTC';

-- refresh_tokens
ALTER TABLE refresh_tokens
    ALTER COLUMN expires_at   TYPE TIMESTAMPTZ USING expires_at   AT TIME ZONE 'UTC',
    ALTER COLUMN created_at   TYPE TIMESTAMPTZ USING created_at   AT TIME ZONE 'UTC',
    ALTER COLUMN last_used_at TYPE TIMESTAMPTZ USING last_used_at AT TIME ZONE 'UTC';

-- registration_tokens
ALTER TABLE registration_tokens
    ALTER COLUMN expires_at TYPE TIMESTAMPTZ USING expires_at AT TIME ZONE 'UTC',
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC',
    ALTER COLUMN used_at    TYPE TIMESTAMPTZ USING used_at    AT TIME ZONE 'UTC',
    ALTER COLUMN revoked_at TYPE TIMESTAMPTZ USING revoked_at AT TIME ZONE 'UTC';

-- token_seats (newly created table)
ALTER TABLE token_seats
    ALTER COLUMN used_at TYPE TIMESTAMPTZ USING used_at AT TIME ZONE 'UTC';

-- agent_subsystems
ALTER TABLE agent_subsystems
    ALTER COLUMN last_run_at TYPE TIMESTAMPTZ USING last_run_at AT TIME ZONE 'UTC',
    ALTER COLUMN next_run_at TYPE TIMESTAMPTZ USING next_run_at AT TIME ZONE 'UTC',
    ALTER COLUMN created_at  TYPE TIMESTAMPTZ USING created_at  AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at  TYPE TIMESTAMPTZ USING updated_at  AT TIME ZONE 'UTC';

-- agent_update_packages
ALTER TABLE agent_update_packages
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC';

-- security_settings
ALTER TABLE security_settings
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC',
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC';

-- security_settings_audit
ALTER TABLE security_settings_audit
    ALTER COLUMN changed_at TYPE TIMESTAMPTZ USING changed_at AT TIME ZONE 'UTC';

-- security_incidents
ALTER TABLE security_incidents
    ALTER COLUMN resolved_at TYPE TIMESTAMPTZ USING resolved_at AT TIME ZONE 'UTC',
    ALTER COLUMN created_at  TYPE TIMESTAMPTZ USING created_at  AT TIME ZONE 'UTC';

-- signing_keys
ALTER TABLE signing_keys
    ALTER COLUMN created_at    TYPE TIMESTAMPTZ USING created_at    AT TIME ZONE 'UTC',
    ALTER COLUMN deprecated_at TYPE TIMESTAMPTZ USING deprecated_at AT TIME ZONE 'UTC';

-- storage_metrics
ALTER TABLE storage_metrics
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC';

-- client_errors
ALTER TABLE client_errors
    ALTER COLUMN created_at TYPE TIMESTAMPTZ USING created_at AT TIME ZONE 'UTC';

-- scanner_config
ALTER TABLE scanner_config
    ALTER COLUMN updated_at TYPE TIMESTAMPTZ USING updated_at AT TIME ZONE 'UTC';

-- tracked_software
ALTER TABLE tracked_software
    ALTER COLUMN latest_at       TYPE TIMESTAMPTZ USING latest_at       AT TIME ZONE 'UTC',
    ALTER COLUMN eol_at          TYPE TIMESTAMPTZ USING eol_at          AT TIME ZONE 'UTC',
    ALTER COLUMN last_checked_at TYPE TIMESTAMPTZ USING last_checked_at AT TIME ZONE 'UTC',
    ALTER COLUMN last_synced_at  TYPE TIMESTAMPTZ USING last_synced_at  AT TIME ZONE 'UTC',
    ALTER COLUMN created_at      TYPE TIMESTAMPTZ USING created_at      AT TIME ZONE 'UTC',
    ALTER COLUMN updated_at      TYPE TIMESTAMPTZ USING updated_at      AT TIME ZONE 'UTC';

-- upstream_drift_events
ALTER TABLE upstream_drift_events
    ALTER COLUMN observed_at TYPE TIMESTAMPTZ USING observed_at AT TIME ZONE 'UTC';
