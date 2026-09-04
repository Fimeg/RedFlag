package queries

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/capability"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// CapabilityTokenQueries persists minted capability tokens and their resolved
// closures. The token is the audit/replay record; the executor verifies it
// independently of anything stored here.
type CapabilityTokenQueries struct {
	db *sqlx.DB
}

func NewCapabilityTokenQueries(db *sqlx.DB) *CapabilityTokenQueries {
	return &CapabilityTokenQueries{db: db}
}

// Insert stores a freshly minted, signed token. updateID may be uuid.Nil when
// the token covers an aggregate that is not tied to a single update row.
func (q *CapabilityTokenQueries) Insert(t *capability.Token, updateID uuid.UUID) error {
	closureJSON, err := json.Marshal(t.Closure)
	if err != nil {
		return err
	}
	tokenID, err := uuid.FromString(t.TokenID)
	if err != nil {
		return err
	}
	agentID, err := uuid.FromString(t.AgentID)
	if err != nil {
		return err
	}

	var updatePtr interface{}
	if updateID != uuid.Nil {
		updatePtr = updateID
	}

	query := `
		INSERT INTO capability_tokens (
			token_id, update_id, agent_id, key_id, package_type, operation,
			closure, issued_at, not_before, expires_at, signature
		) VALUES (
			$1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11
		)
	`
	_, err = q.db.Exec(query,
		tokenID, updatePtr, agentID, t.KeyID, t.PackageType, t.Operation,
		closureJSON, t.IssuedAt, t.NotBefore, t.ExpiresAt, t.Signature)
	return err
}

// GetByID returns the stored token reconstructed into a capability.Token.
func (q *CapabilityTokenQueries) GetByID(tokenID uuid.UUID) (*capability.Token, error) {
	var row struct {
		TokenID     uuid.UUID `db:"token_id"`
		AgentID     uuid.UUID `db:"agent_id"`
		KeyID       string    `db:"key_id"`
		PackageType string    `db:"package_type"`
		Operation   string    `db:"operation"`
		Closure     []byte    `db:"closure"`
		IssuedAt    int64     `db:"issued_at"`
		NotBefore   int64     `db:"not_before"`
		ExpiresAt   int64     `db:"expires_at"`
		Signature   string    `db:"signature"`
	}
	query := `
		SELECT token_id, agent_id, key_id, package_type, operation,
		       closure, issued_at, not_before, expires_at, signature
		FROM capability_tokens WHERE token_id = $1`
	if err := q.db.Get(&row, query, tokenID); err != nil {
		return nil, err
	}

	var closure []capability.ClosureEntry
	if err := json.Unmarshal(row.Closure, &closure); err != nil {
		return nil, err
	}
	return &capability.Token{
		Version:     capability.Version,
		TokenID:     row.TokenID.String(),
		AgentID:     row.AgentID.String(),
		KeyID:       row.KeyID,
		PackageType: row.PackageType,
		Operation:   row.Operation,
		Closure:     closure,
		IssuedAt:    row.IssuedAt,
		NotBefore:   row.NotBefore,
		ExpiresAt:   row.ExpiresAt,
		Signature:   row.Signature,
	}, nil
}

// ListUndeliveredForAgent returns minted-but-not-yet-delivered, unexpired tokens
// for an agent, oldest first.
func (q *CapabilityTokenQueries) ListUndeliveredForAgent(agentID uuid.UUID, now int64) ([]uuid.UUID, error) {
	var ids []uuid.UUID
	query := `
		SELECT token_id FROM capability_tokens
		WHERE agent_id = $1 AND delivered_at IS NULL AND expires_at > $2
		ORDER BY created_at ASC`
	if err := q.db.Select(&ids, query, agentID, now); err != nil {
		return nil, err
	}
	return ids, nil
}

// MarkDelivered records that a token was handed to its agent.
func (q *CapabilityTokenQueries) MarkDelivered(tokenID uuid.UUID) error {
	_, err := q.db.Exec(
		`UPDATE capability_tokens SET delivered_at = $1 WHERE token_id = $2 AND delivered_at IS NULL`,
		time.Now().UTC(), tokenID)
	return err
}

// MarkConsumed records the executor's confirmation that a token was used.
func (q *CapabilityTokenQueries) MarkConsumed(tokenID uuid.UUID) (bool, error) {
	res, err := q.db.Exec(
		`UPDATE capability_tokens SET consumed_at = $1 WHERE token_id = $2 AND consumed_at IS NULL`,
		time.Now().UTC(), tokenID)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

// HasActiveForUpdate reports whether an unexpired, unconsumed token already exists
// for an update. Used to keep minting idempotent: a dry-run may be re-run, but one
// approved operation should not produce multiple live capability tokens.
func (q *CapabilityTokenQueries) HasActiveForUpdate(updateID uuid.UUID, now int64) (bool, error) {
	var count int
	query := `
		SELECT COUNT(*) FROM capability_tokens
		WHERE update_id = $1 AND consumed_at IS NULL AND expires_at > $2`
	if err := q.db.Get(&count, query, updateID, now); err != nil {
		return false, err
	}
	return count > 0, nil
}

// GetUpdateIDForToken returns the update row a token was minted for.
func (q *CapabilityTokenQueries) GetUpdateIDForToken(tokenID uuid.UUID) (uuid.UUID, bool, error) {
	var updateID uuid.NullUUID
	if err := q.db.Get(&updateID, `SELECT update_id FROM capability_tokens WHERE token_id = $1`, tokenID); err != nil {
		return uuid.Nil, false, err
	}
	if !updateID.Valid {
		return uuid.Nil, false, nil
	}
	return updateID.UUID, true, nil
}

// IsConsumed reports whether a token has already been marked consumed (server-side
// replay view; the executor holds the authoritative local replay guard).
func (q *CapabilityTokenQueries) IsConsumed(tokenID uuid.UUID) (bool, error) {
	var consumed sql.NullTime
	err := q.db.Get(&consumed, `SELECT consumed_at FROM capability_tokens WHERE token_id = $1`, tokenID)
	if err != nil {
		return false, err
	}
	return consumed.Valid, nil
}

// NOTE: HasConsumedTokenForUpdate was removed deliberately. The scan-set reconciler
// (RECONCILE-001) used to call it to label out-of-band closures as redflag_receipt
// when a consumed token existed, but the row ID is preserved across the UPSERT key
// (agent_id, package_type, package_name), so a token consumed in a *prior* lifecycle
// stays linked to the same ID and produced false redflag_receipt provenance on what
// were genuinely out-of-band closures of a new version. Waiting-state rows can hold
// no token for their current version (tokens mint only after checking_dependencies),
// so the reconciler now unconditionally stamps out_of_band — no per-row token lookup.

// TokenStatusHistory is one row of the token lifecycle visible to operators.
type TokenStatusHistory struct {
	TokenID     uuid.UUID  `json:"token_id" db:"token_id"`
	AgentID     uuid.UUID  `json:"agent_id" db:"agent_id"`
	PackageType string     `json:"package_type" db:"package_type"`
	Operation   string     `json:"operation" db:"operation"`
	KeyID       string     `json:"key_id" db:"key_id"`
	ClosureHash string     `json:"closure_hash" db:"closure_hash"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
	DeliveredAt *time.Time `json:"delivered_at" db:"delivered_at"`
	ConsumedAt  *time.Time `json:"consumed_at" db:"consumed_at"`
	ExpiresAt   int64      `json:"expires_at" db:"expires_at"`
	Signature   string     `json:"signature" db:"signature"`
}

// Status returns the operator-facing lifecycle label: pending, consumed, succeeded, or failed.
// The server only knows pending/consumed; the agent reports the outcome (succeeded/failed)
// via ReportCapabilityResult, which sets consumed_at with the decision metadata on the
// update row. This method reflects what the server knows at the token table level.
func (t *TokenStatusHistory) Status() string {
	if t.ConsumedAt != nil {
		return "consumed"
	}
	if t.DeliveredAt != nil {
		return "delivered"
	}
	return "pending"
}

// GetTokenHistory returns all capability tokens for a given agent + update pair, newest first.
// updateID may be uuid.Nil to return all tokens for the agent.
func (q *CapabilityTokenQueries) GetTokenHistory(agentID uuid.UUID, updateID uuid.UUID) ([]TokenStatusHistory, error) {
	query := `
		SELECT token_id, agent_id, package_type, operation, key_id,
		       ENCODE(SHA256(closure::text::bytea), 'hex') AS closure_hash,
		       created_at, delivered_at, consumed_at, expires_at, signature
		FROM capability_tokens
		WHERE agent_id = $1`
	args := []interface{}{agentID}
	i := 2
	if updateID != uuid.Nil {
		query += fmt.Sprintf(` AND update_id = $%d`, i)
		args = append(args, updateID)
		i++
	}
	query += ` ORDER BY created_at DESC`
	var rows []TokenStatusHistory
	if err := q.db.Select(&rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}

// GetActiveTokenCount returns the count of unconsumed, unexpired capability tokens
// across all agents. Tokens are considered active if consumed_at IS NULL AND
// expires_at > current epoch.
func (q *CapabilityTokenQueries) GetActiveTokenCount() (int, error) {
	var count int
	query := `SELECT COUNT(*) FROM capability_tokens WHERE consumed_at IS NULL AND expires_at > EXTRACT(EPOCH FROM NOW())::bigint`
	if err := q.db.Get(&count, query); err != nil {
		return 0, err
	}
	return count, nil
}
