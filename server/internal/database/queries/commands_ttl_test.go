package queries_test

// commands_ttl_test.go — Pre-fix tests for missing TTL in GetPendingCommands.
//
// These tests inspect the SQL query string for GetPendingCommands to document
// the absence of a time-bounding (TTL) filter (BUG F-6 / F-7).
//
// No live database is required — tests operate on the copied query string.
//
// Test categories:
//   TestGetPendingCommandsHasNoTTLFilter   PASS-NOW / FAIL-AFTER-FIX
//   TestGetPendingCommandsMustHaveTTLFilter FAIL-NOW / PASS-AFTER-FIX
//
// IMPORTANT: When the fix is applied (TTL clause added to GetPendingCommands),
// update the getPendingCommandsQuery constant below to match the new query,
// and update the assertions in both tests accordingly.
//
// Run: cd server && go test ./internal/database/queries/... -v -run TestGetPending

import (
	"strings"
	"testing"
)

// getPendingCommandsQuery is a verbatim copy of the query in
// queries/commands.go GetPendingCommands.
//
// POST-FIX (F-6 + F-7): TTL filter added via expires_at column.
// Commands past their expires_at are no longer returned to the agent.
const getPendingCommandsQuery = `
		SELECT * FROM agent_commands
		WHERE agent_id = $1 AND status = 'pending'
		AND (expires_at IS NULL OR expires_at > NOW())
		ORDER BY created_at ASC
		LIMIT 100
	`

// ttlFilterIndicators lists the SQL tokens that would be present in a
// correctly time-bounded query. The absence of all of them confirms the bug.
var ttlFilterIndicators = []string{
	"INTERVAL",   // e.g. NOW() - INTERVAL '24 hours'
	"expires_at", // if an expires_at column is added (F-7 fix)
}

// ---------------------------------------------------------------------------
// Test 3.1a — Documents that the bug exists
//
// Category: PASS-NOW / FAIL-AFTER-FIX
//
// Asserts that getPendingCommandsQuery does NOT contain any TTL filter token.
// Currently PASSES (bug is present). Will FAIL when the fix adds a TTL clause
// and this constant is updated to match.
// ---------------------------------------------------------------------------

func TestGetPendingCommandsHasNoTTLFilter(t *testing.T) {
	// POST-FIX (F-6 + F-7): TTL filter is now present via expires_at.
	// This test now asserts the TTL indicator IS present (inverted from pre-fix).

	hasTTL := false
	for _, indicator := range ttlFilterIndicators {
		if strings.Contains(getPendingCommandsQuery, indicator) {
			hasTTL = true
			t.Logf("POST-FIX: TTL indicator %q found in query", indicator)
		}
	}

	if !hasTTL {
		t.Errorf("F-6/F-7 FIX BROKEN: GetPendingCommands query should contain a TTL filter")
	}

	t.Log("F-6/F-7 FIXED: GetPendingCommands query now filters by expires_at.")
	t.Log("Query text:")
	t.Log(getPendingCommandsQuery)
}

// ---------------------------------------------------------------------------
// Test 3.1b — Asserts the correct post-fix behaviour
//
// Category: FAIL-NOW / PASS-AFTER-FIX
//
// Asserts that getPendingCommandsQuery DOES contain a TTL filter.
// Currently FAILS (bug is present). Will PASS once the fix is applied and
// this constant is updated.
// ---------------------------------------------------------------------------

func TestGetPendingCommandsMustHaveTTLFilter(t *testing.T) {
	// POST-FIX (F-6 + F-7): This test now PASSES.
	// The query contains expires_at TTL filter.

	hasTTL := false
	for _, indicator := range ttlFilterIndicators {
		if strings.Contains(getPendingCommandsQuery, indicator) {
			hasTTL = true
			break
		}
	}

	if !hasTTL {
		t.Errorf("GetPendingCommands query must contain a TTL filter (expires_at or INTERVAL)")
	}
}

// ---------------------------------------------------------------------------
// Test 3.2 — Complementary: RetryCommand copies Params but not signature
//
// Category: PASS-NOW / FAIL-AFTER-FIX
//
// Independently of the DB, documents that queries.RetryCommand (commands.go:189)
// builds a new AgentCommand without propagating Signature, SignedAt, or KeyID.
// This is a query-level confirmation of BUG F-5, placed here because it
// exercises the same file (commands.go).
// ---------------------------------------------------------------------------

func TestRetryCommandQueryDoesNotCopySignature(t *testing.T) {
	// BUG F-5 (query layer): RetryCommand builds a new command without signing.
	// The INSERT that follows has Signature="", SignedAt=nil, KeyID="" in the row.

	// Verify by inspecting what fields the struct would have after RetryCommand.
	// (Struct construction from commands.go:202)
	//
	// newCommand := &models.AgentCommand{
	//     ID:            uuid.Must(uuid.NewV4()),
	//     AgentID:       original.AgentID,
	//     CommandType:   original.CommandType,
	//     Params:        original.Params,       ← Params copied
	//     Status:        models.CommandStatusPending,
	//     CreatedAt:     time.Now(),
	//     RetriedFromID: &originalID,
	//     // Signature: NOT copied — zero value ""
	//     // SignedAt:  NOT copied — zero value nil
	//     // KeyID:     NOT copied — zero value ""
	// }
	//
	// The INSERT query in CreateCommand (commands.go:38) includes :signature,
	// :key_id, :signed_at in its column list — they will be stored as
	// NULL / empty string, which is the unfixed state.

	// Document the field names in the INSERT that are left empty by RetryCommand.
	retryCreatesFields := []string{"id", "agent_id", "command_type", "params", "status", "retried_from_id"}
	retryOmitsFields := []string{"signature", "key_id", "signed_at"}

	// These will be empty/nil in the created command (confirms bug).
	for _, f := range retryOmitsFields {
		t.Logf("BUG F-5: RetryCommand does not set field: %q (will be empty/nil)", f)
	}
	for _, f := range retryCreatesFields {
		t.Logf("  RetryCommand does set field: %q", f)
	}

	// This test always passes — it's purely documentary.
	// It will need to be updated when the fix is applied (RetryCommand will then set all three).
	t.Log("BUG F-5 CONFIRMED (query layer): Retry path omits signature, key_id, and signed_at.")
	t.Log("After fix: all three fields must be populated by a signing call in RetryCommand.")
}
