# Supply Chain Gate

**Package-manager authorization with signed capabilities and mutation envelopes; pacman has full local artifact custody, while network isolation and kernel enforcement remain design work.**

This is the design of record for the gate — the capability model, the wire contract all
three components agree on, the load-bearing constraints, and component responsibilities.
Build status and per-step implementation tracking live in `docs/tasks/GATE-000-supply-chain-gate-plan.md`.

---

## Overview

RedFlag's supply chain gate inverts the traditional defense model: instead of **allow all installs and detect bad ones**, it **denies all state changes and requires explicit human authorization**.

**Current source scope:** APT and DNF mutation through RedFlag uses a signed capability
token. Standalone pacman approval uses a signed `MutationEnvelope`, exact closure archives,
detached package signatures, helper custody, and a joined receipt. Docker, Winget, and
Windows Update still use the signed-command path. Fleet pacman envelope delivery and kernel
enforcement against out-of-band root mutation are not wired.

---

## The decision: capability tokens, not a decision daemon

The gate authorizes APT/DNF installs with **signed capability tokens** and pacman with a
**signed mutation envelope**, not with a runtime decision daemon. In fleet mode the Server
is the authority: it evaluates policy and mints an Ed25519-signed capability over the
artifact entries the Agent resolved and reported. In standalone mode the short-lived root
helper mints locally after validating the bounded request; this preserves a privilege
boundary, not the fleet's off-host authority boundary.

APT/DNF still require the top-level hash while dependency hashes are best-effort: an
unresolved dependency is logged and omitted rather than making the report fail. Pacman is
stricter. The Agent resolves and downloads the transaction from signed repository metadata,
and the helper requires an exact archive and detached signature for every action before it
will sign or execute.

A small, privileged **executor** (`helper/`, Rust) validates token version and time, host
binding, signature, and replay state, then runs a fixed argv plan without a shell or inherited
environment. It rehashes local artifact files and requires a readable matching file for a
mirror entry. A normal registry entry without a local path is not rehashed helper-side, and
the current transient unit retains host network access.

Pacman now resolves and stages the transaction and rehashes every byte at the privileged
edge. APT/DNF full-closure custody and a network-isolated helper unit remain explicit design
work, not present guarantees.

This replaced the earlier `rs-helper` socket-decision daemon. `rs-helper`'s reusable parts
(the eBPF `InterceptEvent` struct, the package-manager allowlist, the hash cache) moved into
the unprivileged agent-side consumer. Its *role* as a runtime allow/deny RPC is retired.

### Why this model (two tests it has to pass)

**Cross-platform.** Linux eBPF and macOS ESF can pause an exec and ask a daemon "may this
proceed?" Windows WDAC cannot — it is signature-based, with no runtime callback. A
decision-daemon model therefore has no Windows mapping. A capability token maps onto all
three identically: in every case the enforcement layer only needs to answer "is this
execution authorized," and a verified token + a privileged executor that the OS trusts is
platform-agnostic. We build for the platform with the tightest constraint.

**Protects people.** Protection comes down to where the trust root lives. The signing key
lives at the server (the human-approval authority), off the host. An attacker who fully owns
the agent process — a prompt-injected coding agent, the literal threat — can *request* a
token but cannot forge the server's signature, so the executor never runs. The trust root is
outside the blast radius. A socket-RPC daemon degrades to "can the attacker reach the socket
or influence what's pinned," which a compromised agent tier often can.

The two tests converge on the same answer, which is the signal it's right.

---

## Architecture

### Enforcement Layers (Platform-Specific)

| Platform | Enforcement Mechanism | What It Blocks |
|----------|----------------------|----------------|
| **Linux** | eBPF (syscalls/sys_enter_execve) or AppArmor | apt, dnf, yum, pip, npm, bun, docker (CLI) |
| **Windows** | WDAC (Windows Defender Application Control) | winget, npm.cmd, pip.exe, choco, scoop |
| **macOS** | Endpoint Security Framework (ESF) | brew, pip, npm, bun, cargo |

**Target distinction:** kernel enforcement sits below userspace wrappers. The current eBPF
scaffold is not connected to the capability model, so RedFlag does not yet claim that an
out-of-band package-manager invocation is blocked.

### Trust Chain

```
┌─────────────────────────────────────────────────────────────┐
│  Human Operator                                             │
│  - Reviews UI prompt with OSV findings, age checks, etc.   │
│  - Clicks "Approve" or "Reject"                            │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│  RedFlag Server (authority, unprivileged)                   │
│  - Resolves the closure, records/fetches per-artifact hash   │
│  - Runs OSV.dev check + package-age gates                    │
│  - Mints an Ed25519-signed capability token (signer off the  │
│    request path)                                             │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│  RedFlag Agent (consumer, unprivileged)                     │
│  - Polls for the token, confirms agent_id is this host       │
│  - Holds no signing key; cannot run installs directly        │
│  - Hands the token to the executor                           │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│  Helper / Executor (privileged, short-lived, Rust)          │
│  - Invoked via `sudo systemd-run --wait` with token/result   │
│    files; escapes the agent's ProtectSystem sandbox          │
│  - Verifies version/time/host/signature/replay state         │
│  - Rehashes local files; registry entries may have no path   │
│  - Executes fixed argv: no shell, env stripped               │
│  - Current unit retains host network access                  │
└─────────────────────────────────────────────────────────────┘
```

---

## The token (the contract all three sides agree on)

This is the canonical wire contract. The Go (`server/`, `agent/internal/capability/token.go`)
and Rust (`helper/src/main.rs`) implementations reconstruct identical bytes from it.

```jsonc
{
  "version": 1,
  "token_id": "<uuid>",            // unique; replay guard + receipt/audit
  "agent_id": "<uuid>",            // bound to exactly one host
  "key_id": "<hex, 32 chars>",     // authority key fingerprint (rotation)
  "package_type": "apt|dnf|npm|bun|pip|docker|winget|agent-self",
  "operation": "install|upgrade",  // forward-only; no downgrade
  "closure": [                     // signed reported set; target is the full closure
    {
      "name": "<pkg>",
      "version": "<exact>",
      "sha256": "<hex>",           // expected artifact hash
      "source": "mirror|registry",
      "artifact_path": "<local path or url, optional>"
    }
  ],
  "issued_at":  <unix>,
  "not_before": <unix>,
  "expires_at": <unix>,            // short TTL
  "signature": "<hex ed25519>"     // over the canonical message below
}
```

**Canonical signed message** (deterministic, language-agnostic — mirrors the existing v3
command format so Go and Rust reconstruct identical bytes):

```
closure_hash  = hex(sha256( "\n".join( sorted("{name}@{version}#{sha256}") ) ))
signed_message = "{agent_id}:{token_id}:{operation}:{package_type}:{closure_hash}:{expires_at}"
signature      = ed25519_sign(authority_priv, signed_message)
```

The closure is sorted before hashing so ordering can't change the digest. Tampering with any
artifact, version, or hash changes `closure_hash` and breaks verification.

### Mutation manifest contract (live for standalone pacman)

The closure token remains live for APT and DNF. Pacman is the first runtime backend migrated
to `MutationEnvelope`, so new backends can converge without extending the closure metaphor
or preserving its unsigned `source`/`artifact_path` ambiguity.

Those two legacy unsigned fields steer capability-token *verification* today, not helper
execution:
`build_plan` reads only name, version, package type, and operation, and the self-update
branches take their source path from a helper constant. The server never populates
`artifact_path` and the agent reports `source=registry`, so the mirror branch is currently
unreached. Pacman's envelope avoids that defect: repository, requested-root marker, cache
locations, archive hash, and signature hash all live inside each signed backend payload.

The envelope carries two objects, and the executor answers with a third:

- `MutationManifest`: format, operation ID, target ID, backend, operation kind,
  backend-owned resolved-action payloads, and provenance/evidence digests.
- `MutationAuthorization`: manifest hash, authority kind/identity, target ID, issue and
  validity times, decision, key ID, and Ed25519 signature.
- `MutationReceipt`: the response half — the operation/manifest/authorization join, the
  decision and typed reason, exit code, verified-action count, and timestamp. It is
  **not signed**: the executor records what it did inside a boundary that already trusts
  it, and is not made a second authority by writing a receipt.

**`target_id` is the RedFlag `agent_id`.** Both copies are signed and a verifier requires
them equal; the executor compares them with the identity it reads for itself from a
root-owned SEC-021-validated file, exactly as it does for a closure token today. The field
name is generic so a later protocol may define another target namespace deliberately —
there is no second namespace today and no `body_id`.

Two shape rules bind the authority rather than the executor. `authorization_id` must be a
canonical UUID v4, the discipline standalone mint already applies to `request_id`, fixed
before that identifier becomes a replay key. And `expires_at - not_before` may not exceed
3600s — `DefaultTokenTTL`, the fleet minter's own window, enforced at signing as well as
at verification.

The manifest hash covers the exact UTF-8 JSON bytes of every backend payload. Fetch/cache
location therefore cannot change outside the signed object. Provenance evidence is a
separate field: a producer or signed-repository claim is not an execution location.

Canonical records are domain-separated and length-prefixed. Resolved actions and evidence
are sorted by canonical bytes, while exact duplicates are retained and change the hash. The
outer action collection is therefore an unordered multiset; a backend that needs ordered
steps must encode their order inside one backend payload. Unknown format values fail closed.

The canonical encoding is specified in `protocol/README.md`. One shared fixture under
`protocol/testdata/` pins canonical manifest bytes, manifest hash, authorization bytes, key
ID, signature, and receipt bytes/digest across Server, Agent, and helper. Tamper tests cover
provenance, execution location, target, backend, resolved action, duplicates, ordering,
authorization metadata, validity, decision, identifier shape, lifetime ceiling, and unknown
formats.

`redflag-helper verify-envelope` remains an inspection-only compatibility path: it proves
the common envelope and refuses without consuming replay state. `mint-envelope` is the
standalone pacman authority. It requires exactly one requested root, detached signatures,
strict operation semantics (`upgrade` moves that root forward; `install` introduces it),
and non-decreasing dependencies. `execute-envelope` repeats identity, hash, signature, and
version checks over a fresh root stage, atomically claims the authorization, and runs one
fixed pacman plan without `--needed`.

### Reuse, don't reinvent

The token extends the existing Ed25519 infrastructure rather than introducing new crypto:
- `server/internal/services/signing.go` — `SigningService` (Ed25519, `GetPublicKeyHex`,
  `GetCurrentKeyID` = SHA-256(pubkey)[:16] hex). The token is a new payload it signs.
- `server/internal/database/queries/signing_keys.go` — key storage + rotation/version.
- `agent/internal/crypto/verification.go` — verifies against active keys; v3 message format
  `{agent_id}:{id}:{type}:{sha256(params)}:{ts}`. The token mirrors this.
- `agent/internal/client/client.go::GetActivePublicKeys` — already "verify keys not servers."

---

## Component responsibilities

- **Server (fleet authority).** For dnf/apt, persist the agent-reported resolved entries, run OSV
  over that set, and mint a host-bound token after the approval boundary. The top-level hash
  is required, but the current agent may omit a dependency whose hash did not resolve. Full
  transitive resolution, the mirror tier, and signer process isolation remain target work.
- **Agent consumer (unprivileged).** Run discovery and resolution, receive or request the
  capability, confirm `agent_id` is this host, and hand it to the executor. It holds no
  signing key and has no direct package-manager mutation method. For pacman it owns a private
  sync database/cache and retains those files until mint and execution finish.
- **Executor (`helper/`, privileged, Rust).** Verify validity window → resolve trusted key by
  `key_id` from a local pinned keyring → reconstruct `signed_message` → Ed25519 verify →
  rehash each locally available artifact (and require mirror paths) → build a fixed argv plan
  → replay-guard on `token_id` → exec without a shell or inherited environment → structured
  result + exit code. For pacman it also stages archives and signatures into root custody,
  verifies the local Arch keyring, and refuses downgrades or operation-name lies. Legacy
  registry entries without local paths are not rehashed. The current unit is not
  network-isolated.
- **Kernel layer (where applicable).** Linux eBPF / Windows WDAC / macOS ESF deny
  package-manager execution except via the trusted executor. This is defense-in-depth design;
  the present eBPF scaffold is not wired to the capability model.

---

## Load-bearing constraints (target invariants)

Current deviations are named above and below. These constraints describe the boundary the
system is meant to reach; they must not be presented as deployed enforcement until code and
runtime evidence support them.

1. **Sign the resolved closure, not the top-level package.** Aggregate updates and the
   scheduler chaining dependencies mean the token must cover every transitive artifact and
   its hash. Authorizing only the top-level reopens the gap where the modern attacks live.
   Resolve-and-hash-the-closure is also the mirror's real security job; air-gapping from the
   registry is the bonus.
2. **The signer lives off the web process.** Server-as-authority plus server-as-mirror
   concentrates blast radius. The signing key must not be reachable from the request path
   (separate signer service / key material not loaded in the API process), so a web
   compromise cannot both mint tokens and serve artifacts.
3. **Verified-cache fallback, fail-closed only on change.** When installs route through the
   mirror, an already-approved-and-hashed artifact must still install from local cache if the
   server blinks. Fail closed on *new change*, not on a brief outage of an already-authorized
   operation.
4. **Verify keys, not servers.** The agent and helper trust *a public key (set)* identified by
   `key_id` fingerprint — never a server URL. Today the key lives on your server; nothing
   changes operationally. But the contract ("trust this key") lets the authority later be
   rotated, replicated, or held by a federation/guild node without touching the agent↔helper
   interface. This is the long-term cross-platform answer hidden in a one-line design choice.
5. **Kernel stops are defense-in-depth, not a prerequisite.** The capability model protects on
   a host where eBPF/WDAC/ESF cannot be deployed (locked-down managed box, constrained
   container). Kernel enforcement raises the cost of bypass; it does not gate whether the model
   means anything. Partial deployment still moves a host out of the soft-target category.
6. **No doctrinal knobs.** Signing required and forward-only (no downgrade) are ETHOS doctrine,
   not configurable. The token has no "skip verification" path.

---

## Gate Features

### 1. Version Pinning (Security Primitive)

**Normal model:** `npm install express` → resolves to `latest` → fetches from registry → installs

**RedFlag model today:** the exact version is resolved and its expected SHA256 is signed into
the capability. The helper enforces that hash when it receives a local artifact path. For a
normal registry entry without a local path, it fixes the package name/version in argv but does
not compare the fetched bytes with the signed hash; APT/DNF still relies on its signed
repository metadata. Making the installed artifact itself match the capability hash in every
case is the mirror-backed target.

The pin's hash source depends on who can reach the artifact:

- **npm / PyPI** — one canonical public registry exists, so the **server** fetches the
  artifact and computes the hash directly at approval (`computeAndStorePackageHash`).
- **dnf / apt** — artifacts come from each agent's own GPG-signed repos, which the server
  cannot reach. The **agent** resolves the canonical hash from its signed repo metadata at the
  dry-run step and reports it (`installer.ResolveArtifactSHA256`: dnf via `dnf download`+SHA256,
  apt via the `SHA256:` field of the signed index). The server pins what the agent reports.
  Trust is anchored in the repo signature; the pin is set before the install-time compromise
  the gate defends against.

> Historical note: an earlier draft of this doc specified a `ResolvePin` / `fetchAndHash`
> function and a `security_packages` table. Neither was built. The shipped registry is the two
> stores below. This section documents what exists.

### 2. Hash Registry (Layer 1)

Every name, version, and hash carried in a token is covered by its Ed25519 signature. That is
not the same as rehashing every installed byte. The helper's current verification boundary is:

- `source=mirror`: `artifact_path` is required; missing or mismatched bytes deny.
- Any entry with an existing local `artifact_path`: the helper rehashes it; mismatch denies.
- Normal `source=registry` with no local file: the hash remains signed into the token, but the
  helper does not rehash the bytes APT/DNF later fetches.

The top-level APT/DNF hash is mandatory before the server stores a closure. Dependency hash
resolution is best-effort and unresolved entries can be omitted. Complete closure staging and
helper-side verification of every byte remain the intended mirror-backed end state.

**Storage (as built):**
- `current_package_state.expected_sha256` (migration 040) — the pinned top-level hash per
  update. On normal registry-backed dnf/apt helper execution it is signed into authority but
  not rehashed against fetched bytes.
- `capability_tokens` (migration 042) — the minted, signed token carries the reported resolved
  set (per-artifact name/version/sha256/source) as JSONB. The token row *is* the closure
  record; there is no separate `security_packages` table.

OSV findings, package age, and published-at are recorded on the update's own `metadata` JSONB
at approval, not in a dedicated table.

### 3. Local Mirror (Optional)

**Purpose:** Decouple fleet from upstream availability after approval.

**Flow:**
1. Operator approves update → server fetches artifact → stores in local mirror
2. Server issues approval token with artifact path in mirror
3. Agent fetches from mirror (not upstream) → verifies SHA256 → installs

**When to enable:**
- Large fleets (>100 agents) where redundant fetches are noisy
- Upstream availability is a concern
- Maximum isolation desired

**Configuration:** `security.package_mirror.enabled` (boolean)

### 4. Time Gates: Package Age + Version Soak (live policies, v0.2.6.2)

Two distinct time-based gates, both under the `supply_chain.*` settings category, both
resolving env → config → DB → default. These are *policies* (configurable, with enforcement
modes) — unlike capability-signature validation and required local-artifact checks, which
have no skip setting.

**Approval-time age gate** (`package_age.go`) — the Shai-Hulud defense. Packages younger
than the threshold draw a warning or a block at approval:

| Setting | Default | Meaning |
|---------|---------|---------|
| `min_package_age_hours` | 24 | Minimum publish age before approval is clean |
| `gate_enforcement` | `warn` | `warn` or `block` |
| `block_unknown_age` | `false` | Opt-in: under `block`, registry-backed ecosystems (npm, PyPI) fail closed when the publish date can't be determined — a dark recency source is itself a Shai-Hulud-class signal. Default off, by sovereignty: the operator chooses to fail closed on unknowns. |

**Install-time version-soak gate** (`soak_gate.go` — this is GATE-005, *not* the age gate):

| Setting | Default | Meaning |
|---------|---------|---------|
| `soak_window_days` | 14 | A version must soak this long before the install path will take it |
| `soak_enforcement` | `block` | `warn` or `block` |

### 5. OSV.dev Integration

Top-level vulnerability scanning begins at **detection time** (moved in v0.2.6.2). After
the dnf/apt dry-run report, `checkClosureAndAdvance` queries OSV for the resolved entries the
agent reported; verdicts persist to package metadata (`supply_chain_vulns`,
`supply_chain_checked_at`) and gate auto-confirm/minting. An unresolved dependency omitted
from the report is not checked by this path.

Approval *reads* the persisted verdict — it does not re-scan. Any known vulnerability
among the reported entries checked is a full stop (see Enforcement Posture below); there is
no severity threshold below which approval proceeds quietly.

**Standalone path resilience.** In standalone mode there is no server, so the agent
queries OSV.dev directly before requesting a mint (`agent/internal/supplychain/osv.go`).
That client retries transient failures (transport error, 5xx, 429) with exponential
backoff and trips a process-wide circuit breaker after a run of failures, fast-failing
to `unreachable` instead of hammering the endpoint (ETHOS #3, SEC-029). The verdict
semantics are unchanged and remain fail-closed: an exhausted retry or an open breaker
surfaces as `unreachable` — never a silent clear — and `unreachable` still gates the
mint behind an explicit operator override. The resilience only avoids turning a transient
OSV blip into a forced operator action.

### 6. SLSA/Sigstore Attestation (Visibility Signal)

**Not a hard block** — surfaced as a visibility indicator.

**UI:** When unpinning a package without attestation:
```
This package lacks SLSA provenance or Sigstore signature.
Consider waiting for an attested release before proceeding.
```

---

## Enforcement Posture (v0.2.3.1, extended v0.2.6.2)

Approval is an enforcement point, not advisory. A known vulnerability in any reported entry
that was checked — top-level or transitive — is a hard stop: `ApproveUpdate` returns `409` and
mints nothing. For capability-gated ecosystems, a reported closure that OSV could not check
(service unreachable) is also a stop. This does not claim coverage for a dependency omitted
because its artifact hash did not resolve.

The only path through is an explicit operator override with a documented reason. The override
waives the vulnerability judgment only — the signed token still binds the reported artifact
entries, and the executor still validates authority plus any local artifact paths. Every
override writes a `supply_chain_override` system event. Bulk approve carries no blanket
override: flagged updates come back in
`blocked[]` and must be approved individually.

Auto-confirm shares the `ClosureCleared` predicate with manual approval — the two paths cannot
drift on what counts as a clean closure.

Standalone pacman does not claim that predicate. RedFlag has no Arch OSV ecosystem mapping
in this path, so the local gate records `unsupported` and requires an explicit operator
reason. Archive hashes and Arch package signatures still remain mandatory; the reason waives
only the missing advisory coverage.

---

## Build sequence (lineage)

The order the gate is built in. Per-step *status* is tracked in
`docs/tasks/GATE-000-supply-chain-gate-plan.md`; this records the intended dependency order.

1. `helper/` executor + token contract (the keystone; defines the schema in code).
2. Go `capability` token type + canonical encoder + Ed25519 sign/verify (server & agent share
   the definition; mirrored in both modules — no shared module exists).
3. Server: closure resolver + mint/sign at approval; signer off web process; endpoint to
   deliver tokens to the agent.
4. Migration: store resolved closure + per-artifact hashes alongside the pinned state.
5. Agent consumer: accept token, bind-check, pass to executor; fold in `rs-helper` parts.
6. Mirror tier (optional): pull+hash closure at approval; verified-cache fallback.
7. Kernel adapters wire the executor as the only permitted caller.

Steps 1–5 produced the capability-token path. Pacman then exercised the envelope migration:
private resolution, local root mint, detached signatures, custody, strict forward semantics,
execution, and receipt are wired in source. Fleet envelope mint/delivery, the mirror tier,
and kernel adapters remain.

---

## Cross-References

- **Build status / implementation tracking** → `docs/tasks/GATE-000-supply-chain-gate-plan.md`
- **Command signing** → `verification/01-signing-pipeline.md`
- **Agent verification** → `verification/02-agent-verification.md`
- **Replay protection** → `verification/04-replay-protection.md`
- **Trust boundaries** → `security/01-trust-boundaries.md`

---

## Footer: Assumptions & Connections

**Assumption:** The capability model is the floor; kernel-level primitives (eBPF, WDAC, ESF)
are defense-in-depth on top, not a prerequisite. Userspace wrappers alone are bypassable.

**Current:** The privileged executor has a narrow argv-only API, no shell, and a stripped
environment; the Agent that hands it capabilities is unprivileged and holds no signing key.
Pacman archives and detached signatures cross root custody before a fixed offline `pacman
-U`; legacy APT/DNF registry entries may still require network. The transient unit retains
host network access.

**Target:** Bring APT/DNF to the same full-custody envelope and enforce network isolation on
the helper unit. Neither property applies globally until each migrated backend proves it.

**Connection:** [security/01-trust-boundaries](01-trust-boundaries.md) (kernel enforcement as trust boundary)

**Connection:** [security/04-machine-binding](04-machine-binding.md) (agent identity verification)

---

*Last reviewed: 2026-09-01*

*Last reviewed: 2026-08-26*
