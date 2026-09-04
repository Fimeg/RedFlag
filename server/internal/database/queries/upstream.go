package queries

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

type UpstreamQueries struct {
	db *sqlx.DB
}

func NewUpstreamQueries(db *sqlx.DB) *UpstreamQueries {
	return &UpstreamQueries{db: db}
}

func (q *UpstreamQueries) List() ([]models.TrackedSoftware, error) {
	var rows []models.TrackedSoftware
	err := q.db.Select(&rows, `SELECT * FROM tracked_software ORDER BY name`)
	if err != nil {
		return nil, fmt.Errorf("upstream: list tracked_software: %w", err)
	}
	return rows, nil
}

// ListDrifted returns only rows where current_version != latest_version
// (and both are known). This is what the dashboard panel renders.
func (q *UpstreamQueries) ListDrifted() ([]models.TrackedSoftware, error) {
	var rows []models.TrackedSoftware
	err := q.db.Select(&rows, `
		SELECT * FROM tracked_software
		WHERE current_version IS NOT NULL
		  AND latest_version IS NOT NULL
		  AND current_version <> latest_version
		ORDER BY (eol_at IS NOT NULL AND eol_at < NOW()) DESC,
		         updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("upstream: list drifted: %w", err)
	}
	return rows, nil
}

// DueForSync returns enabled tracked_software rows whose last_checked_at is
// older than the given threshold (or never). Caller iterates and dispatches
// per source.
func (q *UpstreamQueries) DueForSync(staleAfter time.Duration, limit int) ([]models.TrackedSoftware, error) {
	if limit <= 0 {
		limit = 50
	}
	cutoff := time.Now().UTC().Add(-staleAfter)
	var rows []models.TrackedSoftware
	err := q.db.Select(&rows, `
		SELECT * FROM tracked_software
		WHERE enabled = TRUE
		  AND (last_checked_at IS NULL OR last_checked_at < $1)
		ORDER BY last_checked_at NULLS FIRST
		LIMIT $2`, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("upstream: due_for_sync: %w", err)
	}
	return rows, nil
}

func (q *UpstreamQueries) GetByID(id uuid.UUID) (*models.TrackedSoftware, error) {
	var row models.TrackedSoftware
	err := q.db.Get(&row, `SELECT * FROM tracked_software WHERE id = $1`, id)
	if err != nil {
		return nil, fmt.Errorf("upstream: not found: %w", err)
	}
	return &row, nil
}

func (q *UpstreamQueries) Create(in models.TrackedSoftwareInput) (*models.TrackedSoftware, error) {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	trackPre := false
	if in.TrackPrereleases != nil {
		trackPre = *in.TrackPrereleases
	}
	var row models.TrackedSoftware
	err := q.db.Get(&row, `
		INSERT INTO tracked_software
		    (name, ecosystem, source, source_ref, current_version, enabled, track_prereleases, repology_slug, container_image_pattern, binary_probe)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING *`,
		in.Name, in.Ecosystem, in.Source, in.SourceRef, in.CurrentVersion, enabled, trackPre,
		in.RepologySlug, in.ContainerImagePattern, in.BinaryProbe)
	if err != nil {
		return nil, fmt.Errorf("upstream: create: %w", err)
	}
	return &row, nil
}

func (q *UpstreamQueries) UpdateSettings(id uuid.UUID, in models.TrackedSoftwareSettingsInput) (*models.TrackedSoftware, error) {
	var row models.TrackedSoftware
	err := q.db.Get(&row, `
		UPDATE tracked_software
		SET enabled = COALESCE($2, enabled),
		    track_prereleases = COALESCE($3, track_prereleases),
		    updated_at = NOW()
		WHERE id = $1
		RETURNING *`, id, in.Enabled, in.TrackPrereleases)
	if err != nil {
		return nil, fmt.Errorf("upstream: update_settings: %w", err)
	}
	return &row, nil
}

func (q *UpstreamQueries) Delete(id uuid.UUID) error {
	res, err := q.db.Exec(`DELETE FROM tracked_software WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("upstream: delete: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("upstream: not found")
	}
	return nil
}

// ApplySyncResult writes back the result of a successful or failed fetch.
// On success: latest_version + latest_at + eol_at updated, last_error cleared.
// On failure: only last_checked_at + last_error updated (preserve stale data).
func (q *UpstreamQueries) ApplySyncResult(id uuid.UUID, latestVersion string, latestAt, eolAt *time.Time) error {
	_, err := q.db.Exec(`
		UPDATE tracked_software
		SET latest_version  = $2,
		    latest_at       = $3,
		    eol_at          = $4,
		    last_checked_at = NOW(),
		    last_synced_at  = NOW(),
		    last_error      = NULL,
		    updated_at      = NOW()
		WHERE id = $1`, id, latestVersion, latestAt, eolAt)
	if err != nil {
		return fmt.Errorf("upstream: apply_sync_result: %w", err)
	}
	return nil
}

func (q *UpstreamQueries) ApplySyncError(id uuid.UUID, errMsg string) error {
	_, err := q.db.Exec(`
		UPDATE tracked_software
		SET last_checked_at = NOW(),
		    last_error      = $2,
		    updated_at      = NOW()
		WHERE id = $1`, id, errMsg)
	if err != nil {
		return fmt.Errorf("upstream: apply_sync_error: %w", err)
	}
	return nil
}

func (q *UpstreamQueries) InsertDriftEvent(softwareID uuid.UUID, severity string, fromV, toV *string, note *string) error {
	_, err := q.db.Exec(`
		INSERT INTO upstream_drift_events (tracked_software_id, drift_severity, from_version, to_version, note)
		VALUES ($1, $2, $3, $4, $5)`,
		softwareID, severity, fromV, toV, note)
	if err != nil {
		return fmt.Errorf("upstream: insert_drift_event: %w", err)
	}
	return nil
}

// SeedSelf upserts a tracked_software row keyed on (source, source_ref). It's
// used at startup to keep RedFlag's own self-tracking entry honest across
// upgrades without clobbering operator intent: on conflict it refreshes only
// current_version (so the row reflects the running build) and updated_at. It
// deliberately does NOT touch enabled or track_prereleases on conflict — an
// operator who disabled the self-entry, or flipped its prerelease tracking,
// keeps their choice. The DEFAULTs apply only on first insert.
// Known pre-canonical self rows are folded into the canonical key first so
// upgrades do not leave duplicate RedFlag entries.
//
// Returns true when a row was inserted (no prior self-entry), false when an
// existing row was refreshed, so the caller can log the first seed once.
func (q *UpstreamQueries) SeedSelf(in models.TrackedSoftwareInput) (inserted bool, err error) {
	enabled := true
	if in.Enabled != nil {
		enabled = *in.Enabled
	}
	trackPre := false
	if in.TrackPrereleases != nil {
		trackPre = *in.TrackPrereleases
	}
	tx, err := q.db.Beginx()
	if err != nil {
		return false, fmt.Errorf("upstream: seed_self begin: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()
	if isCanonicalRedFlagSelf(in) {
		if err = normalizeRedFlagSelfRows(tx, in, trackPre); err != nil {
			return false, err
		}
	}
	// xmax = 0 on the freshly-inserted tuple, non-zero on the row touched by DO
	// UPDATE — the standard way to tell INSERT from UPDATE in an upsert RETURNING.
	row := tx.QueryRow(`
		INSERT INTO tracked_software (name, ecosystem, source, source_ref, current_version, enabled, track_prereleases)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (source, source_ref) DO UPDATE
		SET current_version = EXCLUDED.current_version,
		    updated_at      = NOW()
		RETURNING (xmax = 0) AS inserted`,
		in.Name, in.Ecosystem, in.Source, in.SourceRef, in.CurrentVersion, enabled, trackPre)
	if err = row.Scan(&inserted); err != nil {
		return false, fmt.Errorf("upstream: seed_self: %w", err)
	}
	if err = tx.Commit(); err != nil {
		return false, fmt.Errorf("upstream: seed_self commit: %w", err)
	}
	return inserted, nil
}

func isCanonicalRedFlagSelf(in models.TrackedSoftwareInput) bool {
	return strings.EqualFold(strings.TrimSpace(in.Name), "RedFlag") &&
		in.Source == "forgejo" &&
		strings.Trim(strings.TrimSpace(in.SourceRef), "/") == "codeberg.org/Fimeg/RedFlag"
}

func normalizeRedFlagSelfRows(tx *sqlx.Tx, in models.TrackedSoftwareInput, trackPre bool) error {
	var canonicalID uuid.UUID
	err := tx.QueryRow(`
		SELECT id
		FROM tracked_software
		WHERE source = 'forgejo'
		  AND source_ref = 'codeberg.org/Fimeg/RedFlag'
		LIMIT 1`).Scan(&canonicalID)
	switch {
	case err == nil:
		if _, err := tx.Exec(`
			DELETE FROM tracked_software
			WHERE `+redFlagSelfLegacyWhere()+`
			  AND id <> $1`, canonicalID); err != nil {
			return fmt.Errorf("upstream: seed_self delete legacy duplicates: %w", err)
		}
		return nil
	case err != sql.ErrNoRows:
		return fmt.Errorf("upstream: seed_self find canonical: %w", err)
	}

	var legacyID uuid.UUID
	err = tx.QueryRow(`
		SELECT id
		FROM tracked_software
		WHERE ` + redFlagSelfLegacyWhere() + `
		ORDER BY created_at
		LIMIT 1`).Scan(&legacyID)
	switch {
	case err == sql.ErrNoRows:
		return nil
	case err != nil:
		return fmt.Errorf("upstream: seed_self find legacy: %w", err)
	}

	if _, err := tx.Exec(`
		UPDATE tracked_software
		SET name = $2,
		    ecosystem = $3,
		    source = $4,
		    source_ref = $5,
		    current_version = $6,
		    track_prereleases = $7,
		    updated_at = NOW()
		WHERE id = $1`,
		legacyID, in.Name, in.Ecosystem, in.Source, in.SourceRef, in.CurrentVersion, trackPre); err != nil {
		return fmt.Errorf("upstream: seed_self canonicalize legacy: %w", err)
	}
	if _, err := tx.Exec(`
		DELETE FROM tracked_software
		WHERE `+redFlagSelfLegacyWhere()+`
		  AND id <> $1`, legacyID); err != nil {
		return fmt.Errorf("upstream: seed_self delete extra legacy rows: %w", err)
	}
	return nil
}

func redFlagSelfLegacyWhere() string {
	return `
		LOWER(name) = 'redflag'
		AND (
		    (source = 'gitea' AND source_ref IN (
		        'Fimeg/RedFlag',
		        'codeberg.org/Fimeg/RedFlag',
		        'https://codeberg.org/Fimeg/RedFlag',
		        'https://codeberg.org/Fimeg/RedFlag/'
		    ))
		    OR (source = 'forgejo' AND source_ref IN (
		        'https://codeberg.org/Fimeg/RedFlag',
		        'https://codeberg.org/Fimeg/RedFlag/'
		    ))
		)`
}

func (q *UpstreamQueries) RecentDriftEvents(limit int) ([]models.UpstreamDriftEvent, error) {
	if limit <= 0 {
		limit = 25
	}
	var rows []models.UpstreamDriftEvent
	err := q.db.Select(&rows, `
		SELECT * FROM upstream_drift_events
		ORDER BY observed_at DESC
		LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("upstream: recent_drift: %w", err)
	}
	return rows, nil
}
