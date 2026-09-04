# Architecture Decisions

**Key architectural choices that shaped RedFlag.**

---

## Decision 1: Pull-Based Agent Polling

**Choice:** Agents poll server every 5 minutes (configurable) rather than server pushing commands.

**Rationale:**
- Simpler failure mode — if server is down, agents simply stop checking. No need to manage push infrastructure, retry queues, or webhook delivery.
- Easier to reason about — no race conditions where command is sent but never received.
- Cost-effective for homelabs — one HTTP connection handles both commands and heartbeats.

**Trade-offs:**
- Higher latency for commands (max 5 minutes between dispatch and execution)
- More frequent server checks (agents still ping every 5 min for commands)
- Requires exponential backoff for server unavailability

**Implementation:**
- Polling loop: `agent/internal/agent/loop.go`
- Backoff logic: `agent/internal/retry/retry.go:calculateDelay()`
- Heartbeat: `agent/internal/orchestrator/system_scanner.go`

**Cross-references:**
- `flows/01-registration.md` (TOFU key caching)
- `flows/02-command-execution.md` (polling loop implementation)
- `verification/02-agent-verification.md` (nonce-based replay protection)

---

## Decision 2: Hardware-Bound Machine IDs

**Choice:** SHA-256 hash of the `machineid` library identifier (cross-platform), with OS-specific fallbacks — `/etc/machine-id`, dbus machine-id, DMI product UUID, and hostname only as a last resort on generic platforms.

**Rationale:**
- Prevents config file copying between machines — a stolen agent config cannot be used on a different machine.
- Detects hardware changes (SSD replacement, motherboard swap) and triggers security event.
- Simple to compute on agent, compare on server.

**Trade-offs:**
- Machine ID changes on major hardware changes (requires rebind endpoint)
- Linux-only custom implementation (Windows/macOS rely on OS identifiers)

**Implementation:**
```go
// agent/internal/system/machine_id.go
func GetMachineID() (string, error) {
    id, err := machineid.ID()          // cross-platform; Linux reads /etc/machine-id
    if err == nil && id != "" {
        return hashMachineID(id), nil  // SHA-256, hex-encoded — no hostname
    }
    // OS-specific fallbacks: /etc/machine-id, dbus machine-id, DMI product UUID;
    // hostname only as a last resort on generic/unknown platforms
    return osSpecificFallback()
}
```

**Cross-references:**
- `security/01-trust-boundaries.md` (machine binding middleware)
- `flows/01-registration.md` (TOFU machine ID validation)

---

## Decision 3: Ed25519 Command Signing

**Choice:** All commands signed with Ed25519 private key on server, verified on agent.

**Rationale:**
- Ed25519 provides strong cryptographic guarantees with small key sizes (32 bytes).
- Key rotation support — can rotate signing keys without downtime.
- Agent-side verification is fast and side-channel resistant.

**Trade-offs:**
- Server must keep signing key secure (environment variable or Docker secret)
- Agent must cache server public key (TOFU model)
- Signature adds ~72 bytes per command

**Implementation:**
v3 format: `"{agent_id}:{id}:{command_type}:{sha256(params)}:{unix_timestamp}"`

**Cross-references:**
- `verification/01-signing-pipeline.md` (server-side signing)
- `verification/02-agent-verification.md` (agent-side verification)
- `verification/03-key-rotation.md` (key rotation support)

---

## Decision 4: Multi-Layer Authentication

**Choice:** Four-layer auth stack — registration tokens → JWT → refresh tokens → machine binding.

**Rationale:**
- Registration tokens provide one-time enrollment without storing secrets.
- JWT provides short-lived access tokens (24h) for API calls.
- Refresh tokens provide long-lived authentication (90d sliding window) for polling.
- Machine binding ties JWT to specific hardware.

**Trade-offs:**
- More complex than single-layer auth
- Refresh token expiration requires careful management
- JWT expiry requires renewal logic (currently TODO)

**Cross-references:**
- `security/02-authentication-stack.md` (full stack details)
- `security/03-refresh-tokens.md` (token lifecycle)

---

## Decision 5: Circuit Breakers Per Subsystem

**Choice:** Individual circuit breakers for each scanner (APT, DNF, Winget, WUA, Docker).

**Rationale:**
- APT failure doesn't affect Docker scanning.
- Circuit breaker auto-heals — if WUA comes back online, it resumes without manual intervention.
- Prevents cascading failures (one scanner timeout doesn't slow down the entire agent).

**Trade-offs:**
- More state management (per-subsystem circuit breaker state)
- Configuration required (failure threshold, failure window, open duration)

**Implementation:**
```go
// agent/internal/circuitbreaker/circuitbreaker.go
type CircuitBreaker struct {
    failureThreshold int
    failureWindow    time.Duration
    openDuration     time.Duration
    halfOpenAttempts int
}
```

**Cross-references:**
- `flows/02-command-execution.md` (circuit breaker integration in polling loop)
- `testing/01-test-pyramid.md` (circuit breaker unit tests)

---

## Decision 6: Idempotent Scanner Synchronization

**Choice:** Poll-based scanner sync (`syncAvailableScanners`) that re-runs every check-in.

**Rationale:**
- Scanners can be installed after registration (e.g., Docker installed on agent post-registration).
- Re-running sync every poll is safe — it's a diff operation (INSERT new, don't DELETE missing).
- Handles transient failures gracefully — if scanner is temporarily unavailable, it's not torn down.

**Anti-pattern (pre-ARC-001):**
```go
// BAD — not idempotent
func syncScanners(scannerList []string) {
    for _, scanner := range scannerList {
        db.Exec("INSERT INTO agent_subsystems ...")
    }
    // Running twice would create duplicate rows
}
```

**Correct pattern:**
```go
// GOOD — idempotent
func syncAvailableScanners(agentID string, scanners []string) {
    for _, scanner := range scanners {
        db.Exec("INSERT INTO agent_subsystems (agent_id, name, enabled)
                 VALUES ($1, $2, true)
                 ON CONFLICT (agent_id, name) DO NOTHING", agentID, scanner)
    }
}
```

**Implementation:**
- `server/internal/database/queries/subsystems.go`
- `server/internal/api/handlers/agents.go:syncAvailableScanners()`

**Cross-references:**
- `flows/05-capability-advertisement.md` (capability advertisement)
- `core/01-ethos.md` (principle #4: Idempotency is a Requirement)

---

## Decision 7: Command Deduplication

**Choice:** Persist executed command IDs to disk (`executed_commands.json`) with 4-hour max age.

**Rationale:**
- Prevents duplicate execution after agent restart.
- Survives service restarts — if agent crashes between poll and execution, restart can't replay command.
- 4-hour window aligns with command max age (replay protection).

**Trade-offs:**
- Requires disk I/O for every command executed
- Disk corruption could cause duplicates (mitigated by atomic writes)

**Implementation:**
```go
// agent/internal/orchestrator/command_handler.go
func (c *CommandHandler) handleCommand(cmd *Command) {
    executedIDs := loadExecutedCommands()
    if executedIDs.Contains(cmd.ID) {
        logSecurityEvent("[security] [agent] [command] Duplicate command rejected:", cmd.ID)
        return
    }
    executedIDs.Add(cmd.ID)
    saveExecutedCommands(executedIDs)
}
```

**Cross-references:**
- `verification/04-replay-protection.md` (timestamp + nonce replay protection)
- `flows/02-command-execution.md` (deduplication in polling loop)

---

## Decision 8: Agent Self-Upgrade with Rollback

**Choice:** 7-step agent self-upgrade with atomic binary swap and automatic rollback on failure.

**Rationale:**
- Agents can update without manual intervention.
- Rollback ensures zero-downtime if update fails.
- Backup (.bak) ensures previous version is always available if watchdog times out.

**Trade-offs:**
- Requires service manager (systemd on Linux, SCM on Windows)
- Container-only agents cannot self-update — must redeploy image instead
- Update failure requires manual rollback if watchdog times out

**7-step flow:**
1. Admin triggers update
2. Server creates signed `update_agent` command
3. Agent downloads new binary
4. Agent verifies checksum + Ed25519 signature
5. Agent creates backup (.bak)
6. Agent atomic replacement + service restart
7. Watchdog 15min timer → success or rollback

**Watchdog timeout:** 15-minute default (configurable via `security_settings.operational.agent_update_timeout_minutes`)

**Cross-references:**
- `flows/03-agent-upgrade.md` (full upgrade flow)
- `flows/02-command-execution.md` (deduplication for update commands)

---

## Decision 9: Software as a Service (SaaS) vs Self-Hosted

**Choice:** Purely self-hosted — no cloud dependencies, no SaaS features.

**Rationale:**
- Homelab-first design — operators who value control, privacy, and cost sanity.
- No vendor lock-in — all data stays on local infrastructure.
- No recurring costs — $0/agent/month vs ConnectWise's $50/agent/month.

**Trade-offs:**
- No built-in monitoring/alerting infrastructure (no retry queues, no delivery guarantees, no SMTP relay pool). RedFlag pushes events to external tools the operator owns — Wazuh, ntfy, SMTP — via one-shot emitters on the server. This is the precedent set by `flows/08-wazuh-event-emitter.md` and extended by `docs/tasks/NOTIFY-001-notification-system.md`. The dashboard remains the source of truth; external notifications are a courtesy tap on the shoulder.
- No multi-tenant support (single-tenant by design)
- Requires operational overhead (updates, backups, maintenance)

---

## Decision 10: Update Nonce for Replay Protection

**Choice:** Ed25519-signed nonce tied to check-in interval for update commands.

**Rationale:**
- Binds update commands to specific time window (2× check-in interval).
- Prevents replay attacks where captured commands are re-sent.
- Server validates nonce age before executing update_agent commands.

**Trade-offs:**
- Adds computational overhead for nonce generation/validation
- Requires server to track nonce expiry state

**Implementation:**
- `server/internal/services/update_nonce.go`
- `server/internal/middleware/machine_binding.go:validateNonce()`

**Cross-references:**
- `verification/04-replay-protection.md` (nonce validation)
- `security/02-authentication-stack.md` (machine binding integration)

---

## Decision 11: Security Settings Service

**Choice:** Centralized policy management via `SecuritySettingsService` with granular operational controls.

**Rationale:**
- Centralizes policy decisions (dry runs, nonce requirements, auto-heartbeat).
- Enables runtime configuration without restarts.
- Provides audit trail for policy changes.

**Trade-offs:**
- Adds database dependency for policy storage
- Requires careful default configuration

**Operational settings:**
- `policy.allow_dry_runs` (default true)
- `policy.require_nonce` (default true)
- `policy.auto_heartbeat_enabled` (default true)
- `operational.update_stuck_minutes` (default 5)
- `operational.agent_update_timeout_minutes` (default 15)

**Cross-references:**
- `flows/02-command-execution.md` (dry run gating)
- `verification/04-replay-protection.md` (nonce requirement)

---

## Decision 12: Scheduler Job Eviction on Disable

**Choice:** Disabling a subsystem removes its job from the in-memory scheduler priority queue immediately, rather than waiting for a scheduler reload or relying on the worker to check DB state.

**Rationale:**
- The scheduler loads subsystem state once at startup into an in-memory priority queue (`scheduler.LoadSubsystems`). It never re-reads `enabled` from the DB.
- `DisableSubsystem` previously only flipped the DB column — the scheduler kept creating commands, making disable non-functional until server restart.
- The PriorityQueue already had a `Remove(agentID, subsystem)` method. The fix was to surface it as `Scheduler.RemoveSubsystemJob` and call it from the disable handler.

**Anti-pattern (pre-ARC-012):**
```go
// BAD — flips DB column but scheduler never checks it again
func (h *SubsystemHandler) DisableSubsystem(...) {
    h.subsystemQueries.DisableSubsystem(agentID, subsystem)
    c.JSON(http.StatusOK, ...)
}
```

**Correct pattern:**
```go
// GOOD — evicts from in-memory scheduler immediately
func (h *SubsystemHandler) DisableSubsystem(...) {
    h.subsystemQueries.DisableSubsystem(agentID, subsystem)
    if h.scheduler != nil {
        h.scheduler.RemoveSubsystemJob(agentID, subsystem)
    }
    c.JSON(http.StatusOK, ...)
}
```

**Trade-offs:**
- The scheduler still has a small window between `processQueue` popping the job and `worker.run` checking — a disable during that window could still produce one last command. This is acceptable: the DB flip means the command will be a no-op on the agent, and the gap is at most ~1 second (one GOP waiting on the rate limiter).
- `RemoveSubsystemJob` is nil-safe (unset scheduler is a no-op), so handler construction order doesn't matter.

**Implementation:**
- `server/internal/scheduler/scheduler.go:RemoveSubsystemJob()`
- `server/internal/api/handlers/subsystems.go:DisableSubsystem()`
- `server/cmd/server/main.go` (SetScheduler injection)

**Cross-references:**
- `components/01-server.md` (scheduler component docs)
- `core/01-ethos.md` (principle #1: Errors are History — the one-last-command gap is logged)

## Decision 13: Development Trunk Distinct From Publication Output

**Choice:** `main` is the internal development authority. `public` is an output ref written only by the publication job, never by a person.

**Rationale:**
- Until 2026-09-04 RedFlag's internal repository had exactly one branch, `public`. Development trunk and publication output were the same ref, so every internal commit was already a publication decision whether or not anyone made one. There was no private side for work-in-progress to be safe in.
- The cost was not theoretical. `fd23d08` introduced a private forge address and was caught only at the publication gate, because there was no earlier place for it to be harmless.
- Separating them makes the publication boundary a real transaction with two sides: a source SHA that may contain ordinary development mess, and a candidate tree that is constructed, gated, built, and only then published.
- A public tree that is *constructed* can be sanitized without making the development repository useless. Sanitizing the development repository until it is publishable destroys the working record instead.

**Boundary:**
- `main` accepts normal work. Internal operational material is legal there.
- `public` is advanced by CI only, after the tree, content, metadata, and build gates in `.gitea/workflows/ci.yml` all pass.
- Publication is fast-forward only. `.publication/EPOCH` may authorize exactly one replacement, and only of the SHA it names, so the authorization is spent by its own use.
- `.publication/surface.json` is the authority for which paths may cross. Changing it is a public-surface decision, and it is reviewed as one.

**Consequences:**
- Public history begins at a deliberate projection epoch rather than accumulating whatever the development branch happened to record. Development history is preserved internally and is never rewritten.
- Downstream mirrors reproduce the verified public forge ref. They are never independent publication authorities, and a mirror failure cannot roll the public forge back.

**Trade-off accepted:** the public repository shows no history before the epoch. That is the true statement, because the history before it was not admissible.

---

## Architecture Cross-References

- **Pull-based polling** → `flows/02-command-execution.md`
- **Machine IDs** → `security/01-trust-boundaries.md`
- **Ed25519 signing** → `verification/01-signing-pipeline.md`
- **Multi-layer auth** → `security/02-authentication-stack.md`
- **Circuit breakers** → `flows/02-command-execution.md`
- **Idempotent sync** → `flows/05-capability-advertisement.md`
- **Command deduplication** → `verification/04-replay-protection.md`
- **Agent upgrade** → `flows/03-agent-upgrade.md`
- **Nonce validation** → `verification/04-replay-protection.md`
- **Security settings** → `security/02-authentication-stack.md`

---

*Last reviewed: 2026-09-04*

**Footer: Assumptions & Connections**

**Assumption:** Decision 10 (nonce) and Decision 11 (security settings) are orthogonal enhancements to the core auth stack — they complement rather than replace existing mechanisms.

**Connection:** Nonce validation (`verification/04-replay-protection.md`) reinforces Decision 4 (machine binding) by adding temporal binding to sensitive operations.

**Connection:** Security settings service (`security/02-authentication-stack.md`) provides the policy layer that gates decision execution paths.

**Connection:** 15-minute watchdog (Decision 8) aligns with `operational.agent_update_timeout_minutes` setting (Decision 11).
