package services

import (
	"fmt"
	"log"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/capability"
	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
)

// DefaultTokenTTL bounds how long a minted token stays valid. Short by design:
// a token authorizes one already-approved operation that the agent should pick
// up promptly. Long enough to survive normal poll/delivery latency, short enough
// that a leaked token is not a standing capability.
const DefaultTokenTTL = time.Hour

// CapabilityMinter builds, signs, and persists supply-chain capability tokens at
// approval time. It is the server's authority role: it only mints after the
// caller's policy checks (OSV, age, hash) have cleared.
//
// The signing key stays inside SigningService; the minter never touches key
// material directly. Full off-web-process signer isolation (plan constraint #2)
// is an infrastructure step layered on top of this seam — this type is the only
// place that asks for a signature, so relocating the signer means swapping the
// SigningService dependency here, not rewriting callers.
type CapabilityMinter struct {
	signing *SigningService
	tokens  *queries.CapabilityTokenQueries
	ttl     time.Duration
}

func NewCapabilityMinter(signing *SigningService, tokens *queries.CapabilityTokenQueries) *CapabilityMinter {
	return &CapabilityMinter{signing: signing, tokens: tokens, ttl: DefaultTokenTTL}
}

// SetTTL overrides the token validity window.
func (m *CapabilityMinter) SetTTL(d time.Duration) {
	if d > 0 {
		m.ttl = d
	}
}

// Enabled reports whether minting can produce signed tokens. When signing is
// disabled the gate is simply not active; that is not an error condition.
func (m *CapabilityMinter) Enabled() bool {
	return m != nil && m.signing != nil && m.signing.IsEnabled()
}

// MintForUpdate builds a token authorizing the approved update over the supplied
// resolved closure, signs it, and persists it for delivery. The caller assembles
// the closure (today: the top-level package plus its expected hash; later: the
// full transitive set) so this stays the single signing chokepoint.
//
// Returns (nil, nil) when signing is disabled — the gate is optional and its
// absence must not break the approval path it hangs off.
func (m *CapabilityMinter) MintForUpdate(update *models.UpdateState, closure []capability.ClosureEntry) (*capability.Token, error) {
	if !m.Enabled() {
		return nil, nil
	}
	if len(closure) == 0 {
		return nil, fmt.Errorf("capability: refusing to mint a token over an empty closure")
	}

	operation := "upgrade"
	if update.CurrentVersion == "" {
		operation = "install"
	}

	now := time.Now().UTC()
	token := &capability.Token{
		Version:     capability.Version,
		TokenID:     uuid.Must(uuid.NewV4()).String(),
		AgentID:     update.AgentID.String(),
		PackageType: update.PackageType,
		Operation:   operation,
		Closure:     closure,
		IssuedAt:    now.Unix(),
		NotBefore:   now.Unix(),
		ExpiresAt:   now.Add(m.ttl).Unix(),
	}

	if err := m.signing.SignCapabilityToken(token); err != nil {
		return nil, fmt.Errorf("capability: sign failed: %w", err)
	}

	if err := m.tokens.Insert(token, update.ID); err != nil {
		return nil, fmt.Errorf("capability: persist failed: %w", err)
	}

	log.Printf("[SECURITY] [server] [capability] token_minted token_id=%s agent_id=%s package_type=%s operation=%s closure_size=%d key_id=%s expires_at=%d",
		token.TokenID, token.AgentID, token.PackageType, token.Operation, len(token.Closure), token.KeyID, token.ExpiresAt)

	return token, nil
}
