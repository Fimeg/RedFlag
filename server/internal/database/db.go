package database

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
)

// Connection-pool defaults. Sized for a single well-tuned server fronting 50–200
// agents (SCALE-001 S1). The old 25/5 ceiling starved around ~30 concurrent agents
// once the scheduler workers, sweeps, syncer, reconciler, and request handlers all
// contended for the pool. All three are overridable via env for smaller/larger hosts.
const (
	defaultMaxOpenConns        = 100
	defaultMaxIdleConns        = 25
	defaultConnMaxLifetimeMins = 30
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

	// Configure connection pool (SCALE-001 S1). ConnMaxLifetime caps how long a
	// connection is reused so the pool recycles cleanly behind poolers/restarts
	// instead of holding stale handles indefinitely.
	maxOpen := envInt("REDFLAG_DB_MAX_OPEN_CONNS", defaultMaxOpenConns)
	maxIdle := envInt("REDFLAG_DB_MAX_IDLE_CONNS", defaultMaxIdleConns)
	lifetimeMins := envInt("REDFLAG_DB_CONN_MAX_LIFETIME_MINUTES", defaultConnMaxLifetimeMins)
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(time.Duration(lifetimeMins) * time.Minute)

	// Test the connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	log.Printf("[INFO] [server] [database] pool_configured max_open=%d max_idle=%d conn_max_lifetime_min=%d", maxOpen, maxIdle, lifetimeMins)
	return &DB{db}, nil
}

// envInt reads a positive integer from env, falling back to def when unset, empty,
// unparseable, or non-positive. A non-positive override is ignored so a stray
// "0"/"-1" can't silently disable the pool ceiling.
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		log.Printf("[WARN] [server] [database] invalid_env key=%s value=%q falling_back=%d", key, v, def)
		return def
	}
	return n
}

// Migrate runs database migrations with proper tracking and safety
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

	// ISSUE-001: Create pre-migration backup point for recovery
	if _, err := db.Exec("CREATE TABLE IF NOT EXISTS schema_migrations_backup AS SELECT * FROM schema_migrations WHERE false"); err != nil {
		log.Printf("[WARN] [server] [database] backup_table_creation_failed: %v", err)
		// Non-fatal: continue without backup capability
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
			log.Printf("[INFO] [server] [database] migration_skipped version=%s already_applied=true", filename)
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
			// Rollback the failed transaction
			tx.Rollback()

			// Check if it's an "already exists" error - this indicates the migration
			// may have been partially applied. We need to verify state before deciding.
			if strings.Contains(err.Error(), "already exists") ||
			   strings.Contains(err.Error(), "duplicate key") {

				// Check if this migration was already recorded as applied
				var count int
				checkErr := db.Get(&count, "SELECT COUNT(*) FROM schema_migrations WHERE version = $1", filename)
				if checkErr == nil && count > 0 {
					// Migration was recorded but execution failed - this is a CONSISTENCY ERROR
					// The database state may not match what the migration intended
					log.Printf("[ERROR] [server] [database] migration_inconsistent version=%s error=%q", filename, err)
					return fmt.Errorf("migration %s recorded as applied but execution failed with 'already exists': database may be in inconsistent state. Manual intervention required: %w", filename, err)
				}
				// Migration not recorded and failed - this is a normal error
				return fmt.Errorf("migration %s failed: %w", filename, err)
			}

			// For any other error, fail
			return fmt.Errorf("failed to execute migration %s: %w", filename, err)
		}

		// Record the migration as applied (normal success path)
		if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES ($1)", filename); err != nil {
			tx.Rollback()
			return fmt.Errorf("failed to record migration %s: %w", filename, err)
		}

		// Commit the transaction
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("failed to commit migration %s: %w", filename, err)
		}

		log.Printf("[INFO] [server] [database] migration_applied version=%s", filename)
	}

	return nil
}

// MigrationHealth checks for inconsistent migration state (ISSUE-001)
func (db *DB) MigrationHealth() (map[string]interface{}, error) {
	health := map[string]interface{}{
		"status": "healthy",
		"issues": []string{},
	}

	// Check for duplicate migration versions (should never happen with PK, but verify)
	var duplicates []string
	err := db.Select(&duplicates, `
		SELECT version FROM schema_migrations 
		GROUP BY version HAVING COUNT(*) > 1
	`)
	if err != nil {
		health["status"] = "error"
		health["issues"] = append(health["issues"].([]string), fmt.Sprintf("duplicate_check_failed: %v", err))
	} else if len(duplicates) > 0 {
		health["status"] = "critical"
		health["issues"] = append(health["issues"].([]string), fmt.Sprintf("duplicate_versions: %v", duplicates))
	}

	// Check for migrations applied out of order
	var versions []string
	err = db.Select(&versions, `SELECT version FROM schema_migrations ORDER BY applied_at`)
	if err != nil {
		health["status"] = "error"
		health["issues"] = append(health["issues"].([]string), fmt.Sprintf("version_check_failed: %v", err))
	} else {
		// Verify versions are sorted lexicographically (migration naming convention)
		for i := 1; i < len(versions); i++ {
			if versions[i] < versions[i-1] {
				health["status"] = "warning"
				health["issues"] = append(health["issues"].([]string), 
					fmt.Sprintf("out_of_order: %s applied before %s", versions[i], versions[i-1]))
			}
		}
	}

	return health, nil
}

// Close closes the database connection
func (db *DB) Close() error {
	return db.DB.Close()
}
