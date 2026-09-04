package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/client"
)

// signCommand is a test helper that signs a command using the v3 format (with agent_id).
// Format: "{agent_id}:{id}:{type}:{sha256(params)}:{unix_timestamp}"
// Falls back to v2 format if cmd.AgentID is empty (for backward compat tests).
func signCommand(t *testing.T, privKey ed25519.PrivateKey, cmd *client.Command, signedAt time.Time) string {
	t.Helper()
	paramsJSON, err := json.Marshal(cmd.Params)
	if err != nil {
		t.Fatalf("failed to marshal params: %v", err)
	}
	paramsHash := sha256.Sum256(paramsJSON)
	paramsHashHex := hex.EncodeToString(paramsHash[:])
	var message string
	if cmd.AgentID != "" {
		// v3 format with agent_id
		message = fmt.Sprintf("%s:%s:%s:%s:%d", cmd.AgentID, cmd.ID, cmd.Type, paramsHashHex, signedAt.Unix())
	} else {
		// v2 format without agent_id (backward compat)
		message = fmt.Sprintf("%s:%s:%s:%d", cmd.ID, cmd.Type, paramsHashHex, signedAt.Unix())
	}
	sig := ed25519.Sign(privKey, []byte(message))
	return hex.EncodeToString(sig)
}

// signCommandV2 explicitly signs using v2 format (no agent_id) for backward compat tests.
func signCommandV2(t *testing.T, privKey ed25519.PrivateKey, cmd *client.Command, signedAt time.Time) string {
	t.Helper()
	paramsJSON, err := json.Marshal(cmd.Params)
	if err != nil {
		t.Fatalf("failed to marshal params: %v", err)
	}
	paramsHash := sha256.Sum256(paramsJSON)
	paramsHashHex := hex.EncodeToString(paramsHash[:])
	message := fmt.Sprintf("%s:%s:%s:%d", cmd.ID, cmd.Type, paramsHashHex, signedAt.Unix())
	sig := ed25519.Sign(privKey, []byte(message))
	return hex.EncodeToString(sig)
}

// signCommandOld is a test helper that signs using the old format (no timestamp).
// Format: "{id}:{type}:{sha256(params)}"
func signCommandOld(t *testing.T, privKey ed25519.PrivateKey, cmd *client.Command) string {
	t.Helper()
	paramsJSON, _ := json.Marshal(cmd.Params)
	paramsHash := sha256.Sum256(paramsJSON)
	paramsHashHex := hex.EncodeToString(paramsHash[:])
	message := fmt.Sprintf("%s:%s:%s", cmd.ID, cmd.Type, paramsHashHex)
	sig := ed25519.Sign(privKey, []byte(message))
	return hex.EncodeToString(sig)
}

func generateKeyPair(t *testing.T) (ed25519.PublicKey, ed25519.PrivateKey) {
	t.Helper()
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}
	return pub, priv
}

func TestVerifyCommandWithTimestamp_ValidRecent(t *testing.T) {
	pub, priv := generateKeyPair(t)
	v := NewCommandVerifier()

	now := time.Now().UTC()
	cmd := client.Command{
		ID:      "test-cmd-1",
		Type:    "scan_updates",
		Params:  map[string]interface{}{"target": "apt"},
		AgentID: "agent-001",
	}
	cmd.SignedAt = &now
	cmd.Signature = signCommand(t, priv, &cmd, now)

	err := v.VerifyCommandWithTimestamp(cmd, pub, 24*time.Hour, 5*time.Minute)
	if err != nil {
		t.Errorf("expected valid recent command to pass, got: %v", err)
	}
}

func TestVerifyCommandWithTimestamp_TooOld(t *testing.T) {
	pub, priv := generateKeyPair(t)
	v := NewCommandVerifier()

	oldTime := time.Now().UTC().Add(-2 * time.Hour)
	cmd := client.Command{
		ID:      "test-cmd-2",
		Type:    "scan_updates",
		Params:  map[string]interface{}{},
		AgentID: "agent-002",
	}
	cmd.SignedAt = &oldTime
	cmd.Signature = signCommand(t, priv, &cmd, oldTime)

	// With maxAge of 1 hour — should fail
	err := v.VerifyCommandWithTimestamp(cmd, pub, 1*time.Hour, 5*time.Minute)
	if err == nil {
		t.Error("expected old command to fail timestamp check, but it passed")
	}
}

func TestVerifyCommandWithTimestamp_FutureBeyondSkew(t *testing.T) {
	pub, priv := generateKeyPair(t)
	v := NewCommandVerifier()

	futureTime := time.Now().UTC().Add(10 * time.Minute)
	cmd := client.Command{
		ID:      "test-cmd-3",
		Type:    "scan_updates",
		Params:  map[string]interface{}{},
		AgentID: "agent-003",
	}
	cmd.SignedAt = &futureTime
	cmd.Signature = signCommand(t, priv, &cmd, futureTime)

	// With clockSkew of 5 min — should fail (10 min future)
	err := v.VerifyCommandWithTimestamp(cmd, pub, 24*time.Hour, 5*time.Minute)
	if err == nil {
		t.Error("expected future-dated command to fail, but it passed")
	}
}

func TestVerifyCommandWithTimestamp_FutureWithinSkew(t *testing.T) {
	pub, priv := generateKeyPair(t)
	v := NewCommandVerifier()

	futureTime := time.Now().UTC().Add(2 * time.Minute) // within 5 min skew
	cmd := client.Command{
		ID:      "test-cmd-4",
		Type:    "scan_updates",
		Params:  map[string]interface{}{},
		AgentID: "agent-004",
	}
	cmd.SignedAt = &futureTime
	cmd.Signature = signCommand(t, priv, &cmd, futureTime)

	err := v.VerifyCommandWithTimestamp(cmd, pub, 24*time.Hour, 5*time.Minute)
	if err != nil {
		t.Errorf("expected command within clock skew to pass, got: %v", err)
	}
}

func TestVerifyCommandWithTimestamp_BackwardCompatNoTimestamp(t *testing.T) {
	pub, priv := generateKeyPair(t)
	v := NewCommandVerifier()

	// Set CreatedAt to recent time so F-3 48h check passes
	createdAt := time.Now().Add(-1 * time.Hour)
	cmd := client.Command{
		ID:        "test-cmd-5",
		Type:      "scan_updates",
		Params:    map[string]interface{}{"pkg": "nginx"},
		SignedAt:  nil, // no timestamp — old server
		CreatedAt: &createdAt,
	}
	cmd.Signature = signCommandOld(t, priv, &cmd)

	// Should fall back to old verification and succeed (within 48h)
	err := v.VerifyCommandWithTimestamp(cmd, pub, 4*time.Hour, 5*time.Minute)
	if err != nil {
		t.Errorf("expected backward-compat (no timestamp) command to pass, got: %v", err)
	}
}

func TestVerifyCommandWithTimestamp_WrongKey(t *testing.T) {
	_, priv := generateKeyPair(t)
	wrongPub, _ := generateKeyPair(t)
	v := NewCommandVerifier()

	now := time.Now().UTC()
	cmd := client.Command{
		ID:      "test-cmd-6",
		Type:    "scan_updates",
		Params:  map[string]interface{}{},
		AgentID: "agent-006",
	}
	cmd.SignedAt = &now
	cmd.Signature = signCommand(t, priv, &cmd, now)

	err := v.VerifyCommandWithTimestamp(cmd, wrongPub, 24*time.Hour, 5*time.Minute)
	if err == nil {
		t.Error("expected wrong-key verification to fail, but it passed")
	}
}

func TestVerifyCommand_BackwardCompat(t *testing.T) {
	pub, priv := generateKeyPair(t)
	v := NewCommandVerifier()

	// Set CreatedAt to recent time so F-3 48h check passes
	createdAt := time.Now().Add(-1 * time.Hour)
	cmd := client.Command{
		ID:        "test-cmd-7",
		Type:      "install_updates",
		Params:    map[string]interface{}{"package": "nginx", "version": "1.20.0"},
		CreatedAt: &createdAt,
	}
	cmd.Signature = signCommandOld(t, priv, &cmd)

	if err := v.VerifyCommand(cmd, pub); err != nil {
		t.Errorf("expected old-format verification to pass, got: %v", err)
	}
}
