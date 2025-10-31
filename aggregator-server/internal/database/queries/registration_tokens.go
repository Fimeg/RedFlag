package queries

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type RegistrationTokenQueries struct {
	db *sqlx.DB
}

type RegistrationToken struct {
	ID           uuid.UUID      `json:"id" db:"id"`
	Token        string         `json:"token" db:"token"`
	Label        *string        `json:"label" db:"label"`
	ExpiresAt    time.Time      `json:"expires_at" db:"expires_at"`
	CreatedAt    time.Time      `json:"created_at" db:"created_at"`
	UsedAt       *time.Time     `json:"used_at" db:"used_at"`
	UsedByAgentID *uuid.UUID    `json:"used_by_agent_id" db:"used_by_agent_id"`
	Revoked      bool           `json:"revoked" db:"revoked"`
	RevokedAt    *time.Time     `json:"revoked_at" db:"revoked_at"`
	RevokedReason *string       `json:"revoked_reason" db:"revoked_reason"`
	Status       string         `json:"status" db:"status"`
	CreatedBy    string         `json:"created_by" db:"created_by"`
	Metadata     json.RawMessage `json:"metadata" db:"metadata"`
	MaxSeats     int            `json:"max_seats" db:"max_seats"`
	SeatsUsed    int            `json:"seats_used" db:"seats_used"`
}

type TokenRequest struct {
	Label      string                 `json:"label"`
	ExpiresIn  string                 `json:"expires_in"` // e.g., "24h", "7d"
	MaxSeats   int                    `json:"max_seats"`  // Number of agents that can use this token (default: 1)
	Metadata   map[string]interface{} `json:"metadata"`
}

type TokenResponse struct {
	Token         string    `json:"token"`
	Label         string    `json:"label"`
	ExpiresAt     time.Time `json:"expires_at"`
	InstallCommand string   `json:"install_command"`
}

func NewRegistrationTokenQueries(db *sqlx.DB) *RegistrationTokenQueries {
	return &RegistrationTokenQueries{db: db}
}

// CreateRegistrationToken creates a new registration token with seat tracking
func (q *RegistrationTokenQueries) CreateRegistrationToken(token, label string, expiresAt time.Time, maxSeats int, metadata map[string]interface{}) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	// Ensure maxSeats is at least 1
	if maxSeats < 1 {
		maxSeats = 1
	}

	query := `
		INSERT INTO registration_tokens (token, label, expires_at, max_seats, metadata)
		VALUES ($1, $2, $3, $4, $5)
	`

	_, err = q.db.Exec(query, token, label, expiresAt, maxSeats, metadataJSON)
	if err != nil {
		return fmt.Errorf("failed to create registration token: %w", err)
	}

	return nil
}

// ValidateRegistrationToken checks if a token is valid and has available seats
func (q *RegistrationTokenQueries) ValidateRegistrationToken(token string) (*RegistrationToken, error) {
	var regToken RegistrationToken
	query := `
		SELECT id, token, label, expires_at, created_at, used_at, used_by_agent_id,
			   revoked, revoked_at, revoked_reason, status, created_by, metadata,
			   max_seats, seats_used
		FROM registration_tokens
		WHERE token = $1 AND status = 'active' AND expires_at > NOW() AND seats_used < max_seats
	`

	err := q.db.Get(&regToken, query, token)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid, expired, or seats full")
		}
		return nil, fmt.Errorf("failed to validate token: %w", err)
	}

	return &regToken, nil
}

// MarkTokenUsed marks a token as used by an agent
// With seat tracking, this increments seats_used and only marks status='used' when all seats are taken
func (q *RegistrationTokenQueries) MarkTokenUsed(token string, agentID uuid.UUID) error {
	// Call the PostgreSQL function that handles seat tracking logic
	query := `SELECT mark_registration_token_used($1, $2)`

	var success bool
	err := q.db.QueryRow(query, token, agentID).Scan(&success)
	if err != nil {
		return fmt.Errorf("failed to mark token as used: %w", err)
	}

	if !success {
		return fmt.Errorf("token not found, already used, expired, or seats full")
	}

	return nil
}

// GetActiveRegistrationTokens returns all active tokens
func (q *RegistrationTokenQueries) GetActiveRegistrationTokens() ([]RegistrationToken, error) {
	var tokens []RegistrationToken
	query := `
		SELECT id, token, label, expires_at, created_at, used_at, used_by_agent_id,
			   revoked, revoked_at, revoked_reason, status, created_by, metadata,
			   max_seats, seats_used
		FROM registration_tokens
		WHERE status = 'active'
		ORDER BY created_at DESC
	`

	err := q.db.Select(&tokens, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get active tokens: %w", err)
	}

	return tokens, nil
}

// GetAllRegistrationTokens returns all tokens with pagination
func (q *RegistrationTokenQueries) GetAllRegistrationTokens(limit, offset int) ([]RegistrationToken, error) {
	var tokens []RegistrationToken
	query := `
		SELECT id, token, label, expires_at, created_at, used_at, used_by_agent_id,
			   revoked, revoked_at, revoked_reason, status, created_by, metadata,
			   max_seats, seats_used
		FROM registration_tokens
		ORDER BY created_at DESC
		LIMIT $1 OFFSET $2
	`

	err := q.db.Select(&tokens, query, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("failed to get all tokens: %w", err)
	}

	return tokens, nil
}

// RevokeRegistrationToken revokes a token (can revoke tokens in any status)
func (q *RegistrationTokenQueries) RevokeRegistrationToken(token, reason string) error {
	query := `
		UPDATE registration_tokens
		SET status = 'revoked',
			revoked = true,
			revoked_at = NOW(),
			revoked_reason = $1
		WHERE token = $2
	`

	result, err := q.db.Exec(query, reason, token)
	if err != nil {
		return fmt.Errorf("failed to revoke token: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("token not found")
	}

	return nil
}

// DeleteRegistrationToken permanently deletes a token from the database
func (q *RegistrationTokenQueries) DeleteRegistrationToken(tokenID uuid.UUID) error {
	query := `DELETE FROM registration_tokens WHERE id = $1`

	result, err := q.db.Exec(query, tokenID)
	if err != nil {
		return fmt.Errorf("failed to delete token: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}

	if rowsAffected == 0 {
		return fmt.Errorf("token not found")
	}

	return nil
}

// CleanupExpiredTokens marks expired tokens as expired
func (q *RegistrationTokenQueries) CleanupExpiredTokens() (int, error) {
	query := `
		UPDATE registration_tokens
		SET status = 'expired',
			used_at = NOW()
		WHERE status = 'active' AND expires_at < NOW() AND used_at IS NULL
	`

	result, err := q.db.Exec(query)
	if err != nil {
		return 0, fmt.Errorf("failed to cleanup expired tokens: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("failed to get rows affected: %w", err)
	}

	return int(rowsAffected), nil
}

// GetTokenUsageStats returns statistics about token usage
func (q *RegistrationTokenQueries) GetTokenUsageStats() (map[string]int, error) {
	stats := make(map[string]int)

	query := `
		SELECT status, COUNT(*) as count
		FROM registration_tokens
		GROUP BY status
	`

	rows, err := q.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("failed to get token stats: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var status string
		var count int
		if err := rows.Scan(&status, &count); err != nil {
			return nil, fmt.Errorf("failed to scan token stats row: %w", err)
		}
		stats[status] = count
	}

	return stats, nil
}