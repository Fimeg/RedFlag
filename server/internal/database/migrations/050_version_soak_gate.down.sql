-- Reverse GATE-005: version soak gate.
DROP TABLE IF EXISTS version_soak_overrides;
DROP INDEX IF EXISTS idx_cps_selected_version;
ALTER TABLE current_package_state
    DROP COLUMN IF EXISTS soak_window_hours_override;
ALTER TABLE current_package_state
    DROP COLUMN IF EXISTS selected_version;
