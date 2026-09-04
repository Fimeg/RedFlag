package crypto

// replay_test.go — Pre-fix tests for command replay attack surface on the agent side.
//
// These tests document the current (buggy) behaviour of the agent's command
// verification path. All tests use helpers defined in verification_test.go
// (generateKeyPair, signCommand, signCommandOld) which are in the same package.
//
// Each test is categorised:
//
//   PASS-NOW / FAIL-AFTER-FIX  — documents a bug as-is; flips to fail when fix is applied.
//
// Run: cd agent && go test ./internal/crypto/... -v -run TestReplay
//      cd agent && go test ./internal/crypto/... -v -run TestOld
//      cd agent && go test ./internal/crypto/... -v -run TestNew
//      cd agent && go test ./internal/crypto/... -v -run TestSame
//      cd agent && go test ./internal/crypto/... -v -run TestCross

import (
	"testing"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/client"
)

// ---------------------------------------------------------------------------
// Test 2.1 — BUG F-3: Old-format commands are valid forever
//
// Category: PASS-NOW / FAIL-AFTER-FIX
//
// VerifyCommand (crypto/verification.go:25) reconstructs the message as
// "{id}:{type}:{sha256(params)}" and verifies the Ed25519 signature.
// There is NO time check. A command signed 72 hours ago — or 72 years ago —
// passes without error. The test PASSES now (bug is present).
// After fix (add expiry to VerifyCommand or deprecate old format): FAILS.
// ---------------------------------------------------------------------------

func TestOldFormatReplayIsUnbounded(t *testing.T) {
	// POST-FIX (F-3): Old-format commands with CreatedAt older than 48h are rejected.
	pub, priv := generateKeyPair(t)
	v := NewCommandVerifier()

	// Old-format command created 72 hours ago
	createdAt := time.Now().Add(-72 * time.Hour)
	cmd := client.Command{
		ID:        "old-replay-cmd-72h",
		Type:      "install_updates",
		Params:    map[string]interface{}{"package": "nginx"},
		SignedAt:  nil,
		CreatedAt: &createdAt,
	}
	cmd.Signature = signCommandOld(t, priv, &cmd)

	// After F-3 fix: 72h old-format command must be rejected
	err := v.VerifyCommand(cmd, pub)
	if err == nil {
		t.Error("F-3 FIX BROKEN: old-format command 72h old should be rejected, but passed")
	}
	t.Logf("F-3 FIXED: Old-format command created 72h ago correctly rejected: %v", err)
}

func TestOldFormatRecentCommandStillPasses(t *testing.T) {
	// POST-FIX (F-3): Old-format commands WITHIN 48h still pass (backward compat).
	pub, priv := generateKeyPair(t)
	v := NewCommandVerifier()

	// Old-format command created 12 hours ago (within 48h limit)
	createdAt := time.Now().Add(-12 * time.Hour)
	cmd := client.Command{
		ID:        "old-recent-cmd",
		Type:      "install_updates",
		Params:    map[string]interface{}{"package": "nginx"},
		SignedAt:  nil,
		CreatedAt: &createdAt,
	}
	cmd.Signature = signCommandOld(t, priv, &cmd)

	err := v.VerifyCommand(cmd, pub)
	if err != nil {
		t.Errorf("old-format command within 48h should pass: %v", err)
	}
	t.Log("F-3 BACKWARD COMPAT: Old-format command created 12h ago passes verification.")
}

// ---------------------------------------------------------------------------
// Test 2.2 — F-4: New-format commands can be replayed for 24 hours
//
// Category: PASS-NOW / MAY-REMAIN-PASSING-UNTIL-maxAge-IS-REDUCED
//
// VerifyCommandWithTimestamp allows commands signed up to commandMaxAge (24h)
// in the past. A captured command from 23 hours and 59 minutes ago still
// passes verification. This documents the replay window.
//
// Note: This test reflects an intentional (but generous) design decision.
// It will only flip to FAIL if commandMaxAge is reduced below 24h.
// ---------------------------------------------------------------------------

func TestNewFormatCommandCanBeReplayedWithin24Hours(t *testing.T) {
	// POST-FIX (F-4): commandMaxAge reduced to 4h. Test updated to use 3h59m.
	pub, priv := generateKeyPair(t)
	v := NewCommandVerifier()

	// Command signed almost 4 hours ago — still within the new 4h window.
	signedAt := time.Now().UTC().Add(-3*time.Hour - 59*time.Minute)

	cmd := client.Command{
		ID:      "replay-4h-cmd",
		Type:    "install_updates",
		Params:  map[string]interface{}{"package": "nginx"},
		AgentID: "agent-replay-test",
	}
	cmd.SignedAt = &signedAt
	cmd.Signature = signCommand(t, priv, &cmd, signedAt)

	err := v.VerifyCommandWithTimestamp(cmd, pub, 4*time.Hour, 5*time.Minute)
	if err != nil {
		t.Fatalf("expected 3h59m old command to pass within 4h window, got: %v", err)
	}

	t.Log("F-4 FIXED: Command signed 3h59m ago passes VerifyCommandWithTimestamp with 4h window.")
	t.Logf("  SignedAt: %v (window: 4h)", signedAt.Format(time.RFC3339))
}

func TestCommandBeyond4HoursIsRejected(t *testing.T) {
	// POST-FIX (F-4): Commands older than 4h must be rejected.
	pub, priv := generateKeyPair(t)
	v := NewCommandVerifier()

	signedAt := time.Now().UTC().Add(-4*time.Hour - 1*time.Minute)
	cmd := client.Command{
		ID:      "expired-4h-cmd",
		Type:    "install_updates",
		Params:  map[string]interface{}{"package": "nginx"},
		AgentID: "agent-replay-test",
	}
	cmd.SignedAt = &signedAt
	cmd.Signature = signCommand(t, priv, &cmd, signedAt)

	err := v.VerifyCommandWithTimestamp(cmd, pub, 4*time.Hour, 5*time.Minute)
	if err == nil {
		t.Error("expected 4h1m old command to be rejected, but it passed")
	}
	t.Logf("F-4 FIXED: Command signed 4h1m ago correctly rejected: %v", err)
}

// ---------------------------------------------------------------------------
// Test 2.3 — BUG F-2: The same command can be verified any number of times
//
// Category: PASS-NOW / FAIL-AFTER-FIX
//
// VerifyCommandWithTimestamp is a pure function — given the same inputs it
// returns the same output every time. There is no nonce, no single-use token,
// and no agent-side deduplication. A replayed command passes verification
// identically on the second, third, and Nth call within the time window.
// ---------------------------------------------------------------------------

func TestSameCommandCanBeVerifiedTwice(t *testing.T) {
	// POST-FIX (F-2): Deduplication is now at the ProcessCommand level,
	// not at the VerifyCommandWithTimestamp level. The verifier is a pure
	// function — it still returns success on repeated calls. The dedup
	// is handled by CommandHandler.ProcessCommand's executedIDs set.
	//
	// This test documents that the VERIFIER allows repeated verification
	// (which is correct — dedup is a higher-layer concern).
	pub, priv := generateKeyPair(t)
	v := NewCommandVerifier()

	signedAt := time.Now().UTC()
	cmd := client.Command{
		ID:      "no-nonce-cmd",
		Type:    "reboot",
		Params:  map[string]interface{}{},
		AgentID: "agent-dedup-test",
	}
	cmd.SignedAt = &signedAt
	cmd.Signature = signCommand(t, priv, &cmd, signedAt)

	// Verifier-level: still passes multiple times (pure function)
	err1 := v.VerifyCommandWithTimestamp(cmd, pub, 4*time.Hour, 5*time.Minute)
	if err1 != nil {
		t.Fatalf("first verification should pass: %v", err1)
	}

	err2 := v.VerifyCommandWithTimestamp(cmd, pub, 4*time.Hour, 5*time.Minute)
	if err2 != nil {
		t.Fatalf("second verification should also pass at verifier level: %v", err2)
	}

	t.Log("F-2 NOTE: Verifier is a pure function — dedup is at ProcessCommand layer.")
	t.Log("CommandHandler.ProcessCommand maintains executedIDs set for single-use enforcement.")
}

// ---------------------------------------------------------------------------
// Test 2.4 — BUG F-1: Signed message contains no agent binding
//
// Category: PASS-NOW / FAIL-AFTER-FIX
//
// The signed message format is: "{id}:{type}:{sha256(params)}:{timestamp}"
// None of these components are bound to a specific agent. The client.Command
// struct has no agent_id field at all — it is stripped before delivery.
// The same signature verifies regardless of which agent receives the command.
// ---------------------------------------------------------------------------

func TestCrossAgentSignatureVerifies(t *testing.T) {
	// POST-FIX (F-1): agent_id is now in the signed message.
	// A command signed for agent A must fail verification when presented
	// with agent B's ID.
	pub, priv := generateKeyPair(t)
	v := NewCommandVerifier()

	signedAt := time.Now().UTC()
	agentA := "agent-aaa-111"
	agentB := "agent-bbb-222"

	// Sign command for agent A using v3 format
	cmd := client.Command{
		ID:      "cross-agent-cmd",
		Type:    "install_updates",
		Params:  map[string]interface{}{"package": "nginx"},
		AgentID: agentA,
	}
	cmd.SignedAt = &signedAt
	cmd.Signature = signCommand(t, priv, &cmd, signedAt)

	// Verify with correct agent A — should pass
	err := v.VerifyCommandWithTimestamp(cmd, pub, 24*time.Hour, 5*time.Minute)
	if err != nil {
		t.Fatalf("verification with correct agent A should pass, got: %v", err)
	}

	// Now try with agent B's ID — should FAIL
	cmdForB := cmd
	cmdForB.AgentID = agentB
	err = v.VerifyCommandWithTimestamp(cmdForB, pub, 24*time.Hour, 5*time.Minute)
	if err == nil {
		t.Error("F-1 FIX BROKEN: cross-agent verification with agent B should FAIL but passed")
	}

	t.Logf("F-1 FIXED: Command signed for agent %q fails verification when presented as agent %q", agentA, agentB)
	t.Log("The signature is now bound to the target agent_id in the v3 message format.")
}
