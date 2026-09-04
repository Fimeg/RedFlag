-- Reversal of 037: drop the uniqueness constraint. Deleted duplicate rows
-- are not recreated — they were redundant signed artifacts of identical
-- binaries and re-creating them would only re-introduce the original bug.

DROP INDEX IF EXISTS uq_agent_update_packages_version_platform_arch;
