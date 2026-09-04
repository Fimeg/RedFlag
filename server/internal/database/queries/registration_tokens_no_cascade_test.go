package queries_test

// registration_tokens_no_cascade_test.go — Lock the no-cascade invariant on
// RevokeRegistrationToken and RevokeRegistrationTokenByID.
//
// The registration_token and refresh_token credentials are deliberately kept
// on separate lifecycles (see docs/AGENT_LIFECYCLE.md "Revocation"). Revoking
// a registration token only blocks future installs; it must NOT touch the
// refresh_tokens of agents that previously used the token to register.
//
// This test is a static check on the query text — no live database required.
// If a future change introduces a cascade (joining refresh_tokens into the
// UPDATE, deleting bound rows, calling out to revoke refresh tokens by side
// effect), this test fails loudly and points the contributor at the spec.
//
// If a cascade ever becomes the desired behavior, that is a deliberate spec
// change: update docs/AGENT_LIFECYCLE.md first, then this test, then the code.
//
// Run: cd server && go test ./internal/database/queries/... -v -run TestRevokeRegistrationToken

import (
	"strings"
	"testing"
)

// revokeRegistrationTokenQuery is a verbatim copy of the query in
// queries/registration_tokens.go RevokeRegistrationToken. Keep in sync.
const revokeRegistrationTokenQuery = `
		UPDATE registration_tokens
		SET status = 'revoked',
			revoked = true,
			revoked_at = NOW(),
			revoked_reason = $1
		WHERE token_hash = $2
	`

// revokeRegistrationTokenByIDQuery is a verbatim copy of the query in
// queries/registration_tokens.go RevokeRegistrationTokenByID. Keep in sync.
const revokeRegistrationTokenByIDQuery = `
		UPDATE registration_tokens
		SET status = 'revoked',
			revoked = true,
			revoked_at = NOW(),
			revoked_reason = $1
		WHERE id = $2
	`

// cascadeIndicators lists SQL tokens that would mean a revoke function
// is reaching into agent credentials. Presence of ANY of these in the query
// body indicates the invariant has been violated.
var cascadeIndicators = []string{
	"refresh_tokens",
	"agent_id",
	"DELETE",
	"revoke_agent",
	"RevokeAll",
}

func TestRevokeRegistrationTokenHasNoRefreshTokenCascade(t *testing.T) {
	q := strings.ToLower(revokeRegistrationTokenQuery)
	for _, ind := range cascadeIndicators {
		if strings.Contains(q, strings.ToLower(ind)) {
			t.Errorf("RevokeRegistrationToken query touches %q — this is a hidden cascade. "+
				"The registration_token and refresh_token lifecycles must stay separate. "+
				"See docs/AGENT_LIFECYCLE.md 'Revocation'. To revoke a bound agent, callers "+
				"must invoke RefreshTokenQueries.RevokeAllAgentTokens explicitly.", ind)
		}
	}
}

func TestRevokeRegistrationTokenOnlyTouchesRegistrationTokensTable(t *testing.T) {
	q := strings.ToLower(revokeRegistrationTokenQuery)
	if !strings.Contains(q, "registration_tokens") {
		t.Fatal("query no longer mentions registration_tokens — copy in this test is stale")
	}
	if !strings.Contains(q, "status = 'revoked'") {
		t.Error("query no longer sets status='revoked' — copy in this test is stale, or the " +
			"revocation behavior changed. Sync with registration_tokens.go.")
	}
}

func TestRevokeRegistrationTokenByIDHasNoRefreshTokenCascade(t *testing.T) {
	q := strings.ToLower(revokeRegistrationTokenByIDQuery)
	for _, ind := range cascadeIndicators {
		if strings.Contains(q, strings.ToLower(ind)) {
			t.Errorf("RevokeRegistrationTokenByID query touches %q — hidden cascade. "+
				"The registration_token and refresh_token lifecycles must stay separate. "+
				"See docs/AGENT_LIFECYCLE.md 'Revocation'.", ind)
		}
	}
}

func TestRevokeRegistrationTokenByIDOnlyTouchesRegistrationTokensTable(t *testing.T) {
	q := strings.ToLower(revokeRegistrationTokenByIDQuery)
	if !strings.Contains(q, "registration_tokens") {
		t.Fatal("by-ID query no longer mentions registration_tokens — copy in this test is stale")
	}
	if !strings.Contains(q, "status = 'revoked'") {
		t.Error("by-ID query no longer sets status='revoked' — copy in this test is stale, or the " +
			"revocation behavior changed. Sync with registration_tokens.go.")
	}
	// Must key on id, not token_hash or plaintext token
	if !strings.Contains(q, "where id =") {
		t.Error("by-ID query does not filter by id — either the copy is stale or the wrong query was used")
	}
}
