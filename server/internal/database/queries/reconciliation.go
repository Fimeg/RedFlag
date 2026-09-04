package queries

import (
	"fmt"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// ReconciliationQueries finds tracked_software entries that match agent-reported packages.
type ReconciliationQueries struct {
	db *sqlx.DB
}

func NewReconciliationQueries(db *sqlx.DB) *ReconciliationQueries {
	return &ReconciliationQueries{db: db}
}

// MatchByRepology finds agent packages whose name appears in the repology_aliases
// for a given slug. Returns matches where no manual binding already exists.
func (q *ReconciliationQueries) MatchByRepology(slug string, aliases []string) ([]models.MatchResult, error) {
	if len(aliases) == 0 {
		return nil, nil
	}
	var rows []models.MatchResult
	err := q.db.Select(&rows, `
		SELECT DISTINCT
		    u.agent_id,
		    t.id AS tracked_software_id,
		    u.package_name,
		    u.current_version AS version,
		    u.package_type
		FROM current_package_state u
		JOIN tracked_software t ON t.repology_slug = $1
		LEFT JOIN agent_tracked_software ats
		    ON ats.agent_id = u.agent_id
		    AND ats.tracked_software_id = t.id
		    AND ats.match_method = 'manual'
		WHERE u.package_name = ANY($2)
		  AND ats.id IS NULL
		ORDER BY u.agent_id, t.id`,
		slug, pq.Array(aliases))
	if err != nil {
		return nil, fmt.Errorf("reconciliation: match_by_repology: %w", err)
	}
	return rows, nil
}

// MatchByContainer finds tracked_software entries with a container_image_pattern
// that matches an agent-reported docker image name.
func (q *ReconciliationQueries) MatchByContainer(pattern string) ([]models.MatchResult, error) {
	if pattern == "" {
		return nil, nil
	}
	var rows []models.MatchResult
	err := q.db.Select(&rows, `
		SELECT DISTINCT
		    di.agent_id,
		    t.id AS tracked_software_id,
		    di.package_name,
		    di.current_version AS version,
		    'docker' AS package_type
		FROM docker_images di
		JOIN tracked_software t
		    ON t.container_image_pattern IS NOT NULL
		    AND (di.package_name ILIKE REPLACE(REPLACE(t.container_image_pattern, '%', '\\%'), '_', '\\_') || ':%' ESCAPE '\\'
		         OR di.package_name = t.container_image_pattern)
		LEFT JOIN agent_tracked_software ats
		    ON ats.agent_id = di.agent_id
		    AND ats.tracked_software_id = t.id
		    AND ats.match_method = 'manual'
		WHERE t.container_image_pattern = $1
		  AND ats.id IS NULL
		ORDER BY di.agent_id, t.id`,
		pattern)
	if err != nil {
		return nil, fmt.Errorf("reconciliation: match_by_container: %w", err)
	}
	return rows, nil
}

// MatchByExactName finds tracked_software where source_ref matches the
// agent-reported package name exactly.
func (q *ReconciliationQueries) MatchByExactName(sourceRef string) ([]models.MatchResult, error) {
	if sourceRef == "" {
		return nil, nil
	}
	var rows []models.MatchResult
	err := q.db.Select(&rows, `
		SELECT DISTINCT
		    u.agent_id,
		    t.id AS tracked_software_id,
		    u.package_name,
		    u.current_version AS version,
		    u.package_type
		FROM current_package_state u
		JOIN tracked_software t ON t.source_ref = u.package_name
		LEFT JOIN agent_tracked_software ats
		    ON ats.agent_id = u.agent_id
		    AND ats.tracked_software_id = t.id
		    AND ats.match_method = 'manual'
		WHERE u.package_name = $1
		  AND ats.id IS NULL
		ORDER BY u.agent_id, t.id`,
		sourceRef)
	if err != nil {
		return nil, fmt.Errorf("reconciliation: match_by_exact_name: %w", err)
	}
	return rows, nil
}
