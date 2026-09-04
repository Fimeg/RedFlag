# Trust Boundaries

**Complete trust boundary matrix for all endpoints.**

---

## Overview

Every HTTP endpoint must be classified by who is allowed to call it and which middleware enforces that classification.

**Cross-references:**
- [security/02-authentication-stack](02-authentication-stack.md) (auth layers)
- [security/03-refresh-tokens](03-refresh-tokens.md) (token lifecycle)
- [security/04-machine-binding](04-machine-binding.md) (machine ID binding)

---

## Trust Boundary Matrix

| Trust Boundary | Group | Middleware | Example Routes | Notes |
|----------------|-------|------------|----------------|-------|
| **Public** | `public` | None | `GET /api/v1/install/:platform` | Rate-limited per-IP |
| **Public** | `public` | None | `GET /api/v1/downloads/:platform` | Rate-limited per-IP, no signature when version="latest" |
| **Public** | `public` | None | `POST /api/v1/agents/register` | Uses registration token |
| **Public** | `public` | In-handler machine binding | `POST /api/v1/agents/renew` | Refresh token **+ X-Machine-ID must match the registered host**. Refresh token presented from a different machine → 403 (stolen-token replay defense, logged as machine_id_mismatch). |
| **Agent** | `agent-auth` | `AuthMiddleware + MachineBindingMiddleware` | `GET /api/v1/agents/:id/commands` | Requires JWT + correct machine ID |
| **Agent** | `agent-auth` | `AuthMiddleware + MachineBindingMiddleware` | `POST /api/v1/agents/:id/reports` | Requires JWT + correct machine ID |
| **Agent** | `agent-auth` | `AuthMiddleware + MachineBindingMiddleware` | `POST /api/v1/agents/:id/logs` | Requires JWT + correct machine ID |
| **Agent** | `agent-auth` | `AuthMiddleware + MachineBindingMiddleware` | `POST /api/v1/agents/:id/rebind-machine-id` | Admin-initiated machine rebind |
| **Agent** | `agent-auth` | `AuthMiddleware + MachineBindingMiddleware` | `GET /api/v1/downloads/updates/:package_id` | Download signed agent packages |
| **Web** | `web-auth` | `WebAuthMiddleware` | `GET /api/v1/dashboard/*` | Admin dashboard |
| **Web** | `web-auth` | `WebAuthMiddleware` | `GET /api/v1/agents/*` | Agent management |
| **Web** | `web-auth` | `WebAuthMiddleware` | `GET /api/v1/settings/*` | Settings pages |
| **Web** | `web-auth` | `WebAuthMiddleware` | `GET /api/v1/updates/*` | Update management |
| **Web** | `web-auth` | `WebAuthMiddleware` | `GET /api/v1/docker/*` | Docker integration |
| **Web** | `web-auth` | `WebAuthMiddleware` | `GET /api/v1/history/*` | History tracking |
| **Admin** | `admin-only` | `WebAuthMiddleware + RequireAdmin()` | `POST /api/v1/admin/*` | Admin-only operations |
| **Admin** | `admin-only` | `WebAuthMiddleware + RequireAdmin()` | `DELETE /api/v1/admin/agents/:id` | Delete agent (BUG-013 fix) |

---

## Trust Boundary Details

### Public Trust Boundary

**Endpoints:**
- `GET /api/v1/install/:platform?token=<reg_token>&arch=<arch>`
- `GET /api/v1/downloads/:platform?version=<ver>`
- `POST /api/v1/agents/register`
- `POST /api/v1/agents/renew`

**Security Notes:**
- Rate-limited per-IP via `RateLimit("public_access", KeyByIP)`
- Registration tokens are one-time use
- Download endpoint has BUG-003: signature header not set when `version="latest"`
- `/renew` is deliberately on the public route group, not behind `AuthMiddleware`: the agent calls it *because* its JWT has expired, so requiring a valid JWT to renew would be circular. It authenticates with the refresh token in the body instead. It is **not** trust-free, though — the handler reloads the agent and requires `X-Machine-ID` to match the bound machine. This closes the gap where a refresh token (a long-lived on-disk secret) would otherwise mint access tokens from any machine for 90 days.

**Refresh-token rotation + reuse detection — IMPLEMENTED (migration 045, 2026-05-29).** Each refresh token belongs to a *family* (`family_id`) and carries `consumed_at` + `superseded_by`. The state machine in `RenewToken` (`server/internal/api/handlers/agents.go`):

| Presented token state | Action |
|---|---|
| not found | 401 invalid |
| revoked, family still live | **revoke family** + security event → 401 (revoked-token replay is anomalous) |
| expired | 401 (bounded by the token's own 90d window) |
| unconsumed (`consumed_at IS NULL`) | normal rotation: mint successor, mark parent consumed, return new token |
| consumed, successor **unconsumed** | **accept-previous-once** grace: agent crashed before saving the successor (provably never used it) → orphan that leaf, mint a fresh one, return it |
| consumed, successor **consumed/revoked/missing** | **reuse detected** → revoke family + security event → 401 |

Grace is *structural*, not timed: "successor still unconsumed" is the discriminator, bounded by the parent's own 90d expiry. The agent persists the rotated token on each renewal (`loop.go`); a failed persist is recovered by the grace path on the next attempt. All revoke/reuse paths are loud (`LogUnauthorizedAccessAttempt`) and fail-closed — both the legitimate agent and any thief lose access, forcing deliberate human re-registration.

**Residual limitation (documented, not a TODO):** a *perfect same-machine lockstep shadow* — an attacker on the bound host who reads `config.json` and renews in exact alternation with the legit agent — is not detectable by rotation alone, because every token is used exactly once per party and the chain never diverges. This is inherent to all refresh-token rotation. It is mitigated by machine binding (the outer gate: a different host → 403 before rotation runs) and is out of scope for rotation; a same-host root attacker has already won at the OS layer.

**Still human-gated (unchanged):** **admin import / re-grant** — a one-time token authorizing exactly one rebind/registration when an operator moves an agent's identity to new hardware. Deliberately not automated (Casey: "human gated today" — revisit only if/when agent sophistication warrants, ~not near-term). Distinct from accept-previous-once, which is automatic crash-recovery internal to rotation.

Ties to SEC-012 (renewal atomicity): the server side is now fully transactional; the agent↔server two-phase (server commits rotation / agent persists token) is reconciled by the grace path rather than true cross-network atomicity.

**Cross-references:**
- [flows/01-registration](../flows/01-registration.md) (registration flow)
- [flows/03-agent-upgrade](../flows/03-agent-upgrade.md) (download endpoint)

---

### Agent Trust Boundary

**Middleware Chain:**
1. `AuthMiddleware` — Validates JWT with issuer `"redflag-agent"`
2. `MachineBindingMiddleware` — Validates X-Machine-ID matches DB

**Security Notes:**
- JWT expires after 24 hours
- Refresh token extends expiry to 90 days
- Machine ID mismatch returns 403 Forbidden
- Agent row deletion returns 401 Unauthorized
- **Download endpoint** (`GET /api/v1/downloads/updates/:package_id`) requires machine binding to prevent any authenticated agent from downloading any package

**Cross-references:**
- [security/02-authentication-stack](02-authentication-stack.md) (JWT validation)
- [security/04-machine-binding](04-machine-binding.md) (machine ID validation)

---

### Web Trust Boundary

**Middleware:** `WebAuthMiddleware` — Validates JWT with issuer `"redflag-web"`

**Security Notes:**
- JWT expires after 24 hours
- Requires `admin` role claim
- Admin-only routes use `AdminRoleMiddleware`

**Cross-references:**
- [security/02-authentication-stack](02-authentication-stack.md) (web JWT)

---

### Admin Trust Boundary

**Middleware Chain:**
1. `WebAuthMiddleware` — Validates JWT with issuer `"redflag-web"`
2. `RequireAdmin()` — Checks `admin` claim is true

**Security Notes:**
- Can delete agents (was BUG-013: was registered under agent-auth group)
- Can revoke agent tokens
- Can trigger machine rebind

**Cross-references:**
- [security/03-refresh-tokens](03-refresh-tokens.md) (token revocation)

---

### Session Broker Trust Boundary

A **separate privileged Rust binary** spawned on demand for Tier 4 (break-glass /
interactive) sessions. The broker has its own trust boundary, its own network connection
to the server, and its own audit trail. It is not a sub-component of the agent or helper.

**Spawn mechanism:** agent writes a root-owned tmpfile with the minted grant, then
spawns the broker via `sudo systemd-run --pipe --property=ProtectSystem=no
redflag-broker --grant <tmpfile>`. The broker reads the grant once, verifies the
Ed25519 signature independently against the pinned keyring, deletes the tmpfile, and
opens its own WebSocket/gRPC connection to the server.

**Security Notes:**
- Grant is scoped: one agent, one operator, one scope (shell/desktop/script)
- Grant is time-boxed: hard ceiling enforced by the broker, not advisory
- Every command input is hash-chained in an append-only audit log
- Session end: broker signs the audit chain, emits a receipt, exits
- Agent is out of the loop after spawn — cannot influence broker execution
- No package operations — that's the helper's job; no persistent connections
- **Requires RBAC** for grant minting — design complete, gated behind RBAC substrate

**Cross-references:**
- [components/06-session-broker](../components/06-session-broker.md) (design of record)
- [components/04-helper](../components/04-helper.md) (sibling binary, same spawn pattern)

---

### Local Trust Boundary (agent localapi)

Not a server HTTP boundary — this one lives on the **agent host**. The agent exposes a
local-only API for the desktop tray app over a Unix socket
(`/var/lib/redflag/agent/localapi/redflag-agent.sock`) on Linux and a named pipe
(`\\.\pipe\RedFlagAgentLocal`) on Windows.

**Endpoints** (`agent/internal/localapi/server.go`): `/v1/identity`, `/v1/status`,
`/v1/scans/latest`, `/v1/packages`, `/v1/tokens/active`, `/v1/desktop` (tray health
report), `/v1/actions/trigger-scan`, `/v1/actions/approve-update`.

**Authentication is the operating system, not credentials.** There are no tokens on
this surface by design — access is gated by filesystem permissions:

- Socket directory `0750`, socket `0660`, both group-owned by `redflag-local`
  (`agent/internal/localapi/listener_unix.go`).
- The installer creates the group, enrolls the agent user, and enrolls the detected
  desktop user (`linux.sh.tmpl` step 7c).
- A user not in `redflag-local` gets `EACCES` at connect — the kernel is the
  middleware.

**Known sharp edge:** group membership is stamped onto a login session at login.
Adding a user to `redflag-local` does not grant running sessions access; the user must
log out and back in. The desktop app diagnoses this case explicitly (in-group-on-disk
vs in-group-in-session, `desktop/src/main.rs`) instead of surfacing a raw permission
error. Installs that predate the desktop-user enrollment step never ran it; the
upgrade path re-asserts socket-chain ownership but does not re-check membership
(tracked: INSTALL-001 post-install healthcheck).

**Cross-references:**
- [components/05-desktop](../components/05-desktop.md) (the only intended client)

---

## Anti-Pattern: BUG-013

**Problem:** `DELETE /api/v1/agents/:id` was an admin operation registered under the agent-auth group with `MachineBindingMiddleware`.

**Symptom:** Admin request returned 401 Unauthorized (missing X-Machine-ID) because admin JWT doesn't have machine binding.

**Fix:** Moved endpoint to `admin-only` group with `WebAuthMiddleware + AdminRoleMiddleware`.

**Cross-references:**
- [core/01-ethos](../core/01-ethos.md) (principle #2: Security is Non-Negotiable)
- [flows/02-command-execution](../flows/02-command-execution.md) (polling loop)

---

## Structural Enforcement: Boot-Time Route Audit

BUG-013 was a one-off fix; the route audit (`server/internal/routeaudit/`) is the
structural answer to that class. At startup the server walks every registered route
in the Gin engine and verifies each handler chain carries the auth middleware its
trust boundary requires. Any route that lacks auth and is not on the explicit public
allowlist refuses boot: `[CRITICAL] route_missing_auth`, exit 1. An unauthenticated
endpoint cannot ship by omission — it can only exist as a reviewed line in
`PublicPathSet`.

**Classification is by code-pointer identity, not symbol name.** Each trust boundary
has exactly one middleware instance, created once in `main`, used at every route, and
registered with the auditor (`RegisterAuth`). Name-based matching was tried and
retired: compiler inlining renames closure symbols (missing real middleware), and
substring matching can silently accept a colliding name as an auth boundary. Pointer
identity over shared instances fails loudly in the safe direction — a stray fresh
constructor call or unregistered wrapper flags its routes at boot instead of passing
them silently.

**Cross-references:**
- [core/01-ethos](../core/01-ethos.md) (principle #2: no unauthenticated endpoints)

---

## Doctrine: Pull-Only Agent Channel

The agent↔server control channel is **pull-only**. The agent polls; the server never
opens a connection to an agent and never pushes commands at one. This is doctrine
(Casey, 2026-06-10), not a configuration choice — same tier as signing-required and
forward-only.

**Why:** a push channel is a standing inbound control path on every endpoint. Pull keeps
the agent in charge of when it listens, and keeps the server compromise blast radius
bounded by what agents choose to fetch and verify.

**Consequences:**
- Reject designs that assume server-initiated delivery: live-query campaigns,
  push-config, server-side websockets to agents.
- A websocket/push channel is a *maybe later*, and **not until it is PQC-ready**
  (post-quantum cryptography). Until then, latency wants are served by rapid-mode polling.
- Server→*operator-owned third party* outbound emits (SIEM, asset DB — see
  `docs/tasks/INTEG-001`, `INTEG-002`) are a different channel and unaffected: outbound,
  no listener, no control surface.

---

## Cross-References

- **Auth layers** → [security/02-authentication-stack](02-authentication-stack.md)
- **Refresh tokens** → [security/03-refresh-tokens](03-refresh-tokens.md)
- **Machine binding** → [security/04-machine-binding](04-machine-binding.md)
- **Registration** → [flows/01-registration](../flows/01-registration.md)
- **Command execution** → [flows/02-command-execution](../flows/02-command-execution.md)

---

*Last reviewed: 2026-06-14*