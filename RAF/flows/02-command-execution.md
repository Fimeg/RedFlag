# Command Execution Flow

**Polling loop, command dispatch, and at-least-once delivery.**

---

## Overview

Agents poll server for commands every 5 minutes (configurable). Commands are verified, deduplicated, executed, and results reported.

**Cross-references:**
- `flows/01-registration.md` (agent registration)
- `verification/01-signing-pipeline.md` (command signing)
- `verification/02-agent-verification.md` (command verification)
- `verification/04-replay-protection.md` (nonce validation)
- `flows/04-heartbeat.md` (system events)
- `core/01-ethos.md` (principle #3: Assume Failure)

---

## Polling Loop

**File:** `agent/internal/agent/loop.go`

```go
func RunPollingLoop(loopCtx *LoopContext) error {
    ctx := loopCtx
    var consecutiveFailures int
    // Seed RNG for jitter
    rand.New(rand.NewSource(time.Now().UnixNano()))

    for {
        select {
        case <-ctx.Ctx.Done():
            return ctx.Ctx.Err()
        default:
        }

        // 1. Calculate jitter (avoid thundering herd)
        jitter := time.Duration(rand.Int63n(int64(ctx.Cfg.CheckInInterval) / 2))
        sleepWithContext(ctx.Ctx, jitter)

        // 2. Send buffered events (error transparency)
        ctx.APIClient.SendBufferedEvents(ctx.Cfg.AgentID)

        // 3. Poll for commands — auth failures are typed sentinel errors
        response, err := ctx.APIClient.GetCommands(ctx.Cfg.AgentID, metrics)
        if err != nil {
            switch {
            case errors.Is(err, client.ErrMachineMismatch):
                // Terminal: config moved/copied. Loud critical event, keep
                // polling so agent stays visible. Human must re-register.
                log.Printf("[ERROR] [agent] [auth] machine_id_mismatch ...")
            case errors.Is(err, client.ErrUnauthorized) && ctx.Cfg.RefreshToken != "":
                // JWT expired — auto-renew with the refresh token (machine-bound)
                renewErr := ctx.APIClient.RenewToken(...)
                switch {
                case renewErr == nil:
                    ctx.Cfg.Token = ctx.APIClient.GetToken()
                    // Persist rotated refresh token if server returned one
                    if rt := ctx.APIClient.GetRefreshToken(); rt != "" {
                        ctx.Cfg.RefreshToken = rt
                    }
                    ctx.Cfg.Save(...)
                    consecutiveFailures = 0
                    continue
                case errors.Is(renewErr, client.ErrRefreshTokenInvalid):
                    // Terminal: refresh token revoked/expired. Critical event.
                default:
                    // Transient (network, 502). Fall through to backoff.
                }
            }
            // Exponential backoff: 10s → 20s → 40s → ... → 5min cap
            backoff := calculateBackoff(consecutiveFailures)
            consecutiveFailures++
            sleepWithContext(ctx.Ctx, backoff)
            continue
        }

        // Reset backoff on success
        consecutiveFailures = 0

        // 4. Process each command
        for _, cmd := range response.Commands { ... }
    }
}
```

---

## Command Dispatch

**File:** `agent/internal/orchestrator/system_scanner.go`

```go
func (o *Orchestrator) ExecuteCommand(cmd *Command) *CommandResult {
    result := &CommandResult{
        CommandID: cmd.ID,
        Status:    "pending",
    }

    switch cmd.Type {
    // Scanner commands
    case "scan_apt":
        result = o.scanAPT(cmd)
    case "scan_dnf":
        result = o.scanDNF(cmd)
    case "scan_docker":
        result = o.scanDocker(cmd)
    case "scan_windows":
        result = o.scanWindows(cmd)

    // Update commands
    case "update_agent":
        result = o.updateAgent(cmd)

    // System commands
    case "reboot":
        result = o.reboot(cmd)

    default:
        result.Status = "failed"
        result.Error = "unknown command type"
    }

    return result
}
```

---

## At-Least-Once Delivery

**Receipt Tracking:**
```go
// agent/internal/orchestrator/command_handler.go
func (c *CommandHandler) recordReceipt(commandID string) {
    receipts := c.loadReceipts()
    receipts[commandID] = time.Now().Unix()
    atomicWrite(c.receiptFile, receipts)
}
```

**Acknowledgment Tracking:**
```go
// agent/internal/orchestrator/command_handler.go
func (c *CommandHandler) recordAcknowledgment(commandID string) {
    acks := c.loadAcks()
    acks[commandID] = true
    atomicWrite(c.ackFile, acks)
}
```

**Timeout Handling:**
```go
// server/internal/services/timeout.go
func (s *TimeoutService) checkForReceivedTimeouts() {
    for _, cmd := range s.pendingCommands {
        if time.Since(cmd.ReceivedAt) > s.timeoutConfig.ReceivedTimeout {
            if !s.acknowledged[cmd.ID] {
                // Re-emit command
                s.ReemitCommand(cmd)
            }
        }
    }
}
```

---

## Circuit Breaker Integration

**File:** `agent/internal/circuitbreaker/circuitbreaker.go`

```go
func (cb *CircuitBreaker) TryExecute(fn func() error) error {
    if cb.State == "open" {
        // Check if it's time to attempt recovery
        if time.Since(cb.LastFailureTime) > cb.OpenDuration {
            cb.State = "half-open"
            cb.FailureCount = 0
        } else {
            return errors.New("circuit breaker open")
        }
    }

    err := fn()
    if err != nil {
        cb.FailureCount++
        cb.LastFailureTime = time.Now()

        if cb.FailureCount >= cb.FailureThreshold {
            cb.State = "open"
            logSecurityEvent("[reliability] [agent] [circuit-breaker] Opened for", cb.Name)
        }

        return err
    }

    // Success in half-open state
    if cb.State == "half-open" {
        cb.State = "closed"
        logSecurityEvent("[reliability] [agent] [circuit-breaker] Closed for", cb.Name)
    }

    return nil
}
```

---

## Footer: Assumptions & Connections

**Assumption:** Polling is periodic and idempotent — same command may be received multiple times.

**Connection:** Deduplication (`verification/04-replay-protection.md`) prevents duplicate execution.

**Connection:** Nonce validation (`verification/04-replay-protection.md`) binds update commands to time window.

**Connection:** Circuit breaker (`verification/04-replay-protection.md`) prevents scanner failures from blocking other operations.

**Connection:** System events (`flows/04-heartbeat.md`) published to history table for audit trail.

**Connection:** Executed commands (`verification/04-replay-protection.md`) persisted to disk with 4-hour TTL.

---

*Last reviewed: 2026-05-26*
