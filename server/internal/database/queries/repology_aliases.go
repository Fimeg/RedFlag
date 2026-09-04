package queries

import (
	"fmt"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/jmoiron/sqlx"
	"github.com/lib/pq"
)

// RepologyQueries manages the repology_aliases cache table.
type RepologyQueries struct {
	db *sqlx.DB
}

func NewRepologyQueries(db *sqlx.DB) *RepologyQueries {
	return &RepologyQueries{db: db}
}

// UpsertRepologyAlias inserts or updates a cached alias row.
func (q *RepologyQueries) UpsertRepologyAlias(slug, ecosystem string, pkgNames []string, repoRaw *string) error {
	_, err := q.db.Exec(`
		INSERT INTO repology_aliases (slug, ecosystem, pkg_names, repo_raw, fetched_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (slug, ecosystem) DO UPDATE
		    SET pkg_names  = EXCLUDED.pkg_names,
		        repo_raw   = EXCLUDED.repo_raw,
		        fetched_at = NOW()`,
		slug, ecosystem, pq.Array(pkgNames), repoRaw)
	if err != nil {
		return fmt.Errorf("repology_aliases: upsert: %w", err)
	}
	return nil
}

// GetAliases returns every cached alias row for a given slug.
func (q *RepologyQueries) GetAliases(slug string) ([]models.RepologyAlias, error) {
	var rows []models.RepologyAlias
	err := q.db.Select(&rows, `SELECT * FROM repology_aliases WHERE slug = $1 ORDER BY ecosystem`, slug)
	if err != nil {
		return nil, fmt.Errorf("repology_aliases: get_aliases: %w", err)
	}
	return rows, nil
}

// GetAliasesByEcosystem returns the package name list for a specific (slug, ecosystem).
func (q *RepologyQueries) GetAliasesByEcosystem(slug, ecosystem string) ([]string, error) {
	var names []string
	err := q.db.QueryRow(`SELECT pkg_names FROM repology_aliases WHERE slug = $1 AND ecosystem = $2`, slug, ecosystem).Scan(pq.Array(&names))
	if err != nil {
		return nil, fmt.Errorf("repology_aliases: get_by_ecosystem: %w", err)
	}
	return names, nil
}

// GetStaleSlugs returns slugs whose most recent fetched_at is older than staleAfter.
func (q *RepologyQueries) GetStaleSlugs(staleAfter time.Duration, limit int) ([]string, error) {
	if limit <= 0 {
		limit = 50
	}
	cutoff := time.Now().UTC().Add(-staleAfter)
	var slugs []string
	err := q.db.Select(&slugs, `
		SELECT DISTINCT slug FROM repology_aliases
		WHERE fetched_at < $1
		ORDER BY fetched_at
		LIMIT $2`, cutoff, limit)
	if err != nil {
		return nil, fmt.Errorf("repology_aliases: get_stale_slugs: %w", err)
	}
	return slugs, nil
}
