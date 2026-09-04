-- Reversal of migration 051
DROP INDEX IF EXISTS idx_ats_package_name;
DROP INDEX IF EXISTS idx_ats_match_method;
DROP INDEX IF EXISTS idx_repology_aliases_slug;
DROP INDEX IF EXISTS idx_tracked_software_repology_slug;
DROP TABLE IF EXISTS repology_aliases;
ALTER TABLE agent_tracked_software
    DROP COLUMN IF EXISTS package_name,
    DROP COLUMN IF EXISTS match_method;
ALTER TABLE tracked_software
    DROP COLUMN IF EXISTS binary_probe,
    DROP COLUMN IF EXISTS container_image_pattern,
    DROP COLUMN IF EXISTS repology_slug;
