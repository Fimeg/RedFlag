# Ed25519 Signing Pipeline

**Server-side command and binary signing with Ed25519.**

---

## Overview

All commands and binaries are signed with Ed25519 private key on the server. Agents verify signatures before execution.

**Cross-references:**
- [verification/02-agent-verification](02-agent-verification.md) (agent-side verification)
- [verification/03-key-rotation](03-key-rotation.md) (key rotation support)
- [security/01-trust-boundaries](../security/01-trust-boundaries.md) (signature as trust boundary)

---

## Key Generation

**Method:** Server startup automatic key registration

**File:** `server/internal/services/signing.go`

```go
func (s *SigningService) InitializePrimaryKey(ctx context.Context) error {
    // 1. Get current key fingerprint (SHA-256 of public key, truncated)
    keyID := s.GetCurrentKeyID()
    publicKeyHex := s.GetPublicKeyHex()

    // 2. Query next version number from database
    nextVersion, err := s.signingKeyQueries.GetNextVersion(ctx)
    if err != nil {
        nextVersion = 1
    }

    // 3. Insert key (ON CONFLICT DO NOTHING — safe on every startup)
    if err := s.signingKeyQueries.InsertSigningKey(ctx, keyID, publicKeyHex, nextVersion); err != nil {
        return fmt.Errorf("failed to insert signing key: %w", err)
    }

    // 4. Set as primary
    if err := s.signingKeyQueries.SetPrimaryKey(ctx, keyID); err != nil {
        return fmt.Errorf("failed to set primary key: %w", err)
    }

    return nil
}
```

---

## Command Signing

**Method:** v3 message format with agent_id binding

**File:** `server/internal/services/signing.go`

```go
func (s *SigningService) SignCommand(cmd *models.AgentCommand) (string, error) {
    // 1. Record signing time and key identity
    now := time.Now().UTC()
    cmd.SignedAt = &now
    cmd.KeyID = s.GetCurrentKeyID()

    // 2. Serialize params and hash
    paramsJSON, _ := json.Marshal(cmd.Params)
    paramsHash := sha256.Sum256(paramsJSON)
    paramsHashHex := hex.EncodeToString(paramsHash[:])

    // 3. Create v3 message format
    // agent_id binding prevents cross-agent replay (F-1 fix)
    message := fmt.Sprintf("%s:%s:%s:%s:%d",
        cmd.AgentID.String(),
        cmd.ID.String(),
        cmd.CommandType,
        paramsHashHex,
        now.Unix())

    // 4. Sign with Ed25519
    signature := ed25519.Sign(s.privateKey, []byte(message))
    return hex.EncodeToString(signature), nil
}
```

**v3 Message Format Benefits:**
- **Agent binding:** Includes agent_id to prevent command relay attacks
- **Timestamp:** Prevents replay attacks (4-hour max age)
- **Parameter hash:** Hides full parameter data while allowing verification

---

## Binary Signing

**Method:** BuildOrchestrator signs binaries at startup

**File:** `server/internal/services/build_orchestrator.go`

```go
func (s *BuildOrchestratorService) BuildAndSignAgent(version, platform, architecture string) (*models.AgentUpdatePackage, error) {
    // 1. Load binary from disk
    binaryName := "redflag-agent"
    if strings.HasPrefix(platform, "windows") {
        binaryName += ".exe"
    }

    binaryPath := filepath.Join(s.agentDir, "binaries", platform+"-"+architecture, binaryName)

    if _, err := os.Stat(binaryPath); os.IsNotExist(err) {
        return nil, fmt.Errorf("binary not found for platform %s: %w", platform, err)
    }

    if s.signingService.IsEnabled() {
        // 2. Compute checksum and sign
        signedPackage, err := s.signingService.SignFile(binaryPath)
        if err != nil {
            return nil, fmt.Errorf("failed to sign agent binary: %w", err)
        }

        // 3. Set metadata
        signedPackage.Version = version
        signedPackage.Platform = platform
        signedPackage.Architecture = architecture

        // 4. Store in database
        err = s.packageQueries.StoreSignedPackage(signedPackage)
        if err != nil {
            return nil, fmt.Errorf("failed to store signed package: %w", err)
        }

        log.Printf("[INFO] [server] [build_orchestrator] package_signed id=%s version=%s platform=%s arch=%s", signedPackage.ID, version, platform, architecture)
        return signedPackage, nil
    } else {
        log.Printf("Signing disabled, creating unsigned package entry")
        // Create unsigned package entry for backward compatibility
        unsignedPackage := &models.AgentUpdatePackage{
            ID:           uuid.New(),
            Version:      version,
            Platform:     platform,
            Architecture: architecture,
            BinaryPath:   binaryPath,
            Signature:    "",
            Checksum:     "",
            CreatedBy:    "build-orchestrator",
            IsActive:     true,
        }

        // Get file info
        if info, err := os.Stat(binaryPath); err == nil {
            unsignedPackage.FileSize = info.Size()
        }

        // Store unsigned package
        err := s.packageQueries.StoreSignedPackage(unsignedPackage)
        if err != nil {
            return nil, fmt.Errorf("failed to store unsigned package: %w", err)
        }

        return unsignedPackage, nil
    }
}
```

---

## Footer: Assumptions & Connections

**Assumption:** Ed25519 signing is enabled when `REDFLAG_SIGNING_PRIVATE_KEY` environment variable is set.

**Connection:** [verification/02-agent-verification](02-agent-verification.md) (agent-side signature verification)

**Connection:** [verification/03-key-rotation](03-key-rotation.md) (multi-key rotation support)

**Connection:** [security/01-trust-boundaries](../security/01-trust-boundaries.md) (cryptographic trust boundary)

---

*Last reviewed: 2026-06-14*