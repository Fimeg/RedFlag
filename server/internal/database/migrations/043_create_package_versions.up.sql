-- Package version timeline (catalog). Accumulates every (package_type, name, version)
-- RedFlag has ever seen across the fleet, with the provenance it collected: when the
-- version was first scanned and last seen, its upstream publish date, the artifact
-- SHA256, and its OSV posture. This is the historical surface the package detail pane
-- renders and the query surface the as-of-date closure resolution reads — "what was
-- the newest clean version >= N days old."
--
-- One row per (package_type, package_name, version) regardless of how many agents run
-- it; the per-host state stays in current_package_state. Enrichment (publish date /
-- hash / OSV) is folded in over time via idempotent upsert, so re-running a scan or
-- approval never duplicates a version or clobbers known data with nulls.
CREATE TABLE IF NOT EXISTS package_versions (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    package_type     VARCHAR(32) NOT NULL,
    package_name     TEXT NOT NULL,
    version          TEXT NOT NULL,
    published_at     TIMESTAMPTZ,
    first_scanned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    sha256           TEXT,
    osv_status       VARCHAR(16),            -- 'clean' | 'vulnerable' | 'unknown'
    osv_vulns        JSONB,
    source           TEXT,
    CONSTRAINT package_versions_unique UNIQUE (package_type, package_name, version)
);

CREATE INDEX IF NOT EXISTS idx_package_versions_pkg ON package_versions(package_type, package_name);
CREATE INDEX IF NOT EXISTS idx_package_versions_published ON package_versions(package_type, package_name, published_at);
