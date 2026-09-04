# Update Lifecycle Flow

**Package state machine, two execution paths, and the orchestrator that drives them.**

---

## Overview

Packages move through a server-owned state machine from scan discovery to terminal resolution. The agent is a stateless executor — it receives commands, executes them, and reports results. The server owns every state transition.

The flow has two execution paths that diverge at install time:
- **Capability gate** (dnf, apt): server mints an Ed25519-signed token → agent's Rust helper verifies + executes → agent reports receipt
- **Legacy command** (docker, winget, windows_update): server creates a `confirm_dependencies` command → agent executes → agent reports via `ReportLog`

**Implementation status:** State machine enforced with typed `PackageStatus` and `ValidateTransition` guards (LIFECYCLE-001, v0.2.1.3). Lifecycle orchestrator running with stuck-state recovery and auto-advance (LIFECYCLE-003, v0.2.2.0). Approval-time supply chain enforcement: a vulnerability in any reported entry checked is a full stop with an audited override path (v0.2.3.1).

**Cross-references:**
- `flows/02-command-execution.md` — agent polling, command dispatch, at-least-once delivery
- `flows/04-heartbeat.md` — heartbeat lifecycle, system-vs-manual source
- `security/05-supply-chain-gate.md` — capability token design, helper execution
- `core/01-ethos.md` — idempotency (§4), assume failure (§3)
- `reference/projects/redflag-framework.md` §11.10 — State Machine Exhaustiveness
- `docs/tasks/GATE-000-supply-chain-gate-plan.md` — gate build status (steps 1-5 done, 6-7 remain)

---

## State Machine

Eight states. Three terminal (`installed`, `failed`, `ignored`), three active (`checking_dependencies`, `pending_dependencies`, `installing`), two waiting (`pending`, `approved`).

```
pending ──────► approved ──────► checking_deps ─┬──► installing ─┬──► installed
  │                │                 │            │                │
  │                │                 ├──► pending_deps            ├──► failed
  │                │                 │            │                │
  └──► ignored     └──► ignored      ├──► installed               └──► pending_deps
                                     └──► failed                  (new deps surfaced)

                               pending_deps ──► installing ───────┤
                                 │                                 │
                                 └──► failed                       │
                                                                   │
                               installing ───► pending_deps ───────┘
```

| State | Type | Entered by |
|-------|------|------------|
| `pending` | waiting | `UpdateCurrentStateInTx` — scan discovery |
| `approved` | waiting | `ApproveUpdate` — operator or auto-approve policy |
| `checking_dependencies` | active | `SetCheckingDependencies` — dry-run command queued |
| `pending_dependencies` | active | `SetPendingDependencies` — agent reported deps, operator must review |
| `installing` | active | `InstallUpdate` / `SetInstallingWithNoDependencies` — agent executing |
| `installed` | terminal | `UpdatePackageStatus` — install succeeded |
| `failed` | terminal | `UpdatePackageStatus` — install failed, timeout, or token mint failed |
| `ignored` | terminal | `RejectUpdate` — operator rejected |

**Re-scan behavior:** `UpdateCurrentStateInTx` (queries/updates.go:595) preserves terminal states on re-scan. Currently preserves `updated` and `ignored`; `failed` is NOT preserved (bug — fixed in LIFECYCLE-001). All other states reset to `pending` when a new version is discovered.

**Implementation:**
- Transition functions: `server/internal/database/queries/updates.go`
- Handler orchestration: `server/internal/api/handlers/updates.go`
- DB constraint: `current_package_state.status CHECK (...)` — migrations 003, 005, 007

---

## Capability Gate Path (Linux: dnf, apt)

```
checking_dependencies
  │
  │  Agent polls, receives dry_run_update command
  │  Agent: DiscoveryRunner.DryRun(pkg, version)
  │  Agent reports: POST /api/v1/updates/report-dependencies
  │
  ▼
ReportDependencies handler  (updates.go:1232)
  │  pinReportedClosure — stores artifact hashes from signed repo metadata
  │
  ├─ 0 deps ──► mintResolvedClosure → capability token (status: pending)
  │             SetInstallingWithNoDependencies → installing
  │
  └─ deps ──► SetPendingDependencies → pending_dependencies
               [operator clicks Confirm]
               ConfirmDependencies handler (updates.go:1470)
               mintResolvedClosure → capability token (status: pending)
               InstallUpdate → installing
  │
  ▼
installing
  │
  │  Agent polls: GET /api/v1/capability-tokens/pending/:agent_id
  │  Agent: consumer.ProcessToken → systemd-run --wait with token/result files
  │  Helper: verify token authority/replay → rehash local paths → fixed dnf/apt argv
  │  Agent reports: POST /api/v1/capability-tokens/:token_id/result
  │
  ▼
ReportCapabilityResult handler  (updates.go:1936)
  │  MarkConsumed(token_id)
  │  UpdatePackageStatus → installed | failed
```

The agent never receives an install command on this path. The capability token is the install authorization and fixes the package entries passed to APT/DNF. The top-level hash is mandatory; unresolved dependency hashes can be omitted from the reported set, and normal registry artifacts without local paths are not rehashed helper-side. APT/DNF may use the network and their signed repository metadata during execution.

**Implementation:**
- Token mint: `server/internal/services/capability_minter.go`
- Token polling: `agent/internal/agent/loop.go:processCapabilityTokens`
- Token consumption: `agent/internal/capability/consumer.go`
- Helper: `helper/src/main.rs`
- Discovery: `agent/internal/installer/dnf.go`, `agent/internal/installer/apt.go`

---

## Legacy Command Path (Docker, Winget, Windows)

```
checking_dependencies
  │  (same dry-run flow as capability path)
  ▼
pending_dependencies
  │  [operator clicks Confirm]
  │  ConfirmDependencies handler (updates.go:1470)
  │  Creates confirm_dependencies command (signed Ed25519)
  │  InstallUpdate → installing
  ▼
installing
  │
  │  Agent polls, receives confirm_dependencies command
  │  Agent: type-asserts installer to access mutation methods
  │  Agent executes install directly (no helper)
  │  Agent reports: POST /api/v1/updates/report-log
  │
  ▼
ReportLog handler  (updates.go:645)
  │  Idempotency check on command_id + terminal command status
  │  MarkCommandCompleted | MarkCommandFailed
  │  If command_type == confirm_dependencies:
  │    UpdatePackageStatus → installed | failed
```

Docker, Winget, and Windows Update are not behind the capability gate. They use direct mutation via type assertion on the installer interface. This is a known gap — the gate design covers them but implementation is deferred.

**Implementation:**
- Docker install: `agent/internal/handlers/docker.go`
- Winget install: `agent/internal/handlers/winget.go`
- Windows install: `agent/internal/handlers/windows_update.go`
- Command dispatch: `agent/internal/orchestrator/system_scanner.go:ExecuteCommand`

---

## Agent Handoff Points

The agent has no lifecycle state awareness. It is a stateless executor — it receives commands, executes them, and reports results.

| Phase | Agent trigger | Agent action | Server endpoint | Server state change |
|-------|--------------|--------------|-----------------|---------------------|
| Scan | Scanner schedule (poll-driven) | Run package manager scan | `POST /api/v1/updates/report-log` | `UpdateCurrentStateInTx` → `pending` |
| Dry-run | Receives `dry_run_update` command | DiscoveryRunner.DryRun | `POST /api/v1/updates/report-dependencies` | → `installing` or `pending_dependencies` |
| Token install | Polls `GET /api/v1/capability-tokens/pending/:agent_id` | consumer.ProcessToken → helper | `POST /api/v1/capability-tokens/:id/result` | → `installed` or `failed` |
| Command install | Receives `confirm_dependencies` command | Direct installer mutation | `POST /api/v1/updates/report-log` | → `installed` or `failed` |

**Heartbeat coordination:** Before creating a dry-run or install command, `InstallUpdate` and `ConfirmDependencies` queue a 10-minute `enable_heartbeat` command if one is not already active. This collapses the agent's poll interval during active lifecycle phases. Heartbeat creation failure is logged but does not block the lifecycle transition.

---

## Architectural Status

### Enforced (v0.2.2.0+)

- **Typed state machine.** `PackageStatus` is a typed enum in Go. `ValidateTransition`
  guards enforce the allowed graph. `TransitionPackageStatus` uses `WHERE status = $current`
  — a concurrent race lands on the constraint, not a silent overwrite. Migration 047 aligned
  all existing rows. (LIFECYCLE-001, v0.2.1.3)

- **Lifecycle orchestrator.** Timer-driven auto-advance and stuck-state recovery for
  `checking_dependencies` and `installing`. Auto-approval policy support. Packages stuck
  in active states no longer require manual operator intervention. (LIFECYCLE-003, v0.2.2.0)

- **Supply chain enforcement at approval.** `ApproveUpdate` checks the reported resolved
  entries against OSV. A vuln anywhere in that checked set returns 409 and mints nothing.
  Override requires an operator reason and is journaled. `ClosureCleared` is shared with
  auto-confirm. Unresolved dependency hashes can currently be omitted before this check.
  (v0.2.3.1)

### Remaining Visibility Gaps

- **Stepper collapses active states.** `checking_dependencies` and `pending_dependencies`
  both render as step 2 ("Approved") in the 4-step lifecycle stepper. The operator cannot
  distinguish "waiting for dry-run" from "dependencies need your review" without reading
  the status badge text.

- **Capability token path is invisible.** Token mint, consumption, and execution are
  tracked only in server logs. There is no UI endpoint for token status, and the operator
  cannot tell whether a package is installing via token or legacy command.

- **Staging area incomplete.** The Staging page exists (v0.2.1.1) but the full vision —
  assembling, staged, installing, completed as a single operator view — is not built.

---

## Target Model: Scan-Set Reconciliation + Maintenance-Window Campaign

**Decided 2026-06-06 (Casey + Opus). Supersedes the additive-scan assumption in lines 203–205.**

### The defect in the current model

A scan today is treated as an **additive discovery stream**, not a **set snapshot**.
`ReportUpdates` (`handlers/updates.go:191`) turns each reported update into a `discovered`
event → per-row UPSERT (`UpdateCurrentStateInTx`). `ReconcileFromScan` (`models/update_state.go`)
only reconciles packages **present** in the scan (resting states preserved, else → `pending`).
The two automatic paths to `installed` are receipt-driven (RedFlag drove it) and operator-manual
("resolved out of band").

**There is no closure-by-absence.** When a package drops out of a scan — patched by
`dnf-automatic`, a sysadmin, or anything outside RedFlag — its row stays `pending` forever.
The system adds and re-discovers but never subtracts. This is a §11.8 violation (render the
divergence, not the union) on a §11.1 lifecycle-boundary gap. It also contradicts the premise:
RedFlag should report what is outstanding **within the window the operator allocates**, not a
live snapshot that silently rots.

### Phase 1 — Scan-set reconciler (foundation)

Treat each ecosystem scan as the authoritative full set for that agent+ecosystem. On report,
diff the reported set against tracked non-resting rows (the DefectDojo reimport pattern —
`to_mitigate = set(tracked) − set(reported)`):

| Scan vs tracked | Transition |
|-----------------|------------|
| reported, not tracked | create `pending` (discovered) |
| reported, tracked | keep / version-bump (`ReconcileFromScan`) |
| **waiting (`pending`/`approved`), absent from scan** | → `installed`, provenance `out_of_band` (resolution is external by construction) |
| in-flight (`checking_dependencies`/`pending_dependencies`/`installing`), absent | **not closed by the reconciler** — owned by orchestrator + receipt path; `installing → installed` carries `redflag_receipt` |
| previously resolved, reappears | reopen → `pending` (`installed → pending` edge; SQL CASE + `ReconcileFromScan` in lockstep) |

**Closure scope is the waiting states only.** Closing in-flight rows would race a RedFlag-driven
install and mislabel its provenance, and split ownership of `installing` between the reconciler and
the orchestrator (§11.7). The `pending/approved → installed` edge was added to the state machine for
this path. Closure routes through `transitionStatus` (not a raw UPSERT) so it stays inside the state
machine and is idempotent (ETHOS §4). Absence must be confirmed by a **successful** scan of that
ecosystem (exit 0) — a failed/empty-due-to-error scan must never close rows (ETHOS §3, assume failure).

### Phase 2 — Maintenance-window campaign

The operator allocates a window. At **window-open** the in-scope set is frozen (the campaign
scope). Through the window, packages are driven to terminal. At **window-close** a closing scan
reconciles (Phase 1) and the campaign reports: applied / failed / deferred / resolved-out-of-band.
Dry-run (`checking_dependencies`) may run anytime; `installing` respects the window (already
asserted in the footer below). Model precedent: TacticalRMM `WinUpdatePolicy`
(`run_time_hour`/`run_time_days`/`run_time_frequency`) + `WinUpdate.date_installed`.

**Work items:** `docs/tasks/RECONCILE-001-scan-set-closure.md` (Phase 1),
`docs/tasks/WINDOW-001-maintenance-window-campaign.md` (Phase 2, depends on RECONCILE-001).

---

## Footer: Assumptions & Connections

**Assumption:** The agent is a stateless executor. It does not track lifecycle states and should not need to. The server owns the state machine.

**Assumption:** ~~Re-scan reconciliation is not a state machine transition — it is periodic scan-driven reset governed by `UpdateCurrentStateInTx`, not `ValidateTransition`.~~ **Superseded 2026-06-06** (see "Target Model" above): re-scan becomes set reconciliation, and closure-by-absence routes through `transitionStatus` inside the state machine.

**Assumption:** Dry-run is read-only and safe to run outside maintenance windows. The `checking_dependencies` phase can proceed anytime. The install phase (`installing`) must respect the maintenance window.

**Connection:** Update lifecycle implements ETHOS §4 (idempotency) — every transition must be run-3x-safe. The guarded UPDATE pattern in `TransitionPackageStatus` (LIFECYCLE-001) enforces this at the DB layer.

**Connection:** Update lifecycle implements ETHOS §3 (assume failure) — every active state must have a timeout path to `failed`. The orchestrator's stuck-state recovery (LIFECYCLE-003) closes the current gap where `checking_dependencies` has no timeout.

**Connection:** Capability gate path (`security/05-supply-chain-gate.md`) is the security-critical execution path. Token visibility (LIFECYCLE-005) makes this path auditable — currently the operator is blind to token lifecycle.

**Connection:** Heartbeat coordination (`flows/04-heartbeat.md`) ensures the agent polls faster during active lifecycle phases. The heartbeat is a side effect of command creation — it does not block the lifecycle transition.

**Connection:** Command execution (`flows/02-command-execution.md`) provides the at-least-once delivery and deduplication that the lifecycle depends on for agent handoff. The lifecycle layer sits above command execution — it creates commands and processes their results.

---

*Last reviewed: 2026-08-25 — implementation boundary reconciled for helper execution and closure coverage*
