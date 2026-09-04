package queries

import (
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/crypto"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// HashRegistrationToken creates a SHA-256 hash of a registration token for storage.
func HashRegistrationToken(token string) string {
	hash := sha256.Sum256([]byte(token))
	return hex.EncodeToString(hash[:])
}

type RegistrationTokenQueries struct {
	db *sqlx.DB
	// encKey is the raw AES-256 key (decoded from the config secret rail) used to
	// encrypt token plaintext at rest and decrypt it for live tokens. May be nil
	// if no key was provisioned, in which case plaintext retrieval is disabled.
	encKey []byte
}

type RegistrationToken struct {
	ID           uuid.UUID      `json:"id" db:"id"`
	TokenHash    string         `json:"-" db:"token_hash"`
	// TokenEncrypted is the AES-256-GCM ciphertext (nonce||ct) of the plaintext
	// token. Never serialised; decrypted into Token for live tokens only.
	TokenEncrypted []byte       `json:"-" db:"token_encrypted"`
	// Token is the decrypted plaintext, populated only for live tokens so the
	// operator UI can rebuild the install one-liner. Empty for spent/expired/
	// revoked tokens and for legacy one-way-hashed tokens.
	Token        string         `json:"token,omitempty" db:"-"`
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
	// TOTPSeedEncrypted is the TOTP seed for fleet-join 2FA (SEC-025), stored
	// as AES-256-GCM ciphertext (nonce||ct) of the base32 seed. Nil for
	// standard registration tokens; set only when the operator creates a
	// fleet-join token that requires the standalone host to prove possession
	// of the seed via a TOTP code. Never serialised; decrypted only at join
	// validation. The join request carries the code, never the seed.
	TOTPSeedEncrypted []byte    `json:"-" db:"totp_seed_encrypted"`
}

// isLive reports whether a token is still usable for enrollment — the only state
// in which its plaintext may be revealed to rebuild the install one-liner.
func (t *RegistrationToken) isLive() bool {
	return t.Status == "active" &&
		t.ExpiresAt.After(time.Now()) &&
		t.SeatsUsed < t.MaxSeats
}

// revealPlaintext decrypts TokenEncrypted into Token when the token is live and a
// key is configured. Best-effort: a decrypt failure (e.g. key rotated, legacy
// row) leaves Token empty rather than erroring the whole listing.
func (q *RegistrationTokenQueries) revealPlaintext(t *RegistrationToken) {
	if len(q.encKey) == 0 || len(t.TokenEncrypted) == 0 || !t.isLive() {
		return
	}
	if plaintext, err := crypto.Decrypt(q.encKey, t.TokenEncrypted); err == nil {
		t.Token = string(plaintext)
	}
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

// NewRegistrationTokenQueries builds the query handle. encKeyB64 is the base64
// AES-256 key from the config secret rail; if empty, token plaintext is neither
// stored nor revealed (validation by hash still works).
func NewRegistrationTokenQueries(db *sqlx.DB, encKeyB64 string) (*RegistrationTokenQueries, error) {
	q := &RegistrationTokenQueries{db: db}
	if encKeyB64 != "" {
		key, err := base64.StdEncoding.DecodeString(encKeyB64)
		if err != nil {
			return nil, fmt.Errorf("invalid token encryption key format: %w", err)
		}
		if len(key) != crypto.KeySize {
			return nil, fmt.Errorf("token encryption key must be %d bytes, got %d", crypto.KeySize, len(key))
		}
		q.encKey = key
	}
	return q, nil
}

// CreateRegistrationToken creates a new registration token with seat tracking.
// The caller passes the plaintext token; the SHA-256 hash is stored for
// validation and (when a key is configured) the reversible ciphertext is stored
// for one-liner rebuilds.
//
// totpSeed is optional (SEC-025 fleet-join 2FA). When non-empty, the seed is
// stored AES-256-GCM encrypted and the token requires a TOTP code at
// fleet-join time. The seed must be retrievable to verify codes, so creating
// a fleet-join token without an encryption key configured is an error —
// fail closed rather than store a seed we can never use or one in plaintext.
func (q *RegistrationTokenQueries) CreateRegistrationToken(token, label string, expiresAt time.Time, maxSeats int, metadata map[string]interface{}, totpSeed string) error {
	metadataJSON, err := json.Marshal(metadata)
	if err != nil {
		return fmt.Errorf("failed to marshal metadata: %w", err)
	}

	if maxSeats < 1 {
		maxSeats = 1
	}

	var encrypted []byte
	if len(q.encKey) > 0 {
		encrypted, err = crypto.Encrypt(q.encKey, []byte(token))
		if err != nil {
			return fmt.Errorf("failed to encrypt registration token: %w", err)
		}
	}

	var totpSeedEncrypted []byte
	if totpSeed != "" {
		if len(q.encKey) == 0 {
			return fmt.Errorf("fleet-join tokens require the settings encryption key (TOTP seed must be stored encrypted)")
		}
		totpSeedEncrypted, err = crypto.Encrypt(q.encKey, []byte(totpSeed))
		if err != nil {
			return fmt.Errorf("failed to encrypt TOTP seed: %w", err)
		}
	}

	tokenHash := HashRegistrationToken(token)
	query := `
		INSERT INTO registration_tokens (token_hash, token_encrypted, label, expires_at, max_seats, metadata, totp_seed_encrypted)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err = q.db.Exec(query, tokenHash, encrypted, label, expiresAt, maxSeats, metadataJSON, totpSeedEncrypted)
	if err != nil {
		return fmt.Errorf("failed to create registration token: %w", err)
	}

	return nil
}

// DecryptTOTPSeed returns the plaintext TOTP seed for a fleet-join token.
// Errors when the token has no seed or no encryption key is configured.
func (q *RegistrationTokenQueries) DecryptTOTPSeed(t *RegistrationToken) (string, error) {
	if len(t.TOTPSeedEncrypted) == 0 {
		return "", fmt.Errorf("token has no TOTP seed")
	}
	if len(q.encKey) == 0 {
		return "", fmt.Errorf("no encryption key configured")
	}
	plaintext, err := crypto.Decrypt(q.encKey, t.TOTPSeedEncrypted)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt TOTP seed: %w", err)
	}
	return string(plaintext), nil
}

// ValidateRegistrationToken checks if a token is valid and has available seats.
// The caller passes the plaintext token; this function hashes it before querying.
func (q *RegistrationTokenQueries) ValidateRegistrationToken(token string) (*RegistrationToken, error) {
	var regToken RegistrationToken
	tokenHash := HashRegistrationToken(token)
	query := `
		SELECT id, token_hash, label, expires_at, created_at, used_at, used_by_agent_id,
			   revoked, revoked_at, revoked_reason, status, created_by, metadata,
			   max_seats, seats_used, totp_seed_encrypted
		FROM registration_tokens
		WHERE token_hash = $1 AND status = 'active' AND expires_at > NOW() AND seats_used < max_seats
	`

	err := q.db.Get(&regToken, query, tokenHash)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("invalid, expired, or seats full")
		}
		return nil, fmt.Errorf("failed to validate token: %w", err)
	}

	return &regToken, nil
}

// MarkTokenUsed marks a token as used by an agent.
// The caller passes the plaintext token; this function hashes it before calling
// the stored procedure (which now matches on token_hash).
func (q *RegistrationTokenQueries) MarkTokenUsed(token string, agentID uuid.UUID) error {
	tokenHash := HashRegistrationToken(token)
	query := `SELECT mark_registration_token_used($1, $2)`

	var success bool
	err := q.db.QueryRow(query, tokenHash, agentID).Scan(&success)
	if err != nil {
		return fmt.Errorf("failed to mark token as used: %w", err)
	}

	if !success {
		return fmt.Errorf("token not found, already used, expired, or seats full")
	}

	return nil
}

// BoundAgent is the per-seat view returned alongside a registration token —
// "which agent is in this seat?" Joins agents to the audit ledger and surfaces
// the fields an operator needs to decide whether to revoke an individual seat.
type BoundAgent struct {
	AgentID  uuid.UUID `json:"agent_id" db:"agent_id"`
	Hostname string    `json:"hostname" db:"hostname"`
	OSType   string    `json:"os_type" db:"os_type"`
	Status   string    `json:"status" db:"status"`
	LastSeen time.Time `json:"last_seen" db:"last_seen"`
	UsedAt   time.Time `json:"used_at" db:"used_at"`
}

// GetAgentsBoundToToken returns the agents that consumed seats on a given
// registration token, ordered by when they registered. Powers the token-detail
// expansion in the dashboard (docs/AGENT_LIFECYCLE.md "Operator surfaces").
//
// Does not reveal credentials — just hostnames + status. Authorization is the
// caller's responsibility (route should sit behind admin middleware).
func (q *RegistrationTokenQueries) GetAgentsBoundToToken(tokenID uuid.UUID) ([]BoundAgent, error) {
	var agents []BoundAgent
	query := `
		SELECT a.id AS agent_id, a.hostname, a.os_type, a.status, a.last_seen, u.used_at
		FROM registration_token_usage u
		JOIN agents a ON a.id = u.agent_id
		WHERE u.token_id = $1
		ORDER BY u.used_at ASC
	`
	if err := q.db.Select(&agents, query, tokenID); err != nil {
		return nil, fmt.Errorf("failed to get agents bound to token: %w", err)
	}
	return agents, nil
}

// GetActiveRegistrationTokens returns all active tokens that haven't expired,
// with plaintext revealed for live tokens so the UI can rebuild the one-liner.
func (q *RegistrationTokenQueries) GetActiveRegistrationTokens() ([]RegistrationToken, error) {
	var tokens []RegistrationToken
	query := `
		SELECT id, token_hash, token_encrypted, label, expires_at, created_at, used_at, used_by_agent_id,
			   revoked, revoked_at, revoked_reason, status, created_by, metadata,
			   max_seats, seats_used
		FROM registration_tokens
		WHERE status = 'active' AND expires_at > NOW()
		ORDER BY created_at DESC
	`

	err := q.db.Select(&tokens, query)
	if err != nil {
		return nil, fmt.Errorf("failed to get active tokens: %w", err)
	}

	for i := range tokens {
		q.revealPlaintext(&tokens[i])
	}

	return tokens, nil
}

// GetAllRegistrationTokens returns all tokens with pagination. Plaintext is
// revealed only for live tokens; spent/expired/revoked rows carry none.
func (q *RegistrationTokenQueries) GetAllRegistrationTokens(limit, offset int) ([]RegistrationToken, error) {
	var tokens []RegistrationToken
	query := `
		SELECT id, token_hash, token_encrypted, label, expires_at, created_at, used_at, used_by_agent_id,
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

	for i := range tokens {
		q.revealPlaintext(&tokens[i])
	}

	return tokens, nil
}

// RevokeRegistrationToken revokes a token by its plaintext value.
//
// INVARIANT — no hidden cascade: this only flips the token row to status='revoked'.
// It deliberately does NOT touch refresh_tokens for agents that previously used
// this token to register. The registration_token and refresh_token credentials
// have separate lifecycles by design (see docs/AGENT_LIFECYCLE.md "Revocation").
// To revoke a bound agent's access, call RefreshTokenQueries.RevokeAllAgentTokens
// — that path is surfaced as an explicit per-agent operator action, not a side
// effect of revoking the issuing token.
func (q *RegistrationTokenQueries) RevokeRegistrationToken(token, reason string) error {
	tokenHash := HashRegistrationToken(token)
	query := `
		UPDATE registration_tokens
		SET status = 'revoked',
			revoked = true,
			revoked_at = NOW(),
			revoked_reason = $1
		WHERE token_hash = $2
	`

	result, err := q.db.Exec(query, reason, tokenHash)
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

// RevokeRegistrationTokenByID revokes a token by its UUID primary key.
//
// The UI sends the row UUID (not the plaintext token string), so this is the
// correct path for operator-initiated revokes from the dashboard. The same
// no-cascade invariant applies: only the registration_tokens row is flipped;
// agents already enrolled keep their refresh tokens until explicitly revoked
// via RevokeAllAgentTokens.
func (q *RegistrationTokenQueries) RevokeRegistrationTokenByID(id uuid.UUID, reason string) error {
	query := `
		UPDATE registration_tokens
		SET status = 'revoked',
			revoked = true,
			revoked_at = NOW(),
			revoked_reason = $1
		WHERE id = $2
	`

	result, err := q.db.Exec(query, reason, id)
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