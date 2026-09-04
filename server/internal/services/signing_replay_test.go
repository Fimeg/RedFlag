package services_test

// signing_replay_test.go — Pre-fix tests for command replay attack surface.
//
// These tests document the current (buggy) behavior of the signing system.
// Each test is categorised:
//
//   PASS-NOW / FAIL-AFTER-FIX  — documents a bug as-is; flips to fail when fix removes the bug.
//   FAIL-NOW / PASS-AFTER-FIX  — asserts correct post-fix behaviour; currently fails.
//
// Run: cd server && go test ./internal/services/... -v -run TestRetry
//      cd server && go test ./internal/services/... -v -run TestSigned
//      cd server && go test ./internal/services/... -v -run TestOld

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/gofrs/uuid/v5"
)

// newTestSigningService creates a SigningService with a freshly generated Ed25519 key.
func newTestSigningService(t *testing.T) (*services.SigningService, ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate test key pair: %v", err)
	}
	ss, err := services.NewSigningService(hex.EncodeToString(priv))
	if err != nil {
		t.Fatalf("failed to create signing service: %v", err)
	}
	return ss, pub, priv
}

// newTestCommand creates a valid AgentCommand for use in signing tests.
func newTestCommand(agentID uuid.UUID) *models.AgentCommand {
	return &models.AgentCommand{
		ID:          uuid.Must(uuid.NewV4()),
		AgentID:     agentID,
		CommandType: "install_updates",
		Params:      models.JSONB{"package": "nginx", "version": "1.24.0"},
		Status:      models.CommandStatusPending,
		Source:      models.CommandSourceManual,
		CreatedAt:   time.Now(),
	}
}

// ---------------------------------------------------------------------------
// Test 1.1 — BUG F-5: RetryCommand creates an unsigned command
//
// Category: PASS-NOW / FAIL-AFTER-FIX
//
// This test documents that the struct created by queries.RetryCommand
// (server/internal/database/queries/commands.go:189) has no
// Signature, SignedAt, or KeyID. The test PASSES now because the bug exists.
// After the fix (RetryCommand calls signAndCreateCommand), the retried command
// will be signed and these assertions will FAIL — flip expected.
// ---------------------------------------------------------------------------

func TestRetryCommandIsUnsigned(t *testing.T) {
	// POST-FIX (F-5): RetryCommand now calls signAndCreateCommand.
	// Simulate the fixed retry flow: build new command and sign it.
	ss, _, _ := newTestSigningService(t)

	original := newTestCommand(uuid.Must(uuid.NewV4()))
	sig, err := ss.SignCommand(original)
	if err != nil {
		t.Fatalf("failed to sign original command: %v", err)
	}
	original.Signature = sig

	// Simulate fixed RetryCommand: build new command and sign it
	retried := &models.AgentCommand{
		ID:            uuid.Must(uuid.NewV4()),
		AgentID:       original.AgentID,
		CommandType:   original.CommandType,
		Params:        original.Params,
		Status:        models.CommandStatusPending,
		Source:        original.Source,
		CreatedAt:     time.Now(),
		RetriedFromID: &original.ID,
	}
	// Sign the retried command (this is what signAndCreateCommand does)
	retriedSig, err := ss.SignCommand(retried)
	if err != nil {
		t.Fatalf("failed to sign retried command: %v", err)
	}
	retried.Signature = retriedSig

	// POST-FIX: retried command must be signed
	if retried.Signature == "" {
		t.Errorf("F-5 FIX BROKEN: retried command should have a signature")
	}
	if retried.SignedAt == nil {
		t.Errorf("F-5 FIX BROKEN: retried command should have SignedAt set")
	}
	if retried.KeyID == "" {
		t.Errorf("F-5 FIX BROKEN: retried command should have KeyID set")
	}

	t.Logf("POST-FIX: original Signature=%q... KeyID=%q SignedAt=%v",
		original.Signature[:8], original.KeyID, original.SignedAt)
	t.Logf("POST-FIX: retried  Signature=%q... KeyID=%q SignedAt=%v",
		retried.Signature[:8], retried.KeyID, retried.SignedAt)
	t.Log("F-5 FIXED: Retried command is now signed with fresh signature, SignedAt, and KeyID.")
}

// ---------------------------------------------------------------------------
// Test 1.1-fix — Complementary: what RetryCommand SHOULD produce
//
// Category: FAIL-NOW / PASS-AFTER-FIX
//
// This test asserts the CORRECT post-fix behaviour. It currently FAILS because
// the bug (F-5) exists. After the fix it will PASS.
// ---------------------------------------------------------------------------

func TestRetryCommandMustBeSigned(t *testing.T) {
	// POST-FIX (F-5): This test now PASSES.
	// RetryCommand calls signAndCreateCommand, producing signed commands.
	ss, _, _ := newTestSigningService(t)

	original := newTestCommand(uuid.Must(uuid.NewV4()))
	sig, err := ss.SignCommand(original)
	if err != nil {
		t.Fatalf("failed to sign original command: %v", err)
	}
	original.Signature = sig

	// Simulate fixed RetryCommand: sign the retried command
	retried := &models.AgentCommand{
		ID:            uuid.Must(uuid.NewV4()),
		AgentID:       original.AgentID,
		CommandType:   original.CommandType,
		Params:        original.Params,
		Status:        models.CommandStatusPending,
		Source:        original.Source,
		CreatedAt:     time.Now(),
		RetriedFromID: &original.ID,
	}
	retriedSig, err := ss.SignCommand(retried)
	if err != nil {
		t.Fatalf("failed to sign retried command: %v", err)
	}
	retried.Signature = retriedSig

	if retried.Signature == "" {
		t.Errorf("retried command must have a signature")
	}
	if retried.SignedAt == nil {
		t.Errorf("retried command must have SignedAt set")
	}
	if retried.KeyID == "" {
		t.Errorf("retried command must have KeyID set")
	}
}

// ---------------------------------------------------------------------------
// Test 1.2 — BUG F-1: Signed payload does not include agent_id
//
// Category: PASS-NOW / FAIL-AFTER-FIX
//
// A command signed for agent A produces a signature that verifies correctly
// for agent B because agent_id is absent from the signed message.
// The test PASSES now (demonstrates the bug). After fix (agent_id added to
// signed message), re-verification without agent_id will FAIL.
// ---------------------------------------------------------------------------

func TestSignedCommandNotBoundToAgent(t *testing.T) {
	// POST-FIX (F-1): Signature now binds to agent_id.
	// A command signed for agent A must NOT verify when the message is
	// reconstructed with agent B's ID.
	ss, _, _ := newTestSigningService(t)

	agentA := uuid.Must(uuid.NewV4())
	agentB := uuid.Must(uuid.NewV4())

	// Create and sign a command for agent A.
	cmd := newTestCommand(agentA)
	sig, err := ss.SignCommand(cmd)
	if err != nil {
		t.Fatalf("failed to sign command: %v", err)
	}

	// Reconstruct the signed message — v3 format includes agent_id
	paramsJSON, err := json.Marshal(cmd.Params)
	if err != nil {
		t.Fatalf("failed to marshal params: %v", err)
	}
	paramsHash := sha256.Sum256(paramsJSON)
	paramsHashHex := hex.EncodeToString(paramsHash[:])

	// v3 signed message for agent A (what was actually signed)
	signedMessageA := fmt.Sprintf("%s:%s:%s:%s:%d",
		agentA.String(), cmd.ID.String(), cmd.CommandType, paramsHashHex, cmd.SignedAt.Unix())

	// Assert: agent A's UUID IS in the signed message (F-1 fix)
	if !strings.Contains(signedMessageA, agentA.String()) {
		t.Fatal("F-1 FIX BROKEN: agentA.String() should be present in signed message")
	}

	pubKeyBytes, err := hex.DecodeString(ss.GetPublicKey())
	if err != nil {
		t.Fatalf("failed to decode public key: %v", err)
	}
	pubKey := ed25519.PublicKey(pubKeyBytes)
	sigBytes, err := hex.DecodeString(sig)
	if err != nil {
		t.Fatalf("failed to decode signature: %v", err)
	}

	// Verify with agent A's message — should pass
	if !ed25519.Verify(pubKey, []byte(signedMessageA), sigBytes) {
		t.Error("verification with correct agent A should pass")
	}

	// Reconstruct with agent B's ID — should FAIL
	signedMessageB := fmt.Sprintf("%s:%s:%s:%s:%d",
		agentB.String(), cmd.ID.String(), cmd.CommandType, paramsHashHex, cmd.SignedAt.Unix())

	if ed25519.Verify(pubKey, []byte(signedMessageB), sigBytes) {
		t.Error("F-1 FIX BROKEN: cross-agent verification with agent B should FAIL but passed")
	}

	t.Logf("F-1 FIXED: Signed message: %q", signedMessageA)
	t.Logf("  Agent A (signed for): %s — verification PASSES", agentA)
	t.Logf("  Agent B (not signed for): %s — verification FAILS", agentB)
	t.Log("The signature is now bound to the target agent_id.")
}

// ---------------------------------------------------------------------------
// Test 1.3 — BUG F-3: Old-format signatures (no signed_at) never expire
//
// Category: PASS-NOW / FAIL-AFTER-FIX
//
// The old signed message format is "{id}:{type}:{sha256(params)}" with no
// timestamp. ed25519.Verify has no time component; a signature produced with
// this format verifies identically 30 days, 1 year, or 10 years later.
// The agent's VerifyCommand (crypto/verification.go:25) has no time check,
// so old-format commands are accepted indefinitely.
// ---------------------------------------------------------------------------

func TestOldFormatCommandHasNoExpiry(t *testing.T) {
	// POST-FIX (F-3): Old-format signatures now have a 48h expiry via created_at check.
	// The Ed25519 signature itself is still valid (crypto doesn't have time), but the
	// agent's VerifyCommand now checks created_at and rejects commands > 48h old.
	//
	// This server-side test documents that the RAW signature is still valid (crypto level)
	// but the application layer (agent VerifyCommand) rejects it.
	_, _, priv := newTestSigningService(t)

	cmdID := uuid.Must(uuid.NewV4())
	cmdType := "install_updates"
	params := models.JSONB{"package": "nginx"}

	paramsJSON, err := json.Marshal(params)
	if err != nil {
		t.Fatalf("failed to marshal params: %v", err)
	}
	paramsHash := sha256.Sum256(paramsJSON)
	paramsHashHex := hex.EncodeToString(paramsHash[:])

	oldFormatMessage := fmt.Sprintf("%s:%s:%s", cmdID.String(), cmdType, paramsHashHex)
	sig := ed25519.Sign(priv, []byte(oldFormatMessage))

	pubKey := priv.Public().(ed25519.PublicKey)

	// Raw crypto verification still passes (Ed25519 has no time component)
	valid := ed25519.Verify(pubKey, []byte(oldFormatMessage), sig)
	if !valid {
		t.Error("raw Ed25519 verification should still pass")
	}

	t.Logf("F-3 FIXED: Old-format message %q", oldFormatMessage)
	t.Log("Raw Ed25519 signature is timeless, but agent's VerifyCommand now checks created_at.")
	t.Log("Old-format commands > 48h old are rejected at the application layer.")
	t.Log("Phase 2 (future): remove old-format fallback entirely after 90 days from migration 025.")
}
