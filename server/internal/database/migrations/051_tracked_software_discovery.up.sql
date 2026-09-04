-- BRIDGE-001: Tracked Software Auto-Discovery Bridge
--
-- Adds Repology alias cache, discovery columns on tracked_software,
-- and match_method tracking on agent_tracked_software.

-- 1. Discovery columns on tracked_software
ALTER TABLE tracked_software
    ADD COLUMN IF NOT EXISTS repology_slug TEXT,
    ADD COLUMN IF NOT EXISTS container_image_pattern TEXT,
    ADD COLUMN IF NOT EXISTS binary_probe TEXT;

-- 2. Repology alias cache
CREATE TABLE IF NOT EXISTS repology_aliases (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    slug        TEXT NOT NULL,
    ecosystem   TEXT NOT NULL,
    pkg_names   TEXT[] NOT NULL DEFAULT '{}',
    repo_raw    TEXT,
    fetched_at  TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE (slug, ecosystem)
);

-- 3. Match metadata on agent_tracked_software
ALTER TABLE agent_tracked_software
    ADD COLUMN IF NOT EXISTS match_method TEXT,
    ADD COLUMN IF NOT EXISTS package_name TEXT;

-- Indexes
CREATE INDEX IF NOT EXISTS idx_tracked_software_repology_slug
    ON tracked_software (repology_slug)
    WHERE repology_slug IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_repology_aliases_slug
    ON repology_aliases (slug);

CREATE INDEX IF NOT EXISTS idx_ats_match_method
    ON agent_tracked_software (match_method)
    WHERE match_method IS NOT NULL AND match_method <> 'manual';

CREATE INDEX IF NOT EXISTS idx_ats_package_name
    ON agent_tracked_software (package_name)
    WHERE package_name IS NOT NULL;
