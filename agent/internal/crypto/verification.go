package crypto

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/client"
)

// CommandVerifier handles Ed25519 signature verification for commands
type CommandVerifier struct{}

// NewCommandVerifier creates a new command verifier
func NewCommandVerifier() *CommandVerifier {
	return &CommandVerifier{}
}

// oldFormatMaxAge is the maximum age for old-format commands (no signed_at).
// Phase 1 (F-3 fix): reject old-format commands older than 48h.
// Phase 2 (future): remove old-format fallback entirely after 90 days from migration 025 deployment.
const oldFormatMaxAge = 48 * time.Hour

// VerifyCommand verifies a command using the old signing format (no timestamp).
// Used for backward compatibility with commands signed before key rotation support.
// Format: "{id}:{command_type}:{sha256(params)}"
// F-3 fix: rejects commands older than 48h if CreatedAt is available.
func (v *CommandVerifier) VerifyCommand(cmd client.Command, serverPubKey ed25519.PublicKey) error {
	// F-3 fix: check age using server-side created_at if available
	if cmd.CreatedAt != nil {
		age := time.Since(*cmd.CreatedAt)
		if age > oldFormatMaxAge {
			return fmt.Errorf("command too old: old-format command exceeds 48h age limit (created %v ago)", age.Round(time.Second))
		}
	}

	if cmd.Signature == "" {
		return fmt.Errorf("command missing signature")
	}
	sig, err := hex.DecodeString(cmd.Signature)
	if err != nil {
		return fmt.Errorf("invalid signature encoding: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length: expected %d bytes, got %d", ed25519.SignatureSize, len(sig))
	}
	message, err := v.reconstructMessage(cmd)
	if err != nil {
		return fmt.Errorf("failed to reconstruct message: %w", err)
	}
	if !ed25519.Verify(serverPubKey, message, sig) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

// reconstructMessage recreates the signed message using the old format (no timestamp).
// Format: "{id}:{command_type}:{sha256(params)}"
func (v *CommandVerifier) reconstructMessage(cmd client.Command) ([]byte, error) {
	paramsJSON, err := json.Marshal(cmd.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameters: %w", err)
	}
	paramsHash := sha256.Sum256(paramsJSON)
	paramsHashHex := hex.EncodeToString(paramsHash[:])
	message := fmt.Sprintf("%s:%s:%s", cmd.ID, cmd.Type, paramsHashHex)
	return []byte(message), nil
}

// reconstructMessageV3 recreates the signed message using v3 format (with agent_id + timestamp).
// Format: "{agent_id}:{id}:{command_type}:{sha256(params)}:{unix_timestamp}"
func (v *CommandVerifier) reconstructMessageV3(cmd client.Command) ([]byte, error) {
	if cmd.SignedAt == nil {
		return nil, fmt.Errorf("command SignedAt is nil, cannot reconstruct v3 message")
	}
	if cmd.AgentID == "" {
		return nil, fmt.Errorf("command AgentID is empty, cannot reconstruct v3 message")
	}
	paramsJSON, err := json.Marshal(cmd.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameters: %w", err)
	}
	paramsHash := sha256.Sum256(paramsJSON)
	paramsHashHex := hex.EncodeToString(paramsHash[:])
	message := fmt.Sprintf("%s:%s:%s:%s:%d", cmd.AgentID, cmd.ID, cmd.Type, paramsHashHex, cmd.SignedAt.Unix())
	return []byte(message), nil
}

// reconstructMessageWithTimestamp recreates the signed message using v2 format (timestamp, no agent_id).
// Format: "{id}:{command_type}:{sha256(params)}:{unix_timestamp}"
func (v *CommandVerifier) reconstructMessageWithTimestamp(cmd client.Command) ([]byte, error) {
	if cmd.SignedAt == nil {
		return nil, fmt.Errorf("command SignedAt is nil, cannot reconstruct timestamped message")
	}
	paramsJSON, err := json.Marshal(cmd.Params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal parameters: %w", err)
	}
	paramsHash := sha256.Sum256(paramsJSON)
	paramsHashHex := hex.EncodeToString(paramsHash[:])
	message := fmt.Sprintf("%s:%s:%s:%d", cmd.ID, cmd.Type, paramsHashHex, cmd.SignedAt.Unix())
	return []byte(message), nil
}

// VerifyCommandWithTimestamp verifies a command signature AND validates the signing timestamp.
// Rejects commands signed more than maxAge in the past, or more than clockSkew in the future.
// Uses the new timestamped message format.
// If cmd.SignedAt is nil, falls back to the old (no-timestamp) verification format for backward compatibility.
//
// The default maxAge used by command_handler.go is 4 hours (reduced from 24h in A-2 fix F-4).
// This balances security (shorter replay window) against operational flexibility (agents
// polling every few minutes have ample time to receive and verify commands).
// See commandMaxAge constant in orchestrator/command_handler.go.
func (v *CommandVerifier) VerifyCommandWithTimestamp(
	cmd client.Command,
	serverPubKey ed25519.PublicKey,
	maxAge time.Duration,
	clockSkew time.Duration,
) error {
	if cmd.SignedAt == nil {
		// No timestamp — fall back to old format (backward compat)
		if teeLogger != nil {
			teeLogger.Warning("agent", "crypto", "verification", "deprecated command format: oldest format, no signed_at", map[string]interface{}{
				"command_id": cmd.ID,
			})
		}
		return v.VerifyCommand(cmd, serverPubKey)
	}

	// Validate timestamp window
	now := time.Now().UTC()
	age := now.Sub(*cmd.SignedAt)
	if age > maxAge {
		return fmt.Errorf("command timestamp too old: signed %v ago (max %v)", age.Round(time.Second), maxAge)
	}
	if age < -clockSkew {
		return fmt.Errorf("command timestamp is in the future: %v ahead (max skew %v)", (-age).Round(time.Second), clockSkew)
	}

	// Verify signature
	if cmd.Signature == "" {
		return fmt.Errorf("command missing signature")
	}
	sig, err := hex.DecodeString(cmd.Signature)
	if err != nil {
		return fmt.Errorf("invalid signature encoding: %w", err)
	}
	if len(sig) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length: expected %d bytes, got %d", ed25519.SignatureSize, len(sig))
	}

	// Try v3 format first (with agent_id) if AgentID is present
	if cmd.AgentID != "" {
		message, err := v.reconstructMessageV3(cmd)
		if err != nil {
			return fmt.Errorf("failed to reconstruct v3 message: %w", err)
		}
		if ed25519.Verify(serverPubKey, message, sig) {
			return nil // v3 verification succeeded
		}
		// v3 failed — try v2 as fallback (server may not have been upgraded yet)
		if teeLogger != nil {
			teeLogger.Warning("agent", "crypto", "verification", "v3 verification failed, trying v2", map[string]interface{}{
				"command_id": cmd.ID,
			})
		}
	}

	// v2 format: timestamp but no agent_id (backward compat)
	if cmd.AgentID == "" {
		if teeLogger != nil {
			teeLogger.Warning("agent", "crypto", "verification", "deprecated command format: v2, no agent_id", map[string]interface{}{
				"command_id": cmd.ID,
			})
		}
	}
	message, err := v.reconstructMessageWithTimestamp(cmd)
	if err != nil {
		return fmt.Errorf("failed to reconstruct timestamped message: %w", err)
	}
	if !ed25519.Verify(serverPubKey, message, sig) {
		return fmt.Errorf("signature verification failed")
	}
	return nil
}

// CheckKeyRotation checks if the key_id in a command is cached locally.
// If not cached, it fetches all active keys from the server and caches them.
// Returns the correct public key to use for verifying this command.
func (v *CommandVerifier) CheckKeyRotation(keyID string, serverURL string) (ed25519.PublicKey, bool, error) {
	if keyID == "" {
		// No key_id in command — backward compat: use primary cached key
		key, err := LoadCachedPublicKey()
		return key, false, err
	}

	// Check if this key is already cached
	if IsKeyIDCached(keyID) {
		key, err := LoadCachedPublicKeyByID(keyID)
		return key, false, err
	}

	// Key not cached — fetch all active keys from server
	if teeLogger != nil {
		teeLogger.Info("agent", "crypto", "verification", "key not cached, fetching active keys from server", map[string]interface{}{
			"key_id": keyID,
		})
	}
	entries, err := FetchAndCacheAllActiveKeys(serverURL)
	if err != nil {
		// Forward-only (SEC-028): the active-set fetch failed, but trusting the
		// primary cache here with NO age bound is a wider hole than the primary
		// fetch path (FetchAndCacheServerPublicKey) closes on its own. Apply the
		// same bounded-stale ceiling: serve the primary only within the window,
		// fail closed past it. A primary the server rotated OUT must not verify a
		// key_id'd command just because /public-keys is unreachable.
		key, loadErr := LoadCachedPublicKey()
		if loadErr != nil {
			return nil, false, fmt.Errorf("key %s not cached and active-set fetch failed: fetch=%v, load=%v", keyID, err, loadErr)
		}
		meta, metaErr := loadCacheMetadata()
		if metaErr != nil {
			return nil, false, fmt.Errorf("active-set fetch failed and primary cache age unknown (no metadata); refusing: %w", err)
		}
		age := time.Since(meta.CachedAt)
		window := staleKeyMaxAge()
		if age > window {
			if teeLogger != nil {
				teeLogger.Error("agent", "crypto", "verification", "active-set fetch failed and primary cache stale; refusing key_id'd command", map[string]interface{}{
					"key_id":    keyID,
					"cache_age": age.Round(time.Hour).String(),
					"max_stale": window.String(),
					"error":     err.Error(),
				})
			}
			return nil, false, fmt.Errorf("active-set fetch failed and primary cache stale (age %s > %s); refusing: %w",
				age.Round(time.Hour), window, err)
		}
		if teeLogger != nil {
			teeLogger.Warning("agent", "crypto", "verification", "active-set fetch failed, serving bounded-stale primary", map[string]interface{}{
				"key_id":    keyID,
				"cache_age": age.Round(time.Minute).String(),
				"max_stale": window.String(),
				"error":     err.Error(),
			})
		}
		return key, false, nil
	}

	// Check if we got the requested key
	for _, entry := range entries {
		if entry.KeyID == keyID {
			key, err := LoadCachedPublicKeyByID(keyID)
			return key, true, err
		}
	}

	// Requested key_id was fetched but is not in the server's active set. A key
	// the server has rotated OUT is dead — forward-only doctrine refuses it
	// rather than silently falling back to the primary key (SEC-028). The caller
	// surfaces this as a command-verification failure.
	if teeLogger != nil {
		teeLogger.Error("agent", "crypto", "verification", "command key_id not in server active set, refusing", map[string]interface{}{
			"key_id": keyID,
		})
	}
	return nil, false, fmt.Errorf("key %s not in server active set", keyID)
}

// VerifyCommandBatch verifies multiple commands efficiently
func (v *CommandVerifier) VerifyCommandBatch(
	commands []client.Command,
	serverPubKey ed25519.PublicKey,
) []error {
	errors := make([]error, len(commands))
	for i, cmd := range commands {
		errors[i] = v.VerifyCommand(cmd, serverPubKey)
	}
	return errors
}

// ExtractCommandIDFromSignature attempts to verify a signature and returns the command ID
func (v *CommandVerifier) ExtractCommandIDFromSignature(
	signature string,
	expectedMessage string,
	serverPubKey ed25519.PublicKey,
) (string, error) {
	sig, err := hex.DecodeString(signature)
	if err != nil {
		return "", fmt.Errorf("invalid signature encoding: %w", err)
	}
	if !ed25519.Verify(serverPubKey, []byte(expectedMessage), sig) {
		return "", fmt.Errorf("signature verification failed")
	}
	return "", nil
}
