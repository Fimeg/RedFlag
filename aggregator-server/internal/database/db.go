package database

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// DB wraps the database connection
type DB struct {
	*sqlx.DB
}

// Connect establishes a connection to the PostgreSQL database
func Connect(databaseURL string) (*DB, error) {
	db, err := sqlx.Connect("postgres", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Configure connection pool
	db.SetMaxOpenConns(25)
	db.SetMaxIdleConns(5)

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return &DB{db}, nil
}

// Migrate runs database migrations with proper tracking
func (db *DB) Migrate(migrationsPath string) error {
	// Create migrations table if it doesn't exist
	createTableSQL := `
	CREATE TABLE IF NOT EXISTS schema_migrations (
		version VARCHAR(255) PRIMARY KEY,
		applied_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
	)`
	if _, err := db.Exec(createTableSQL); err != nil {
		return fmt.Errorf("failed to create migrations table: %w", err)
	}

	// Read migration files
	files, err := os.ReadDir(migrationsPath)
	if err != nil {
		return fmt.Errorf("failed to read migrations directory: %w", err)
	}

	// Filter and sort .up.sql files
	var migrationFiles []string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".up.sql") {
			migrationFiles = append(migrationFiles, file.Name())
		}
	}
	sort.Strings(migrationFiles)

	// Execute migrations that haven't been applied yet
	for _, filename := range migrationFiles {
		// Check if migration has already been applied
		var count int
		err := db.Get(&count, "SELECT COUNT(*) FROM schema_migrations WHERE version = $1", filename)
		if err != nil {
			return fmt.Errorf("failed to check migration status for %s: %w", filename, err)
		}

		if count > 0 {
			fmt.Printf("→ Skipping migration (already applied): %s\n", filename)
			continue
		}

		// Read migration file
		path := filepath.Join(migrationsPath, filename)
		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("failed to read migration %s: %w", filename, err)
		}

		// Execute migration in a transaction
		tx, err := db.Beginx()
		if err != nil {
			return fmt.Errorf("failed to begin transaction for migration %s: %w", filename, err)
		}

		// Execute the migration SQL
		if _, err := tx.Exec(string(content)); err != nil {
			// Check if it's a "already exists" error - if so, handle gracefully
			if strings.Contains(err.Error(), "already exists") ||
			   strings.Contains(err.Error(), "duplicate key") ||
			   strings.Contains(err.Error(), "relation") && strings.Contains(err.Error(), "already exists") {
				fmt.Printf("⚠ Migration %s failed (objects already exist), marking as applied: %v\n", filename, err)
				// Rollback current transaction and start a new one for tracking
				tx.Rollback()
				// Start new transaction just for migration tracking
				if newTx, newTxErr := db.Beginx(); newTxErr == nil {
					if _, insertErr := newTx.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", filename); insertErr == nil {
						newTx.Commit()
					} else {
						newTx.Rollback()
					}
				}
				continue
			}
			tx.Rollback()
			return fmt.Errorf("failed to execute migration %s: %w", filename, err)
		}

		// Record the migration as applied
		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", filename); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %s: %w", filename, err)
		}

		// Commit the transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", filename, err)
		}

		fmt.Printf("✓ Executed migration: %s\n", filename)
	}

	return nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}
