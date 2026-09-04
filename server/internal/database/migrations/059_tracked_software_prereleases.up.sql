-- Migration 059: per-entry prerelease tracking for upstream sync.
--
-- Most upstream sources expose a "latest stable" shortcut that excludes
-- prereleases (Forgejo/Gitea/Codeberg's /releases/latest, GitHub's likewise).
-- For software that ships only prereleases for a stretch — RedFlag itself
-- during the alpha run, where every tag < v0.3.0 publishes as a prerelease —
-- that shortcut would freeze latest_version at the last stable (or never fill
-- it at all). track_prereleases opts a row into the full release walk so the
-- prerelease-aware adapters (forgejo) consider prereleases when picking latest.
--
-- Defaults FALSE: existing rows keep stable-only behavior; only entries that
-- explicitly opt in (or the seeded self-entry) follow prereleases.

ALTER TABLE tracked_software
    ADD COLUMN IF NOT EXISTS track_prereleases BOOLEAN NOT NULL DEFAULT FALSE;

-- Existing canonical RedFlag self rows predate this column, so the DEFAULT
-- would make them stable-only throughout alpha. Flip only the exact managed
-- self-entry during migration; runtime reconciliation preserves later operator
-- choices after this one-time upgrade.
UPDATE tracked_software
SET track_prereleases = TRUE,
    updated_at = NOW()
WHERE source = 'forgejo'
  AND source_ref = 'codeberg.org/Fimeg/RedFlag';
