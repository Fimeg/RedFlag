package queries

import (
	"database/sql"
	"errors"
	"fmt"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// AgentTrackedSoftwareQueries holds the CRUD for agent_tracked_software
// (migration 039). It's split from UpstreamQueries because it crosses the
// agents <-> tracked_software boundary; keeping it separate makes the
// binding semantics easier to read.
type AgentTrackedSoftwareQueries struct {
	db *sqlx.DB
}

func NewAgentTrackedSoftwareQueries(db *sqlx.DB) *AgentTrackedSoftwareQueries {
	return &AgentTrackedSoftwareQueries{db: db}
}

// ListByAgent returns every tracked-software binding for the given agent,
// joined with its parent tracked_software row. Drift / past-EOL flags are
// computed server-side so the UI only renders.
func (q *AgentTrackedSoftwareQueries) ListByAgent(agentID uuid.UUID) ([]models.AgentTrackedSoftwareView, error) {
	var rows []models.AgentTrackedSoftwareView
	err := q.db.Select(&rows, `
		SELECT
		    b.id                AS binding_id,
		    b.agent_id          AS agent_id,
		    t.id                AS tracked_software_id,
		    t.name              AS name,
		    t.ecosystem         AS ecosystem,
		    t.source            AS source,
		    t.source_ref        AS source_ref,
		    b.installed_version AS installed_version,
		    t.latest_version    AS latest_version,
		    t.latest_at         AS latest_at,
		    t.eol_at            AS eol_at,
		    b.install_path      AS install_path,
		    b.notes             AS notes,
		    b.last_observed_at  AS last_observed_at,
		    t.last_synced_at    AS last_synced_at,
		    t.last_error        AS last_error,
		    (t.latest_version IS NOT NULL
		         AND b.installed_version <> t.latest_version) AS drifted,
		    (t.eol_at IS NOT NULL AND t.eol_at < NOW())       AS past_eol
		FROM agent_tracked_software b
		JOIN tracked_software t ON t.id = b.tracked_software_id
		WHERE b.agent_id = $1
		ORDER BY past_eol DESC, drifted DESC, t.name`, agentID)
	if err != nil {
		return nil, fmt.Errorf("agent_tracked_software: list_by_agent: %w", err)
	}
	return rows, nil
}

// ListInstallations returns the agents that have a binding for the given
// tracked_software_id. Symmetric to ListByAgent; used by the settings page.
func (q *AgentTrackedSoftwareQueries) ListInstallations(softwareID uuid.UUID) ([]models.AgentInstallation, error) {
	var rows []models.AgentInstallation
	err := q.db.Select(&rows, `
		SELECT
		    b.id                AS binding_id,
		    a.id                AS agent_id,
		    a.hostname          AS hostname,
		    b.installed_version AS installed_version,
		    b.install_path      AS install_path,
		    b.last_observed_at  AS last_observed_at
		FROM agent_tracked_software b
		JOIN agents a ON a.id = b.agent_id
		WHERE b.tracked_software_id = $1
		ORDER BY a.hostname`, softwareID)
	if err != nil {
		return nil, fmt.Errorf("agent_tracked_software: list_installations: %w", err)
	}
	return rows, nil
}

// Upsert creates a binding or updates the existing one for (agent, software).
// Idempotent — running the same input twice leaves the same row, with
// last_observed_at + updated_at refreshed. Returns the canonical row.
func (q *AgentTrackedSoftwareQueries) Upsert(agentID uuid.UUID, in models.AgentTrackedSoftwareInput) (*models.AgentTrackedSoftware, error) {
	var row models.AgentTrackedSoftware
	err := q.db.Get(&row, `
		INSERT INTO agent_tracked_software
		    (agent_id, tracked_software_id, installed_version, install_path, notes, last_observed_at)
		VALUES ($1, $2, $3, $4, $5, NOW())
		ON CONFLICT (agent_id, tracked_software_id) DO UPDATE
		    SET installed_version = EXCLUDED.installed_version,
		        install_path      = EXCLUDED.install_path,
		        notes             = EXCLUDED.notes,
		        last_observed_at  = NOW(),
		        updated_at        = NOW()
		RETURNING *`,
		agentID, in.TrackedSoftwareID, in.InstalledVersion, in.InstallPath, in.Notes)
	if err != nil {
		return nil, fmt.Errorf("agent_tracked_software: upsert: %w", err)
	}
	return &row, nil
}

// Delete removes a binding by id, scoped to the agent so a stray bindingID
// from another agent can't be deleted by manipulating only the URL.
// Returns sql.ErrNoRows if nothing matched (handler renders 404).
func (q *AgentTrackedSoftwareQueries) Delete(agentID, bindingID uuid.UUID) error {
	res, err := q.db.Exec(`
		DELETE FROM agent_tracked_software
		WHERE id = $1 AND agent_id = $2`, bindingID, agentID)
	if err != nil {
		return fmt.Errorf("agent_tracked_software: delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// GetBinding fetches a single binding row by (agent, binding) for use by
// callers that need to look up tracked_software_id after a delete (to
// trigger RecomputeCurrentVersion). Returns sql.ErrNoRows if absent.
func (q *AgentTrackedSoftwareQueries) GetBinding(agentID, bindingID uuid.UUID) (*models.AgentTrackedSoftware, error) {
	var row models.AgentTrackedSoftware
	err := q.db.Get(&row, `
		SELECT * FROM agent_tracked_software
		WHERE id = $1 AND agent_id = $2`, bindingID, agentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, sql.ErrNoRows
		}
		return nil, fmt.Errorf("agent_tracked_software: get_binding: %w", err)
	}
	return &row, nil
}

// RecomputeCurrentVersion picks the most-recently-observed binding's
// installed_version and writes it to tracked_software.current_version.
// Idempotent and no-op when no bindings exist (leaves operator-supplied
// current_version alone so tracking-only entries still work).
//
// Semantic choice note: "most recently observed" reflects what the operator
// most recently reported as deployed. A future variant could pick MIN by
// semver-aware compare to surface "worst-case drift across the fleet" —
// for now, per-agent rows in the binding view already show that detail.
func (q *AgentTrackedSoftwareQueries) RecomputeCurrentVersion(softwareID uuid.UUID) error {
	_, err := q.db.Exec(`
		UPDATE tracked_software t
		SET current_version = b.installed_version,
		    updated_at      = NOW()
		FROM (
		    SELECT installed_version
		    FROM agent_tracked_software
		    WHERE tracked_software_id = $1
		    ORDER BY last_observed_at DESC
		    LIMIT 1
		) AS b
		WHERE t.id = $1`, softwareID)
	if err != nil {
		return fmt.Errorf("agent_tracked_software: recompute_current_version: %w", err)
	}
	return nil
}

// UpsertReconciled creates or updates a binding with reconciliation metadata.
// Preserves manual bindings (match_method='manual') — automated reconciliation
// never overwrites operator-created bindings.
func (q *AgentTrackedSoftwareQueries) UpsertReconciled(agentID uuid.UUID, in models.AgentTrackedSoftwareInput, matchMethod, packageName string) (*models.AgentTrackedSoftware, error) {
	var row models.AgentTrackedSoftware
	err := q.db.Get(&row, `
		INSERT INTO agent_tracked_software
		    (agent_id, tracked_software_id, installed_version, install_path, notes, match_method, package_name, last_observed_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, NOW())
		ON CONFLICT (agent_id, tracked_software_id) DO UPDATE
		    SET installed_version = EXCLUDED.installed_version,
		        install_path      = EXCLUDED.install_path,
		        notes             = COALESCE(EXCLUDED.notes, agent_tracked_software.notes),
		        match_method      = EXCLUDED.match_method,
		        package_name      = EXCLUDED.package_name,
		        last_observed_at  = NOW(),
		        updated_at        = NOW()
		WHERE agent_tracked_software.match_method IS NULL
		   OR agent_tracked_software.match_method <> 'manual'
		RETURNING *`,
		agentID, in.TrackedSoftwareID, in.InstalledVersion, in.InstallPath, in.Notes, matchMethod, packageName)
	if err != nil {
		return nil, fmt.Errorf("agent_tracked_software: upsert_reconciled: %w", err)
	}
	return &row, nil
}
