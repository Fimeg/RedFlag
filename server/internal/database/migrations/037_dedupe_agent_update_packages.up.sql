-- Migration 037: deduplicate agent_update_packages and enforce uniqueness.
--
-- The BuildOrchestratorService used to re-sign and re-insert every platform
-- binary at every server boot, accumulating 4 fresh rows per restart for the
-- same artifact. The dashboard's "available packages" list ballooned to dozens
-- of identical entries and forced operators to scroll past them when picking a
-- target version.
--
-- The service-side fix lives in build_orchestrator.go (reuse the row if the
-- on-disk checksum matches an existing signed package); this migration
-- cleans up historical duplicates and adds the unique constraint that makes
-- the duplication impossible at the schema level going forward.
--
-- Strategy: keep the newest row per (version, platform, architecture).
-- created_at is the tiebreaker, id breaks ties when two inserts share a
-- timestamp.

DELETE FROM agent_update_packages a
USING agent_update_packages b
WHERE a.version = b.version
  AND a.platform = b.platform
  AND a.architecture = b.architecture
  AND (a.created_at < b.created_at
       OR (a.created_at = b.created_at AND a.id < b.id));

CREATE UNIQUE INDEX IF NOT EXISTS uq_agent_update_packages_version_platform_arch
  ON agent_update_packages (version, platform, architecture);
