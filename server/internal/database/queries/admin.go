package queries

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/alexedwards/argon2id"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

type AdminQueries struct {
	db *sqlx.DB
}

func NewAdminQueries(db *sqlx.DB) *AdminQueries {
	return &AdminQueries{db: db}
}

type Admin struct {
	ID        uuid.UUID `json:"id"`
	Username  string    `json:"username"`
	Email     string    `json:"email"`
	Password  string    `json:"-"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateAdminIfNotExists creates an admin user if they don't already exist
func (q *AdminQueries) CreateAdminIfNotExists(username, email, password string) error {
	ctx := context.Background()

	// Check if admin already exists
	var exists bool
	err := q.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM users WHERE username = $1)", username).Scan(&exists)
	if err != nil {
		return fmt.Errorf("failed to check if admin exists: %w", err)
	}

	if exists {
		return nil // Admin already exists, nothing to do
	}

	// Hash the password
	hashedPassword, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Create the admin
	query := `
		INSERT INTO users (username, email, password_hash, created_at)
		VALUES ($1, $2, $3, NOW())
	`
	_, err = q.db.ExecContext(ctx, query, username, email, hashedPassword)
	if err != nil {
		return fmt.Errorf("failed to create admin: %w", err)
	}

	return nil
}

// UpdateAdminPassword updates the admin's password (always updates from .env)
func (q *AdminQueries) UpdateAdminPassword(username, password string) error {
	ctx := context.Background()

	// Hash the password
	hashedPassword, err := argon2id.CreateHash(password, argon2id.DefaultParams)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	// Update the password
	query := `
		UPDATE users
		SET password_hash = $1
		WHERE username = $2
	`
	result, err := q.db.ExecContext(ctx, query, hashedPassword, username)
	if err != nil {
		return fmt.Errorf("failed to update admin password: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("admin not found")
	}

	return nil
}

// VerifyAdminCredentials validates username and password against the database hash
func (q *AdminQueries) VerifyAdminCredentials(username, password string) (*Admin, error) {
	ctx := context.Background()

	var admin Admin
	query := `
		SELECT id, username, email, password_hash, created_at
		FROM users
		WHERE username = $1
	`

	err := q.db.QueryRowContext(ctx, query, username).Scan(
		&admin.ID,
		&admin.Username,
		&admin.Email,
		&admin.Password,
		&admin.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("admin not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query admin: %w", err)
	}

	// Verify the password
	match, err := argon2id.ComparePasswordAndHash(password, admin.Password)
	if err != nil {
		return nil, fmt.Errorf("failed to compare password: %w", err)
	}

	if !match {
		return nil, fmt.Errorf("invalid credentials")
	}

	return &admin, nil
}

// GetAdminByUsername retrieves admin by username (for JWT claims)
func (q *AdminQueries) GetAdminByUsername(username string) (*Admin, error) {
	ctx := context.Background()

	var admin Admin
	query := `
		SELECT id, username, email, created_at
		FROM users
		WHERE username = $1
	`

	err := q.db.QueryRowContext(ctx, query, username).Scan(
		&admin.ID,
		&admin.Username,
		&admin.Email,
		&admin.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("admin not found")
	}
	if err != nil {
		return nil, fmt.Errorf("failed to query admin: %w", err)
	}

	return &admin, nil
}