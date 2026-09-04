DROP INDEX IF EXISTS idx_upstream_drift_events_recent;
DROP INDEX IF EXISTS idx_upstream_drift_events_software;
DROP TABLE IF EXISTS upstream_drift_events;

DROP INDEX IF EXISTS idx_tracked_software_drift;
DROP INDEX IF EXISTS idx_tracked_software_enabled;
DROP TABLE IF EXISTS tracked_software;
