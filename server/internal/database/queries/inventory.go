package queries

import (
	"fmt"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// InventoryQueries handles database operations for agent inventory.
type InventoryQueries struct {
	db *sqlx.DB
}

// NewInventoryQueries creates a new InventoryQueries instance.
func NewInventoryQueries(db *sqlx.DB) *InventoryQueries {
	return &InventoryQueries{db: db}
}

// UpsertBatch inserts or updates inventory items for an agent.
// Uses ON CONFLICT to update existing items (same agent + ecosystem + item_name).
func (q *InventoryQueries) UpsertBatch(agentID uuid.UUID, items []models.AgentInventoryItem) error {
	if len(items) == 0 {
		return nil
	}

	query := `
		INSERT INTO agent_inventory (
			agent_id, inventory_ecosystem, item_name, item_version,
			description, arch, install_time, size_bytes, vendor, metadata,
			last_seen_at, first_seen_at
		) VALUES (
			:agent_id, :inventory_ecosystem, :item_name, :item_version,
			:description, :arch, :install_time, :size_bytes, :vendor, :metadata,
			now(), now()
		)
		ON CONFLICT (agent_id, inventory_ecosystem, item_name) DO UPDATE SET
			item_version = EXCLUDED.item_version,
			description = EXCLUDED.description,
			arch = EXCLUDED.arch,
			install_time = EXCLUDED.install_time,
			size_bytes = EXCLUDED.size_bytes,
			vendor = EXCLUDED.vendor,
			metadata = EXCLUDED.metadata,
			last_seen_at = now(),
			updated_at = now()
	`

	for _, item := range items {
		item.AgentID = agentID
		if item.ID == uuid.Nil {
			item.ID = uuid.Must(uuid.NewV4())
		}
		_, err := q.db.NamedExec(query, item)
		if err != nil {
			return fmt.Errorf("failed to upsert inventory item %s: %w", item.ItemName, err)
		}
	}

	return nil
}

// GetByAgent retrieves inventory items for an agent with optional ecosystem filter.
func (q *InventoryQueries) GetByAgent(agentID uuid.UUID, ecosystem string, limit, offset int) ([]models.AgentInventoryItem, int, error) {
	// Count query
	countQuery := `SELECT COUNT(*) FROM agent_inventory WHERE agent_id = $1`
	countArgs := []interface{}{agentID}

	if ecosystem != "" {
		countQuery += ` AND inventory_ecosystem = $2`
		countArgs = append(countArgs, ecosystem)
	}

	var total int
	if err := q.db.Get(&total, countQuery, countArgs...); err != nil {
		return nil, 0, fmt.Errorf("failed to count inventory items: %w", err)
	}

	// Data query
	query := `
		SELECT id, agent_id, inventory_ecosystem, item_name, item_version,
		       description, arch, install_time, size_bytes, vendor, metadata,
		       last_seen_at, first_seen_at, created_at, updated_at
		FROM agent_inventory
		WHERE agent_id = $1
	`
	args := []interface{}{agentID}
	argIdx := 2

	if ecosystem != "" {
		query += fmt.Sprintf(` AND inventory_ecosystem = $%d`, argIdx)
		args = append(args, ecosystem)
		argIdx++
	}

	query += ` ORDER BY last_seen_at DESC`
	query += fmt.Sprintf(` LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, limit, offset)

	var items []models.AgentInventoryItem
	if err := q.db.Select(&items, query, args...); err != nil {
		return nil, 0, fmt.Errorf("failed to query inventory items: %w", err)
	}

	return items, total, nil
}

// MarkStale deletes inventory items not seen in the latest scan.
// Called after a successful scan to remove items that are no longer present.
func (q *InventoryQueries) MarkStale(agentID uuid.UUID, ecosystem string, scanTime time.Time) error {
	query := `
		DELETE FROM agent_inventory
		WHERE agent_id = $1 AND inventory_ecosystem = $2 AND last_seen_at < $3
	`
	result, err := q.db.Exec(query, agentID, ecosystem, scanTime)
	if err != nil {
		return fmt.Errorf("failed to mark stale inventory items: %w", err)
	}

	affected, _ := result.RowsAffected()
	if affected > 0 {
		// Log but don't error — stale cleanup is best-effort
		_ = affected
	}

	return nil
}

// GetFleetSummary returns a summary of inventory across all agents.
func (q *InventoryQueries) GetFleetSummary() ([]FleetInventorySummary, error) {
	query := `
		SELECT inventory_ecosystem, COUNT(DISTINCT agent_id) AS agent_count,
		       COUNT(*) AS item_count, SUM(size_bytes) AS total_size_bytes
		FROM agent_inventory
		GROUP BY inventory_ecosystem
		ORDER BY inventory_ecosystem
	`

	var summaries []FleetInventorySummary
	if err := q.db.Select(&summaries, query); err != nil {
		return nil, fmt.Errorf("failed to query fleet inventory summary: %w", err)
	}

	return summaries, nil
}

// FleetInventorySummary is a summary row for fleet-wide inventory.
type FleetInventorySummary struct {
	InventoryEcosystem string `db:"inventory_ecosystem" json:"inventory_ecosystem"`
	AgentCount         int    `db:"agent_count" json:"agent_count"`
	ItemCount          int    `db:"item_count" json:"item_count"`
	TotalSizeBytes     int64  `db:"total_size_bytes" json:"total_size_bytes"`
}
