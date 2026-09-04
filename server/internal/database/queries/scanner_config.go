package queries

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"
)

// ScannerConfigQueries handles scanner timeout configuration in database
type ScannerConfigQueries struct {
	db *sqlx.DB
}

// NewScannerConfigQueries creates new scanner config queries
func NewScannerConfigQueries(db *sqlx.DB) *ScannerConfigQueries {
	return &ScannerConfigQueries{db: db}
}

// ScannerTimeoutConfig represents a scanner timeout configuration
type ScannerTimeoutConfig struct {
	ScannerName string        `db:"scanner_name" json:"scanner_name"`
	TimeoutMs   int           `db:"timeout_ms" json:"timeout_ms"`
	UpdatedAt   time.Time     `db:"updated_at" json:"updated_at"`
}

// UpsertScannerConfig inserts or updates scanner timeout configuration
func (q *ScannerConfigQueries) UpsertScannerConfig(scannerName string, timeout time.Duration) error {
	if q.db == nil {
		return fmt.Errorf("database connection not available")
	}

	query := `
		INSERT INTO scanner_config (scanner_name, timeout_ms, updated_at)
		VALUES ($1, $2, CURRENT_TIMESTAMP)
		ON CONFLICT (scanner_name)
		DO UPDATE SET
			timeout_ms = EXCLUDED.timeout_ms,
			updated_at = CURRENT_TIMESTAMP
	`

	_, err := q.db.Exec(query, scannerName, timeout.Milliseconds())
	if err != nil {
		return fmt.Errorf("failed to upsert scanner config: %w", err)
	}

	return nil
}

// GetScannerConfig retrieves scanner timeout configuration for a specific scanner
func (q *ScannerConfigQueries) GetScannerConfig(scannerName string) (*ScannerTimeoutConfig, error) {
	if q.db == nil {
		return nil, fmt.Errorf("database connection not available")
	}

	var config ScannerTimeoutConfig
	query := `SELECT scanner_name, timeout_ms, updated_at FROM scanner_config WHERE scanner_name = $1`
	
	err := q.db.Get(&config, query, scannerName)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil // Return nil if not found
		}
		return nil, fmt.Errorf("failed to get scanner config: %w", err)
	}

	return &config, nil
}

// GetAllScannerConfigs retrieves all scanner timeout configurations
func (q *ScannerConfigQueries) GetAllScannerConfigs() (map[string]ScannerTimeoutConfig, error) {
	if q.db == nil {
		return nil, fmt.Errorf("database connection not available")
	}

	var configs []ScannerTimeoutConfig
	query := `SELECT scanner_name, timeout_ms, updated_at FROM scanner_config ORDER BY scanner_name`
	
	err := q.db.Select(&configs, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all scanner configs: %w", err)
	}

	// Convert slice to map
	configMap := make(map[string]ScannerTimeoutConfig)
	for _, cfg := range configs {
		configMap[cfg.ScannerName] = cfg
	}

	return configMap, nil
}

// DeleteScannerConfig removes scanner timeout configuration
func (q *ScannerConfigQueries) DeleteScannerConfig(scannerName string) error {
	if q.db == nil {
		return fmt.Errorf("database connection not available")
	}

	query := `DELETE FROM scanner_config WHERE scanner_name = $1`
	
	result, err := q.db.Exec(query, scannerName)
	if err != nil {
		return fmt.Errorf("failed to delete scanner config: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to verify delete: %w", err)
	}

	if rows == 0 {
		return sql.ErrNoRows
	}

	return nil
}

// GetScannerTimeoutWithDefault returns scanner timeout from DB or default value
func (q *ScannerConfigQueries) GetScannerTimeoutWithDefault(scannerName string, defaultTimeout time.Duration) time.Duration {
	config, err := q.GetScannerConfig(scannerName)
	if err != nil {
		return defaultTimeout
	}

	if config == nil {
		return defaultTimeout
	}

	return time.Duration(config.TimeoutMs) * time.Millisecond
}
