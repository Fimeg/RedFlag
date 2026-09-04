-- Migration 039: Agent ↔ tracked_software bindings
--
-- The upstream subsystem (migration 035) tracks software the operator cares
-- about and notices when latest_version moves. But tracked_software.current_version
-- was a free-text operator field — drift detection works in the abstract, but
-- nothing links a tracked entry to "agent X has this installed at version Y."
--
-- This table is that link. One row per (agent, tracked_software) pair, recording
-- the installed version, optional install path (binary or directory), and any
-- operator notes (build flags, custom branch, etc.). The syncer derives
-- tracked_software.current_version from the newest installed_version across
-- bindings, so drift becomes "actually deployed on at least one host falls
-- behind upstream" rather than "operator forgot to update a string."
--
-- Bindings cascade-delete on either side: removing an agent or untracking
-- software cleans up dangling rows. Drift events on tracked_software itself
-- remain (they're audit history; see migration 035).

CREATE TABLE IF NOT EXISTS agent_tracked_software (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    agent_id            UUID NOT NULL REFERENCES agents(id) ON DELETE CASCADE,
    tracked_software_id UUID NOT NULL REFERENCES tracked_software(id) ON DELETE CASCADE,
    installed_version   TEXT NOT NULL,
    install_path        TEXT,
    notes               TEXT,
    last_observed_at    TIMESTAMP NOT NULL DEFAULT NOW(),
    created_at          TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at          TIMESTAMP NOT NULL DEFAULT NOW(),
    UNIQUE (agent_id, tracked_software_id)
);

CREATE INDEX IF NOT EXISTS idx_agent_tracked_software_agent
    ON agent_tracked_software (agent_id);

CREATE INDEX IF NOT EXISTS idx_agent_tracked_software_tracked
    ON agent_tracked_software (tracked_software_id);
