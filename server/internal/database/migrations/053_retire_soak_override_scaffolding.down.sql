-- Reverse of 053: restore the soak-override scaffolding from migration 050.

ALTER TABLE current_package_state
    ADD COLUMN IF NOT EXISTS soak_window_hours_override NUMERIC;

CREATE TABLE IF NOT EXISTS version_soak_overrides (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    update_id           UUID NOT NULL,
    agent_id            UUID NOT NULL,
    package_type        VARCHAR(32) NOT NULL,
    package_name        TEXT NOT NULL,
    version             TEXT NOT NULL,
    soak_days_remaining NUMERIC NOT NULL,
    override_reason     TEXT NOT NULL,
    overridden_by       TEXT NOT NULL,
    overridden_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_soak_overrides_update
    ON version_soak_overrides (update_id);

CREATE INDEX IF NOT EXISTS idx_soak_overrides_agent
    ON version_soak_overrides (agent_id);

CREATE INDEX IF NOT EXISTS idx_soak_overrides_time
    ON version_soak_overrides (overridden_at DESC);
