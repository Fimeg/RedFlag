-- Migration 035: Upstream version sync subsystem
--
-- Operators who own their full stack want to compare their deployed versions
-- against canonical upstream releases (Repology, endoflife.date, npm/PyPI/etc.).
-- This is parallel to OSV.dev vulnerability tracking and package-age gating —
-- a third axis on the same supply-chain surface.
--
-- The category these tools fall under: upstream release monitoring
-- (Anitya / release-monitoring.org, Repology, nvchecker, endoflife.date).
-- We bundle our own minimal version of it via pluggable ReleaseSource
-- adapters; see server/internal/services/upstream/.

CREATE TABLE IF NOT EXISTS tracked_software (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name            TEXT NOT NULL,
    ecosystem       TEXT NOT NULL,                  -- 'system' | 'container' | 'language'
    source          TEXT NOT NULL,                  -- 'repology' | 'endoflife' | 'github' | 'npm' | 'pypi' | 'anitya'
    source_ref      TEXT NOT NULL,                  -- the source's identifier (e.g. "nginx", "postgres", "vercel/next.js")
    current_version TEXT,                           -- deployed/observed locally (nullable until first observation)
    latest_version  TEXT,                           -- canonical upstream latest (filled by syncer)
    latest_at       TIMESTAMP,                      -- when upstream published latest_version
    eol_at          TIMESTAMP,                      -- nullable; from endoflife.date when applicable
    last_checked_at TIMESTAMP,                      -- last attempted sync
    last_synced_at  TIMESTAMP,                      -- last successful sync
    last_error      TEXT,                           -- last sync error message (for visibility)
    enabled         BOOLEAN NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE (source, source_ref)
);

CREATE INDEX IF NOT EXISTS idx_tracked_software_enabled
    ON tracked_software (enabled, last_checked_at NULLS FIRST)
    WHERE enabled = TRUE;

CREATE INDEX IF NOT EXISTS idx_tracked_software_drift
    ON tracked_software (latest_version, current_version)
    WHERE current_version IS NOT NULL
      AND latest_version IS NOT NULL
      AND current_version <> latest_version;

-- Drift events are append-only. Each row records a moment when the syncer
-- observed a change in latest_version (upstream moved) so we can show a
-- timeline on the dashboard and reason about cadence.
CREATE TABLE IF NOT EXISTS upstream_drift_events (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tracked_software_id UUID NOT NULL REFERENCES tracked_software(id) ON DELETE CASCADE,
    observed_at         TIMESTAMP NOT NULL DEFAULT NOW(),
    drift_severity      TEXT NOT NULL,              -- 'minor' | 'major' | 'eol'
    from_version        TEXT,
    to_version          TEXT,
    note                TEXT
);

CREATE INDEX IF NOT EXISTS idx_upstream_drift_events_software
    ON upstream_drift_events (tracked_software_id, observed_at DESC);

CREATE INDEX IF NOT EXISTS idx_upstream_drift_events_recent
    ON upstream_drift_events (observed_at DESC);
