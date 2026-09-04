# Start Here: RedFlag Architecture Overview

**Version:** v0.2.9.3 (July 2026)

This is the entry point into the RedFlag Architecture Framework. Read this first to
understand the shape of the system, then follow the links into the detailed docs.

---

## What RedFlag Is

A machine knowledge and operations system with a self-hosted fleet surface. Agents
report operating-system, process, socket, service, container, software, update, and
security state. RedFlag connects those facts to human approval, signed authority,
privileged execution, and durable lifecycle history instead of treating inspection and
mutation as separate products.

---

## Operating Surfaces and Authority Tiers

### Tier 1: Machine Observation and Fleet Lifecycle

Agents register with a one-time token and a hardware fingerprint (TOFU). The server
issues Ed25519-signed commands; agents verify signatures, check nonces, reject replays.
Pull-based polling (5 min default, rapid mode available). Subsystem scanning across
apt, dnf, pacman, winget, WUA, and Docker sits beside on-demand process/socket
inspection and local machine telemetry.

Packages move through a server-owned state machine (`pending` through `installed` or
`failed`) with typed transitions and guarded UPDATEs — no free-form string jumps. A
lifecycle orchestrator drives auto-advance and recovers stuck states.

**Architecture docs:**
- [core/01-ethos](core/01-ethos.md) — the five principles
- [core/02-architecture-decisions](core/02-architecture-decisions.md) — the twelve foundational choices
- [security/02-authentication-stack](security/02-authentication-stack.md) — four-layer auth (reg tokens, JWT, refresh, machine binding)
- [security/01-trust-boundaries](security/01-trust-boundaries.md) — endpoint classification and middleware matrix
- [flows/06-update-lifecycle](flows/06-update-lifecycle.md) — state machine, two execution paths, orchestrator

### Tier 2: Supply Chain Gate

The differentiator. The server is the signing authority — it evaluates policy (OSV
vulnerability checks, package age, human approval) and mints an Ed25519-signed
capability token describing exactly one operation over the resolved artifact set the
agent reported. On dnf/apt, the top-level hash is mandatory; dependency hashes that
resolve are included, while unresolved dependencies can currently be omitted. A
privileged, short-lived Rust executor (`helper/`) validates the token version and time,
host binding, signature, and replay state before running one fixed argv plan with no
shell and a cleared environment.

The helper rehashes a closure entry when it points to a readable local file and refuses a
missing mirror artifact. Normal registry entries without local paths are not rehashed
helper-side, and the current transient unit retains host network access. Full closure
pinning, complete local byte custody, and network isolation remain the target boundary.

The approval gate is fail-closed over the set it checked: a known vulnerability in a
reported resolved entry — top-level or transitive — blocks the token from being minted.
The operator must override with a documented reason. The override waives the vulnerability
judgment only; it does not waive capability validation or local artifact verification.

Auto-confirm shares the same `ClosureCleared` predicate as manual approval — the two
paths cannot drift on what counts as a clean closure.

**Architecture docs:**
- [security/05-supply-chain-gate](security/05-supply-chain-gate.md) — the design of record: capability model, wire contract,
  load-bearing constraints, enforcement layers, trust chain, hash registry
- `docs/tasks/GATE-000-supply-chain-gate-plan.md` — build status & implementation tracking (not design)

### Tier 3: Break-Glass Sessions

When Tiers 1–3 (read, catalog actions, signed runbooks) can't cover the case — live
shell, desktop control, or an urgent pre-signed runbook triggered by detection — the
session broker provides a break-glass path. It is a **separate privileged Rust binary**
(`redflag-broker`), spawned on demand via the same `sudo systemd-run` pattern as the
helper, disposable, time-boxed, and audit-logged.

The broker cannot start without a minted, Ed25519-signed session grant specifying
exactly what it may do. The agent verifies the grant and spawns the broker; after that
the agent is out of the loop. The broker opens its own connection to the server, streams
live I/O, hash-chains every command in a tamper-evident audit log, and exits when the
grant expires.

**Prerequisite:** Tier 4 requires RBAC (operator-level role gating for grant minting).
The design is complete but gated behind the RBAC substrate — a break-glass path without
role-gated minting is just "anyone can get a root shell."

**Architecture docs:**
- [components/06-session-broker](components/06-session-broker.md) — design of record: grant format,
  trust chain, audit trail, sequence diagram, scope variants

### RedFlag Desktop

The native Qt/QML Desktop is the local-machine surface. It holds no credential or
package-manager authority and speaks only to the Agent's local socket. It presents live
resources, processes, connections, storage, services, containers, installed software,
updates, security posture, and history, and may ask a standalone Agent to run the same
gate-and-helper authority chain.

Fleet-enrolled Agents refuse local minting. Standalone mode owns a stable local identity,
local scans, APT/DNF capability approval, and pacman `MutationEnvelope` approval. It does
not recreate an off-host authority boundary; its exact limits are published in
[security/06-standalone-authority](security/06-standalone-authority.md).

**Architecture docs:**
- [components/05-desktop](components/05-desktop.md) — native structure, Agent IPC, and local approval
- [security/06-standalone-authority](security/06-standalone-authority.md) — same-host trust boundary

### Process Explorer and Software Ownership

On-demand `/proc` filesystem scanning for process inventory and drill-down detail.
Triggered when a user opens the Processes tab — no background broadcasting.
25+ fields per process plus open files, sockets, pipes, environment keys, memory maps,
namespaces, and listening ports. Linux drill-down also reads effective capabilities and
cgroup ownership, attributes a process to a systemd unit or container where the kernel
provides it, and joins the executable back to its installed package when a supported
package manager can prove ownership.

Data collection caps are server-controlled via `ProcessExplorerConfig` (Settings →
Process Explorer) and delivered to agents on check-in. Listening ports use socket
inode correlation against `/proc/net/tcp` — not system-wide assignment.

**Architecture docs:**
- [scanners/05-process-scanner](scanners/05-process-scanner.md) — data model, collection, caps
- [flows/07-process-scan](flows/07-process-scan.md) — command-dispatch flow, API endpoints, schema

---

## Architectural Boundaries

### Fleet Lifecycle Is Server-Owned

In fleet mode the Agent receives commands, observes the host, executes authorized work,
and reports results. It does not own update lifecycle states; the Server owns every
transition. Signature, nonce, replay, target, and helper verdicts are local enforcement
decisions. Standalone mode is an explicit exception to server dependence, not to those
checks: it keeps local observations and runs a bounded local scan/approval loop without
inventing a fleet lifecycle.

### Gated Mutation Only Through the Helper

On capability-gated ecosystems, the Agent cannot run install commands directly. APT and
DNF flow through `consumer.go` and a capability token. Standalone pacman flows through a
signed `MutationEnvelope` whose exact archives and detached signatures are verified
before mint and again before execution. Both end at a fixed `redflag-helper` invocation;
the Agent holds no package-manager sudo. Discovery and resolution stay unprivileged.

### Current Execution Paths

- **Capability gate** (dnf, apt): token minted at approval → agent polls for tokens →
  helper verifies + executes → agent reports receipt. No install command issued.
- **Standalone pacman envelope**: Agent resolves and hashes the official-repository
  transaction → local root helper validates custody and signs → execute mode revalidates
  and runs one fixed pacman plan → joined receipt returns locally.
- **Legacy command** (docker, winget, windows_update): signed command → agent executes
  directly via type-asserted installer methods → reports via ReportLog.

Fleet pacman envelope delivery and the remaining legacy ecosystems are known gaps.

### Six Load-Bearing Constraints

From `security/05-supply-chain-gate.md` — do not regress these:

1. Sign the resolved closure, not the top-level package
2. The signer lives off the web process (seam documented, not yet isolated)
3. Verified-cache fallback, fail-closed only on change
4. Verify keys, not servers
5. Kernel stops are defense-in-depth, not a prerequisite
6. No doctrinal knobs — signing required and forward-only are not configurable

---

## Navigation

| Section | What It Describes |
|---------|-------------------|
| [core](core/) | ETHOS principles, architectural decisions |
| [components](components/) | Server, agent, web, helper, session broker — package structure and responsibilities |
| [security](security/) | Trust boundaries, auth stack, machine binding, supply chain gate |
| [verification](verification/) | Ed25519 signing pipeline, agent verification, key rotation, replay protection |
| [scanners](scanners/) | Per-ecosystem scanner behavior and integration points (incl. process scanner) |
| [flows](flows/) | Data flows — registration, command execution, upgrade, heartbeat, capability advertisement, update lifecycle |
| [reference](reference/) | File mappings, glossary |

---

## Honest Gaps

- **Gate policy visibility**: the soak and age gates are live policies as of v0.2.6.2 (`supply_chain.*` settings — see [security/05-supply-chain-gate](security/05-supply-chain-gate.md) §4), but the dashboard doesn't yet surface their configuration; operators tune them blind. The live install-through-helper path completed e2e on 2026-06-05
- **Closure completeness**: dnf/apt require the top-level hash, but dependency hashes are best-effort and unresolved entries can be omitted; npm/pypi registry pinning remains single-entry
- **Registry artifact verification**: the helper rehashes local paths, but normal registry artifacts without a local path are not rehashed helper-side
- **Helper network isolation**: the current `systemd-run` unit retains host network access; complete local artifact custody and a private network boundary are not built
- **Signer in-process**: key encapsulated in SigningService, minter is the only caller — but true process isolation not built
- **Legacy ecosystems ungated**: docker, winget, windows_update still direct-mutation
- **Fleet pacman seam**: standalone pacman envelopes execute; the Server does not yet mint and deliver the same envelope to fleet Agents
- **Standalone transition**: local startup and approval work, but standalone-to-fleet join is deliberately refused until local-key retirement and trust replacement are one tested transaction
- **Local attribution**: Desktop's operator label is asserted from the session environment; socket peer identity and fresh step-up are unfinished
- **Local journal surface**: helper decisions are journaled, but Desktop cannot yet browse the root journal and no one-time fleet import exists
- **Desktop platforms**: Linux amd64 is the only native Qt release artifact; Windows transport exists in source without a proved Windows Desktop build
- **Kernel enforcement inert**: eBPF scaffold exists, not wired to the capability model
- **Service posture unrepresented**: RedFlag observes what a host *has* and has no notion of what it is *supposed* to have. Comparing declared fleet service expectations against signed host observations and independent network/container health would let it distinguish missing, unhealthy, unknown, undeclared, and intentionally absent services — the observed-versus-intended split the update path already makes, applied to services. Candidate, not designed. The distinction that earns it is **missing** versus **unknown**, which generic health monitoring blurs

These are architectural gaps, not bugs. They define where the system's protection
boundary currently ends. Task tracking for closing them lives in `docs/tasks/`,
which is internal and deliberately not part of the public projection.

---

*Last reviewed: 2026-09-04*
