# Agent Component

**Stateless Go executor that polls the server, verifies every command cryptographically, and reports everything back.**

---

## Doctrine

The agent does not track lifecycle states, make policy decisions, or hold install privileges on gated ecosystems. The server owns every state transition. The agent's only autonomous decisions are: verify this signature, check this nonce, reject this replay. See [core/02-architecture-decisions](../core/02-architecture-decisions.md).

---

## Package Structure

```
agent/
├── cmd/agent/                  # Entry point, flag parsing, service bootstrap
├── internal/
│   ├── agent/loop.go           # RunAgentLoop → RunPollingLoop — the heartbeat of the process
│   ├── handlers/               # Command handlers, routed by dispatch.go
│   │   ├── dispatch.go         # Command-type → handler routing
│   │   ├── scan.go             # Subsystem scan execution
│   │   ├── install.go          # Legacy-path installs (docker, winget, windows_update)
│   │   ├── dry_run.go          # Dependency resolution + hash discovery
│   │   ├── agent_update.go     # Self-update commands
│   │   ├── heartbeat.go        # Heartbeat + rapid polling
│   │   ├── processes.go        # On-demand process explorer scans
│   │   ├── local_approve.go    # Desktop-self local approval flow
│   │   └── reboot.go, screenshot.go, upgrade_attestation.go
│   ├── scanner/                # apt, dnf, pacman, winget, windows (WUA), detect
│   ├── installer/              # DiscoveryRunner + per-ecosystem installers
│   │   ├── discovery.go        # Single chokepoint for read-only package ops
│   │   ├── apt.go, dnf.go      # Gated: 4-method interface, no mutation
│   │   ├── docker.go, winget.go, windows.go  # Legacy: direct mutation via type assertion
│   │   └── artifact_hash.go    # SHA-256 resolution for closure pinning
│   ├── supplychain/            # consumer.go — capability-token → helper invocation
│   ├── crypto/                 # TOFU pubkey cache, signature/nonce/replay verification
│   ├── instancelock/           # flock (Unix) / named mutex (Windows) — one agent per config
│   ├── circuitbreaker/         # Per-scanner circuit breakers
│   ├── event/                  # TeeLogger (structured dual-output), buffered event reporting
│   ├── system/                 # machine_id, system info, /proc process explorer
│   ├── cache/                  # Hash cache, local state
│   ├── config/                 # config.json, subsystems, kernel enforcement flags
│   ├── localapi/, desktop/     # Local API + desktop tray session integration
│   ├── kernel/                 # eBPF scaffold (inert — not wired to capability model)
│   └── registration/, recovery/, retry/, receipt/, acknowledgment/
└── pkg/windowsupdate/          # WUA COM bindings (vendored fork, Apache 2.0)
```

---

## The Polling Loop

`RunAgentLoop` (`agent/internal/agent/loop.go`) initializes config, instance lock, crypto, and circuit breakers, then enters `RunPollingLoop`:

1. **Check in** — report metrics, buffered events, security events, circuit-breaker health
2. **Fetch commands** — verify signature, nonce, timestamp on each; reject replays
3. **`processCommands`** — route through `dispatch.go` to handlers
4. **`processCapabilityTokens`** — fetch minted tokens, hand to `supplychain/consumer.go`
5. **Sleep** — server-controlled interval (`applyServerPolling`), jittered

**Failure handling:** `classifyFailure` buckets errors into failure classes; `delayForFailure` applies a unified backoff policy per class (BUG-014). Typed sentinel errors (`ErrUnauthorized`, `ErrRefreshTokenInvalid`, `ErrMachineMismatch`) are terminal — not retried, logged as `[CRITICAL]`.

---

## Two Execution Paths (agent side)

| Path | Ecosystems | Mechanism |
|------|-----------|-----------|
| Capability gate | dnf, apt | Token fetched in loop → `consumer.ProcessToken` → `sudo systemd-run --wait` with token/result files → `redflag-helper` verifies + executes. Agent never runs the install command. |
| Legacy command | docker, winget, windows_update | Signed command → handler → installer mutation method (type-asserted). |

Discovery (scan, dry-run, hash-resolve) always runs unprivileged through `DiscoveryRunner`. Sudoers grants only discovery commands plus the single helper invocation line — zero sudo otherwise.

**Cross-references:**
- [security/05-supply-chain-gate](../security/05-supply-chain-gate.md) (token contract)
- [flows/02-command-execution](../flows/02-command-execution.md) (command path)
- [flows/07-process-scan](../flows/07-process-scan.md) (process explorer flow)

---

## Verification (every command, no exceptions)

- **TOFU pubkey cache** (`crypto/pubkey.go`) — keys cached by `key_id`; unknown signer triggers re-fetch, no restart needed
- **Signature** — Ed25519 over the v3 message format
- **Nonce + timestamp** — 10-minute validity window, executed-nonce tracking, replay rejection
- Signing-required is doctrine, not config. There is no skip path.

**Cross-references:**
- [verification/02-agent-verification](../verification/02-agent-verification.md) (full pipeline)
- [verification/04-replay-protection](../verification/04-replay-protection.md)

---

## Resilience Machinery

- **Instance lock** — `Global\RedFlagAgent_v1` mutex / flock; prevents two agents racing one `config.json` and burning refresh-token rotations ([security/03-refresh-tokens](../security/03-refresh-tokens.md))
- **Circuit breakers** — fragile scanners (notably WUA) trip open instead of hammering; health reported to server
- **TeeLogger** — every loop event goes to both structured local log and server-bound buffer; tracker save failures tee inward (ETHOS #1)
- **At-least-once acks** — `acknowledgment/tracker.go` persists until the server confirms result-recorded

---

*Last reviewed: 2026-08-25*
