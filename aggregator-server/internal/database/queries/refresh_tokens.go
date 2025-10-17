package queries

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type RefreshTokenQueries struct {
	db *sqlx.DB
}

func NewRefreshTokenQueries(db *sqlx.DB) *RefreshTokenQueries {
	return &RefreshTokenQueries{db: db}
}

// RefreshToken represents a refresh token in the database
type RefreshToken struct {
	ID         uuid.UUID  `db:"id"`
	AgentID    uuid.UUID  `db:"agent_id"`
	TokenHash  string     `db:"token_hash"`
	ExpiresAt  time.Time  `db:"expires_at"`
	CreatedAt  time.Time  `db:"created_at"`
	LastUsedAt *time.Time `db:"last_used_at"`
	Revoked    bool       `db:"revoked"`
}

// GenerateRefreshToken creates a cryptographically secure random token
func GenerateRefreshToken() (string, error) {
	// Generate 32 bytes of random data (256 bits)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate random token: %w", err)
	}

	// Encode as hex string (64 characters)
	token := hex.EncodeToString(tokenBytes)
	return token, nil
}

// HashRefreshToken creates SHA-256 hash of the token for storage
func HashRefreshToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

// CreateRefreshToken stores a new refresh token for an agent
func (q *RefreshTokenQueries) CreateRefreshToken(agentID uuid.UUID, token string, expiresAt time.Time) error {
	tokenHash := HashRefreshToken(token)

	query := `
		INSERT INTO refresh_tokens (agent_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`

	_, err := q.db.Exec(query, agentID, tokenHash, expiresAt)
	return err
}

// ValidateRefreshToken checks if a refresh token is valid
func (q *RefreshTokenQueries) ValidateRefreshToken(agentID uuid.UUID, token string) (*RefreshToken, error) {
	tokenHash := HashRefreshToken(token)

	query := `
		SELECT id, agent_id, token_hash, expires_at, created_at, last_used_at, revoked
		FROM refresh_tokens
		WHERE agent_id = $1 AND token_hash = $2 AND NOT revoked
	`

	var refreshToken RefreshToken
	err := q.db.Get(&refreshToken, query, agentID, tokenHash)
	if err != nil {
		return nil, fmt.Errorf("refresh token not found or invalid: %w", err)
	}

	// Check if token is expired
	if time.Now().After(refreshToken.ExpiresAt) {
		return nil, fmt.Errorf("refresh token expired")
	}

	return &refreshToken, nil
}

// UpdateLastUsed updates the last_used_at timestamp for a refresh token
func (q *RefreshTokenQueries) UpdateLastUsed(tokenID uuid.UUID) error {
	query := `
		UPDATE refresh_tokens
		SET last_used_at = NOW()
		WHERE id = $1
	`

	_, err := q.db.Exec(query, tokenID)
	return err
}

// UpdateExpiration updates the refresh token expiration (for sliding window)
// Resets expiration to specified time and updates last_used_at
func (q *RefreshTokenQueries) UpdateExpiration(tokenID uuid.UUID, newExpiry time.Time) error {
	query := `
		UPDATE refresh_tokens
		SET expires_at = $1, last_used_at = NOW()
		WHERE id = $2
	`

	_, err := q.db.Exec(query, newExpiry, tokenID)
	return err
}

// RevokeRefreshToken marks a refresh token as revoked
func (q *RefreshTokenQueries) RevokeRefreshToken(agentID uuid.UUID, token string) error {
	tokenHash := HashRefreshToken(token)

	query := `
		UPDATE refresh_tokens
		SET revoked = TRUE
		WHERE agent_id = $1 AND token_hash = $2
	`

	_, err := q.db.Exec(query, agentID, tokenHash)
	return err
}

// RevokeAllAgentTokens revokes all refresh tokens for an agent
func (q *RefreshTokenQueries) RevokeAllAgentTokens(agentID uuid.UUID) error {
	query := `
		UPDATE refresh_tokens
		SET revoked = TRUE
		WHERE agent_id = $1 AND NOT revoked
	`

	_, err := q.db.Exec(query, agentID)
	return err
}

// CleanupExpiredTokens removes expired refresh tokens from the database
func (q *RefreshTokenQueries) CleanupExpiredTokens() (int64, error) {
	query := `
		DELETE FROM refresh_tokens
		WHERE expires_at < NOW() OR revoked = TRUE
	`

	result, err := q.db.Exec(query)
	if err != nil {
		return 0, err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return 0, err
	}

	return rowsAffected, nil
}

// GetActiveTokenCount returns the number of active (non-revoked, non-expired) tokens for an agent
func (q *RefreshTokenQueries) GetActiveTokenCount(agentID uuid.UUID) (int, error) {
	query := `
		SELECT COUNT(*)
		FROM refresh_tokens
		WHERE agent_id = $1 AND NOT revoked AND expires_at > NOW()
	`

	var count int
	err := q.db.Get(&count, query, agentID)
	return count, err
}
