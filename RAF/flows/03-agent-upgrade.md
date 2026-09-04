> **⚠ STALE — known deviations from current code (flagged 2026-06-16 by Vanguard).**
> This flow doc describes code that no longer exists. It will be rewritten for v0.3.0
> when the Windows mutation helper (SEC-030) lands. Until then, treat the below as
> historical reference only — **not design of record**.
>
> **Known deviations:**
> 1. `Params: {"version": "latest"}` — server now resolves to `serverVersion.AgentVersion`
>    before sending (CRITICAL-009 fix). `"latest"` rendered as `vlatest` and broke the gate.
> 2. File paths are wrong. `agent/internal/orchestrator/update_handler.go` does not exist.
>    Actual handler: `agent/internal/handlers/agent_update.go`.
> 3. **Two execution paths now exist** (not one "atomic swap"):
>    - **Linux:** capability-token helper owns the swap (`installAgentViaHelper`). Agent holds
>      no sudo for cp/chmod/restart.
>    - **Non-Linux (Windows):** in-process binary swap + detached `sc` restart fallback.
>      No capability token. Tracked as doctrine gap in SEC-030.
> 4. **Watchdog/rollback removed.** `runUpdateWatchdog()` and `rollbackUpdate()` were
>    intentionally deleted — "could not survive systemd's SIGTERM and has been removed.
>    Server-side reconcileAgentUpdates closes the command." The code samples below show
>    functions that do not exist.
> 5. Capability-token authorization is not mentioned at all — it is now the load-bearing
>    authorization for Linux self-upgrade.

---

# Agent Self-Upgrade Flow

**7-step agent upgrade with rollback and verification.**

---

## Overview

Agents can self-update without manual intervention. The flow includes nonce validation, checksum verification, Ed25519 signature verification, atomic binary swap, and automatic rollback on failure.

**Cross-references:**
- `flows/02-command-execution.md` (command dispatch)
- `verification/01-signing-pipeline.md` (binary signing)
- `verification/02-agent-verification.md` (binary verification)
- `verification/04-replay-protection.md` (nonce validation)
- `security/02-authentication-stack.md` (machine binding)

---

## Step-by-Step Flow

### 1. Admin Triggers Update

**File:** `server/internal/api/handlers/agent_updates.go`

```go
func (h *AgentUpdateHandler) TriggerAgentUpdate(w http.ResponseWriter, r *http.Request) {
    // 1. Validate agent ID
    agentID := r.URL.Query().Get("agent_id")
    if agentID == "" {
        http.Error(w, "agent_id required", http.StatusBadRequest)
        return
    }

    // 2. Create signed update_agent command
    cmd := &Command{
        AgentID: agentID,
        ID:      uuid.New(),
        Type:    "update_agent",
        Params:  `{"version": "latest"}`,
    }

    // 3. Generate nonce (2× check-in interval)
    nonce := h.nonceService.GenerateNonce()

    // 4. Sign command with Ed25519
    signature := h.signingService.SignCommand(cmd, nonce)

    // 5. Store command in database
    h.db.CreateCommand(cmd, signature)

    logSecurityEvent("[operation] [server] [update] Triggered update for agent:", agentID)
}
```

---

### 2. Agent Receives Command

**File:** `agent/internal/agent/loop.go`

```go
commands, err := agent.client.GetCommands(ctx, agent.agentID, agent.machineID, agent.metrics)
for _, cmd := range commands {
    if cmd.Type == "update_agent" {
        // Validate nonce first
        if err := agent.validateNonce(cmd); err != nil {
            logSecurityEvent("[security] [agent] [nonce] Nonce validation failed:", err)
            continue
        }

        // Execute update_agent command
        result := agent.orchestrator.ExecuteCommand(cmd)
    }
}
```

---

### 3. Verify Command Nonce

**File:** `agent/internal/orchestrator/command_handler.go`

```go
func (c *CommandHandler) validateNonce(cmd *Command) error {
    nonce := cmd.Nonce
    maxAge := time.Duration(c.timeoutConfig.NonceMaxAge)

    if time.Since(cmd.CreatedAt) > maxAge {
        return errors.New("nonce expired")
    }

    return nil
}
```

---

### 4. Verify Command Signature

**File:** `agent/internal/crypto/verification.go`

```go
func (v *Verifier) VerifyCommand(cmd *Command) error {
    // 1. Parse signature from v3 format
    parts := strings.Split(cmd.Signature, ":")
    if len(parts) != 6 {
        return errors.New("invalid signature format")
    }

    // 2. Verify Ed25519 signature
    agentID, id, cmdType, paramsHash, timestamp := parts[0], parts[1], parts[2], parts[3], parts[4]
    expected := fmt.Sprintf("%s:%s:%s:%s:%s", agentID, id, cmdType, paramsHash, timestamp)

    if !ed25519.Verify(v.serverPublicKey, []byte(expected), []byte(parts[5])) {
        return errors.New("signature verification failed")
    }

    return nil
}
```

---

### 5. Download Agent Binary

**Endpoint:** `GET /api/v1/downloads/agent?version=latest`

**File:** `server/internal/api/handlers/downloads.go`

```go
func (h *DownloadHandler) DownloadAgent(w http.ResponseWriter, r *http.Request) {
    platform := r.URL.Query().Get("platform")
    version := r.URL.Query().Get("version")

    // 1. Fetch signed package from DB
    signedPackage := h.db.GetSignedPackage(platform, version)

    if signedPackage == nil {
        http.Error(w, "package not found", http.StatusNotFound)
        return
    }

    // 2. Serve binary with checksum + signature headers
    binaryData, _ := h.fs.ReadFile(signedPackage.BinaryPath)
    w.Header().Set("Content-Type", "application/octet-stream")
    w.Header().Set("X-Content-SHA256", signedPackage.Checksum)
    w.Header().Set("X-Content-Signature", signedPackage.Signature)
    w.Write(binaryData)
}
```

---

### 6. Verify Checksum and Signature

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

### 7. Atomic Install with Backup

**File:** `agent/internal/orchestrator/update_handler.go`

```go
func (h *UpdateHandler) atomicInstall(binaryData []byte) error {
    binaryPath := "/usr/local/bin/redflag-agent"

    // 1. Create backup
    backupPath := binaryPath + ".bak"
    if err := os.Rename(binaryPath, backupPath); err != nil {
        return err
    }

    // 2. Atomic install (write to temp file, then rename)
    tempPath := binaryPath + ".tmp"
    if err := os.WriteFile(tempPath, binaryData, 0755); err != nil {
        // Rollback: restore backup
        os.Rename(backupPath, binaryPath)
        return err
    }

    os.Rename(tempPath, binaryPath)

    // 3. Restart service
    return h.restartService()
}
```

---

## Watchdog and Rollback

**File:** `agent/internal/orchestrator/update_handler.go`

```go
func (a *Agent) runUpdateWatchdog() {
    // 1. Start 15-minute watchdog (configurable)
    done := make(chan bool)
    go func() {
        time.Sleep(15 * time.Minute)  // Default: operational.agent_update_timeout_minutes
        done <- true
    }()

    // 2. Wait for update completion
    select {
    case <-done:
        // Update completed or timed out
        logSecurityEvent("[operation] [agent] [update] Watchdog timeout, rolling back")
        a.rollbackUpdate()

    case <-a.updateCompleteChan:
        // Update acknowledged by server
        logSecurityEvent("[operation] [agent] [update] Server acknowledged new version")
        return
    }
}

func (a *Agent) rollbackUpdate() {
    backupPath := "/usr/local/bin/redflag-agent.bak"
    binaryPath := "/usr/local/bin/redflag-agent"

    // Restore backup
    os.Rename(backupPath, binaryPath)

    // Restart old version
    a.restartService()

    // Report timeout
    a.reportUpdateTimeout()
}
```

---

## Footer: Assumptions & Connections

**Assumption:** Self-upgrade is a privileged operation — requires admin approval and nonce validation.

**Connection:** Nonce validation (`verification/04-replay-protection.md`) prevents replay attacks on update commands.

**Connection:** Machine binding (`security/04-machine-binding.md`) ensures only authorized agent can receive updates.

**Connection:** Atomic install (`flows/03-agent-upgrade.md`) implements ETHOS #4 (idempotency).

**Connection:** Backup mechanism (`flows/03-agent-upgrade.md`) enables rollback on failure.

**Connection:** Watchdog timeout (`flows/03-agent-upgrade.md`) aligns with `operational.agent_update_timeout_minutes` setting.

---

*Last reviewed: 2026-05-26*
