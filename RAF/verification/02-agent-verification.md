# Agent-Side Verification

**Agent verifies commands and binaries before execution.**

---

## Overview

Agent verifies Ed25519 signatures, timestamps, and nonces before executing commands.

**Cross-references:**
- [verification/01-signing-pipeline](01-signing-pipeline.md) (server-side signing)
- [verification/03-key-rotation](03-key-rotation.md) (key rotation)
- [verification/04-replay-protection](04-replay-protection.md) (replay protection)

---

## Public Key Caching (TOFU)

**Method:** Trust On First Use — TTL+key_id cache with rotation awareness

**File:** `agent/internal/crypto/pubkey.go`

```go
func FetchAndCacheServerPublicKey(serverURL string) (ed25519.PublicKey, error) {
    // 1. Check if cache is valid (TTL + key_id match)
    if meta, err := loadCacheMetadata(); err == nil && meta.KeyID != "" && !meta.IsExpired() {
        if cachedKey, loadErr := LoadCachedPublicKey(); loadErr == nil {
            return cachedKey, nil
        }
    }

    // 2. Fetch from server GET /api/v1/public-key
    pubKeyBytes, err := httpGetPublicKey(serverURL)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch public key: %w", err)
    }

    // 3. Cache to disk
    if err := cachePublicKey(pubKeyBytes); err != nil {
        fmt.Printf("Warning: Failed to cache public key: %v\n", err)
    }

    return pubKeyBytes, nil
}
```

---

## Command Verification

**Method:** Verify v3 signature with timestamp, falls back to older formats for backward compatibility

**File:** `agent/internal/crypto/verification.go`

```go
func (v *CommandVerifier) VerifyCommandWithTimestamp(
    cmd client.Command,
    serverPubKey ed25519.PublicKey,
    maxAge time.Duration,
    clockSkew time.Duration,
) error {
    // 1. If cmd.SignedAt is nil, fall back to oldest format (backward compat)
    if cmd.SignedAt == nil {
        fmt.Printf("[WARNING] [agent] [crypto] command_uses_oldest_format command_id=%s no_signed_at=true upgrade_server_recommended\n", cmd.ID)
        return v.VerifyCommand(cmd, serverPubKey)
    }

    // 2. Validate timestamp window
    now := time.Now().UTC()
    age := now.Sub(*cmd.SignedAt)
    if age > maxAge {
        return fmt.Errorf("command timestamp too old: signed %v ago (max %v)", age.Round(time.Second), maxAge)
    }
    if age < -clockSkew {
        return fmt.Errorf("command timestamp is in the future: %v ahead (max skew %v)", (-age).Round(time.Second), clockSkew)
    }

    // 3. Try v3 format first (with agent_id) if AgentID is present
    if cmd.AgentID != "" {
        message, err := v.reconstructMessageV3(cmd)
        if err != nil {
            return fmt.Errorf("failed to reconstruct v3 message: %w", err)
        }
        if ed25519.Verify(serverPubKey, message, sig) {
            return nil // v3 verification succeeded
        }
        // v3 failed — try v2 as fallback
    }

    // 4. v2 format: timestamp but no agent_id (backward compat)
    message, err := v.reconstructMessageWithTimestamp(cmd)
    if err != nil {
        return fmt.Errorf("failed to reconstruct timestamped message: %w", err)
    }
    if !ed25519.Verify(serverPubKey, message, sig) {
        return errors.New("signature verification failed")
    }
    return nil
}
```

**Verification Modes:**
- **Strict:** Reject all verification failures (default)
- **Warning:** Log failure but execute command
- **Disabled:** Skip verification entirely

**Fallback Chain:** v3 (agent_id + timestamp) → v2 (timestamp only) → oldest (no timestamp)
```

**Verification Modes:**
- **Strict:** Reject all verification failures (default)
- **Warning:** Log failure but execute command
- **Disabled:** Skip verification entirely

---

## Binary Verification

**Method:** Verify checksum and Ed25519 signature before installation

**File:** `agent/internal/orchestrator/update_handler.go`

```go
func (h *UpdateHandler) verifyDownload(binaryData []byte, checksum string) error {
    // 1. Verify checksum
    computedChecksum := sha256.Sum256(binaryData)
    if hex.EncodeToString(computedChecksum[:]) != checksum {
        return errors.New("checksum mismatch")
    }

    // 2. Verify Ed25519 signature
    signature, err := h.downloadBinarySignature()
    if err != nil {
        return err
    }

    publicKey, err := h.LoadCachedPublicKey()
    if err != nil {
        return err
    }

    if !ed25519.Verify(publicKey, binaryData, []byte(signature)) {
        return errors.New("signature verification failed")
    }

    return nil
}
```

---

## Cross-References

- **Server signing** → [verification/01-signing-pipeline](01-signing-pipeline.md)
- **Key rotation** → [verification/03-key-rotation](03-key-rotation.md)
- **Replay protection** → [verification/04-replay-protection](04-replay-protection.md)
- **Install script** → [flows/01-registration](../flows/01-registration.md) (TOFU key caching)

---

*Last reviewed: 2026-06-14*