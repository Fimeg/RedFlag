package queries

import (
	"context"
	"fmt"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// SigningKeyQueries handles database operations for signing keys
type SigningKeyQueries struct {
	db *sqlx.DB
}

// NewSigningKeyQueries creates a new SigningKeyQueries
func NewSigningKeyQueries(db *sqlx.DB) *SigningKeyQueries {
	return &SigningKeyQueries{db: db}
}

// GetPrimarySigningKey retrieves the currently active primary signing key
func (q *SigningKeyQueries) GetPrimarySigningKey(ctx context.Context) (*models.SigningKey, error) {
	var key models.SigningKey
	query := `
		SELECT id, key_id, public_key, algorithm, is_active, is_primary, created_at, deprecated_at, version
		FROM signing_keys
		WHERE is_active = true AND is_primary = true
		ORDER BY version DESC
		LIMIT 1
	`
	err := q.db.GetContext(ctx, &key, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get primary signing key: %w", err)
	}
	return &key, nil
}

// GetActiveSigningKeys retrieves all currently active signing keys
func (q *SigningKeyQueries) GetActiveSigningKeys(ctx context.Context) ([]models.SigningKey, error) {
	var keys []models.SigningKey
	query := `
		SELECT id, key_id, public_key, algorithm, is_active, is_primary, created_at, deprecated_at, version
		FROM signing_keys
		WHERE is_active = true
		ORDER BY version DESC
	`
	err := q.db.SelectContext(ctx, &keys, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get active signing keys: %w", err)
	}
	return keys, nil
}

// GetAllSigningKeys retrieves every signing key — active and deprecated.
// Returns primary first, then active by version, then deprecated by deprecated_at.
func (q *SigningKeyQueries) GetAllSigningKeys(ctx context.Context) ([]models.SigningKey, error) {
	var keys []models.SigningKey
	query := `
		SELECT id, key_id, public_key, algorithm, is_active, is_primary, created_at, deprecated_at, version
		FROM signing_keys
		ORDER BY is_primary DESC, is_active DESC, version DESC, deprecated_at DESC NULLS FIRST
	`
	err := q.db.SelectContext(ctx, &keys, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get all signing keys: %w", err)
	}
	return keys, nil
}

// InsertSigningKey inserts a new signing key record, ignoring conflicts on key_id
func (q *SigningKeyQueries) InsertSigningKey(ctx context.Context, keyID, publicKeyHex string, version int) error {
	query := `
		INSERT INTO signing_keys (id, key_id, public_key, algorithm, is_active, is_primary, created_at, version)
		VALUES (:id, :key_id, :public_key, :algorithm, :is_active, :is_primary, :created_at, :version)
		ON CONFLICT (key_id) DO NOTHING
	`
	now := time.Now().UTC()
	params := map[string]interface{}{
		"id":         uuid.Must(uuid.NewV4()),
		"key_id":     keyID,
		"public_key": publicKeyHex,
		"algorithm":  "ed25519",
		"is_active":  true,
		"is_primary": false,
		"created_at": now,
		"version":    version,
	}
	_, err := q.db.NamedExecContext(ctx, query, params)
	if err != nil {
		return fmt.Errorf("failed to insert signing key: %w", err)
	}
	return nil
}

// SetPrimaryKey atomically sets a key as primary and unsets all other primary keys
func (q *SigningKeyQueries) SetPrimaryKey(ctx context.Context, keyID string) error {
	tx, err := q.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer func() {
		if err != nil {
			_ = tx.Rollback()
		}
	}()

	// Unset all other primary keys
	_, err = tx.ExecContext(ctx, `UPDATE signing_keys SET is_primary = false WHERE is_primary = true`)
	if err != nil {
		return fmt.Errorf("failed to unset existing primary keys: %w", err)
	}

	// Set the new primary key
	result, err := tx.ExecContext(ctx,
		`UPDATE signing_keys SET is_primary = true WHERE key_id = $1`,
		keyID,
	)
	if err != nil {
		return fmt.Errorf("failed to set primary key: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		err = fmt.Errorf("key_id %q not found in signing_keys", keyID)
		return err
	}

	return tx.Commit()
}

// DeprecateKey marks a signing key as inactive and sets the deprecated_at timestamp.
// Refuses to deprecate the current primary — there must always be one active signer.
// To replace the primary, first promote a successor with SetPrimaryKey, then deprecate the old one.
func (q *SigningKeyQueries) DeprecateKey(ctx context.Context, keyID string) error {
	now := time.Now().UTC()
	query := `
		UPDATE signing_keys
		SET is_active = false, deprecated_at = $1
		WHERE key_id = $2 AND is_primary = false
	`
	result, err := q.db.ExecContext(ctx, query, now, keyID)
	if err != nil {
		return fmt.Errorf("failed to deprecate key: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check rows affected: %w", err)
	}
	if rows == 0 {
		// Either the key doesn't exist or it's the primary — distinguish with a follow-up read.
		var isPrimary bool
		err := q.db.GetContext(ctx, &isPrimary, `SELECT is_primary FROM signing_keys WHERE key_id = $1`, keyID)
		if err != nil {
			return fmt.Errorf("key_id %q not found in signing_keys", keyID)
		}
		if isPrimary {
			return fmt.Errorf("cannot deprecate primary signing key %q — promote a successor first", keyID)
		}
		return fmt.Errorf("key_id %q not affected (possibly already deprecated)", keyID)
	}
	return nil
}

// GetNextVersion returns MAX(version) + 1 from signing_keys, or 1 if the table is empty.
// Used by InitializePrimaryKey to assign a monotonically increasing version to a new key.
func (q *SigningKeyQueries) GetNextVersion(ctx context.Context) (int, error) {
	var nextVersion int
	err := q.db.GetContext(ctx, &nextVersion, `SELECT COALESCE(MAX(version), 0) + 1 FROM signing_keys`)
	if err != nil {
		return 1, fmt.Errorf("failed to query next signing key version: %w", err)
	}
	return nextVersion, nil
}

// GetKeyByID retrieves a signing key by its key_id
func (q *SigningKeyQueries) GetKeyByID(ctx context.Context, keyID string) (*models.SigningKey, error) {
	var key models.SigningKey
	query := `
		SELECT id, key_id, public_key, algorithm, is_active, is_primary, created_at, deprecated_at, version
		FROM signing_keys
		WHERE key_id = $1
		LIMIT 1
	`
	err := q.db.GetContext(ctx, &key, query, keyID)
	if err != nil {
		return nil, fmt.Errorf("failed to get signing key by id %q: %w", keyID, err)
	}
	return &key, nil
}
