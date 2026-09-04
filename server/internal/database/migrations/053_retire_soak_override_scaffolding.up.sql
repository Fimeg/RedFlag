-- GATE-005 cleanup: retire dead soak-override scaffolding.
--
-- Migration 050 added a per-package `soak_window_hours_override` column and a
-- `version_soak_overrides` table, but neither was ever wired: the column was
-- loaded and never read, and no query ever touched the table. The soak gate is
-- now configured as a proper security setting (supply_chain.soak_window_days /
-- soak_enforcement, admin-settable with defaults) and operator overrides are
-- journaled to system_events alongside every other gate override. Drop the
-- unused scaffolding so the schema reflects what the code actually does.
--
-- `selected_version` (also from 050) is kept — it is live (SetTargetVersion).

DROP TABLE IF EXISTS version_soak_overrides;

ALTER TABLE current_package_state
    DROP COLUMN IF EXISTS soak_window_hours_override;
