# Session Broker Component

**A privileged, per-session, network-capable Rust binary that executes Tier 4 (break-glass / interactive) operations under a minted, scoped, time-boxed grant — spawned on demand, exits when the session ends.**

---

**Language: Rust.** Same language as the helper — shared Ed25519 keyring, signature verification, grant parsing, and audit trail primitives. Five binaries, two languages: agent (Go), server (Go), helper (Rust), broker (Rust), streamer (Rust). Reference implementations cloned to `Projects/` for design study: Sunshine (C++, Moonlight server), RustDesk (Rust, full remote desktop), MeshCentral (Node.js, full RMM), Tactical RMM (Python+Go, RMM), moonlight-web-stream (Rust+TS, Sunshine-to-browser bridge — the direct ancestor of the streamer).

---

## The five-binary architecture

The existing architecture has three artifacts with clean boundaries. Tier 4 adds two: the broker (security surface) and the streamer (protocol surface).

| Component | Network | Lifetime | Privilege | Purpose |
|-----------|---------|----------|-----------|---------|
| Agent (Go) | None (pull-only) | Persistent daemon | Unprivileged | Poll, scan, process tokens |
| Helper (Rust) | Host network currently reachable; isolation intended | One-shot | Privileged (systemd-run) | Verify authority + execute one mutation |
| Desktop (Rust/Tauri) | None (local socket only) | Persistent | Unprivileged | Local tray status |
| **Broker (Rust)** | WebSocket to server | Per-session | Privileged (systemd-run) | Grant verification, session lifecycle, audit trail |
| **Streamer (Rust)** | WebRTC/WS to browser, ENet to Sunshine | Per-session | Privileged (systemd-run) | Moonlight protocol, video decode, input injection |

The broker and streamer are **separate binaries** by design:

- **Broker is the security surface.** Grant verification, Ed25519 checks, audit logging, systemd-run spawning. ~1,500 lines, minimal dependencies, auditable in one sitting. Always shipped.
- **Streamer is the protocol surface.** WebRTC, moonlight-common-rust, Moonlight handshake, video decode, input injection. Heavy dependencies (`webrtc` crate), complex protocol code. **Opt-in** — installed only on hosts where the operator wants remote desktop capability. Not shipped by default.
- **Attack surface isolation.** A CVE in the `webrtc` crate hits the streamer, not the broker. The broker's grant model remains intact.
- **Update independence.** The streamer can be updated when Moonlight protocol changes without touching the broker.

Tier 4 needs a live interactive channel — streaming I/O, return data, a WebSocket or gRPC connection to the server. None of the existing components can carry this without breaking their design constraints:

- **Agent:** pull-only doctrine forbids server-initiated connections; grafting a push channel onto the agent would destroy the posture that makes it safe.
- **Helper:** one-shot by design. Network isolation is also the target, but the current helper unit does not enforce it; a persistent interactive channel would still violate the intended boundary.
- **Desktop:** local-only by design; no server connectivity.

The session broker is the fifth artifact. It reuses the agent's spawn pattern (`sudo systemd-run --pipe`) but has its own trust boundary, its own network connection, and its own audit trail.

---

## Doctrine

1. **The grant is the gate.** The broker cannot start without a valid, unexpired, Ed25519-signed grant that specifies exactly what it may do. No grant, no session.
2. **The agent never holds session state.** The agent verifies the grant and spawns the broker. After that, the agent is out of the loop. The broker owns the session lifecycle.
3. **The broker is disposable.** Spawned on demand, lives for the session, exits when the grant expires or the operator disconnects. No daemon, no persistent state, no background listener.
4. **Server-side recording is the primary audit record.** The server independently records everything it relays through the session channel. The on-host audit log is corroboration, not the source of truth. A compromised broker can still act outside the channel — this is the honest claim, not "every executed command is captured."
5. **The broker is the most-privileged transient on the host.** It runs as root with `ProtectSystem=no` and a network connection. This is justified only because the grant gates it tightly: short-lived, scoped, reason-documented, RBAC-controlled. Every design choice flows from making this surface as narrow as possible.
6. **The helper stays untouched.** The broker is a separate binary with a separate trust boundary. It does not extend, wrap, or share code with the helper beyond the shared signing infrastructure (keyring, Ed25519 verification).

---

## Grant format

The session grant extends the existing Ed25519 infrastructure. It is a new payload type signed by the same authority key, verified against the same pinned keyring.

```jsonc
{
  "version": 1,
  "grant_type": "session",
  "grant_id": "<uuid>",            // unique; replay guard + audit key
  "agent_id": "<uuid>",            // bound to exactly one host
  "operator_id": "<uuid>",         // the operator who minted this
  "scope": "shell|desktop|script",
  "runbook_id": "<uuid, optional>",// for scope=script: which pre-signed runbook
  "max_duration_seconds": 1800,    // hard ceiling on session runtime
  "issued_at": <unix>,
  "not_before": <unix>,
  "expires_at": <unix>,            // gates STARTING the session (recommended: 15 min)
  "authority_key_id": "<hex, 32>", // which authority key signed this
  "signature": "<hex ed25519>"     // over the canonical message below
}
```

**Canonical signed message** (length-prefixed, no delimiter ambiguity):

```
len(grant_id) || grant_id ||
len(agent_id) || agent_id ||
len(operator_id) || operator_id ||
len(scope) || scope ||
len(runbook_id) || runbook_id ||
len(expires_at) || expires_at ||
len(max_duration_seconds) || max_duration_seconds
```

Each field is prefixed with its byte length (u32 big-endian) followed by the field bytes. This eliminates delimiter-injection: no field can contain a separator that would shift parsing boundaries. The signed message is deterministic and language-agnostic — Go and Rust reconstruct identical bytes.

> **Why not colon-delimited?** GATE-005 taught this lesson: fields containing the delimiter create signing/verification asymmetry. Length-prefixing is unambiguous regardless of field content. UUIDs and enums today, but the contract holds for any charset.

### Duration semantics

`expires_at` and `max_duration_seconds` serve different purposes and must not conflict:

- **`expires_at` gates starting.** A session cannot begin after this timestamp. The broker rejects `session_start > expires_at`.
- **`max_duration_seconds` gates running.** The session hard-stops at `min(expires_at, session_start + max_duration_seconds)`, whichever comes first.
- A grant minted with `expires_at` 15 min out and `max_duration` 1800s: a session started at minute 14 gets 1 minute, not 30 more.
- The broker enforces the ceiling internally; the server enforces it at the relay layer (closes WebSocket at the hard-stop).

### Scope variants

| Scope | What it authorizes | Safety mechanism |
|-------|-------------------|-----------------|
| `shell` | Arbitrary commands in a live shell session | Server-side recording, time-boxed, operator identity bound |
| `desktop` | Moonlight/Sunshine remote desktop via broker relay | Server-side stream recording, time-boxed, operator identity bound |
| `script` | Execute one pre-signed runbook by ID | Hash-pinned artifact (Tier 3 primitive inside Tier 4) |

### Desktop scope: one protocol, two modes

Moonlight/Sunshine is the desktop scope. Not because it's perfect for both support and workstation use, but because it's the best single protocol that covers both without protocol sprawl:

- **USB passthrough** — forward USB devices to the remote session (keyboard, mass storage, smart cards)
- **Clipboard sharing** — bidirectional copy/paste
- **Audio forwarding** — capture and stream host audio
- **GPU-encoded** — low latency on hardware that has a GPU
- **Headless-capable** — Sunshine captures any display output; works on servers without a physical monitor (virtual display or dummy plug)
- **Cross-platform** — Windows, Linux, macOS clients and hosts

The support-grade requirements (NAT traversal, session recording, survives-reboot, end-user consent) are handled by the **broker relay layer**, not by the protocol itself:

- **NAT traversal** — the broker opens the WebSocket to the server; the operator connects through the server relay. No port-forwarding needed on the agent host. The server relays the Moonlight stream, adding a hop but solving the NAT problem.
- **Session recording** — the server records the relayed stream for audit. Not as clean as command logging (it's video, not text), but it captures what the operator saw and did.
- **Survives-reboot** — Sunshine runs as a system service; the broker can reconnect after transient failures within the grant window.
- **End-user consent** — if a user is present, the broker can prompt before allowing the session. If unattended (headless server), the grant itself is the consent.

SPICE and VNC remain available as fallbacks for environments that can't run Sunshine (no GPU, VM passthrough via QEMU/KEMU). But Moonlight/Sunshine is the first-class citizen — the grant format and broker architecture are designed around it, and the fallback protocols get a best-effort integration path, not a first-class grant type.

**Honest tradeoff:** Moonlight requires Sunshine on the host (the capture/encode server). If Sunshine isn't installed or the GPU can't encode, the desktop scope degrades gracefully to "the operator sees a static screenshot and can type commands" (shell scope with visual context). That's a valid fallback, not a failure.

---

## Invocation

The agent spawns the broker via the same mechanism it uses for the helper:

```bash
sudo systemd-run --pipe \
  --property=ProtectSystem=no \
  --property=ReadWritePaths="/var/lib/redflag/sessions" \
  redflag-broker --grant /tmp/redflag-grant-<uuid>.json
```

The grant file is:
1. Written by the agent to a root-owned tmpfile (world-inaccessible)
2. The broker reads it once at startup, verifies the signature, deletes the file
3. The broker opens its own connection to the server using the grant as auth
4. When the session ends (grant expires, operator disconnects, or explicit close), the broker exits and the transient systemd unit is cleaned up

The agent's sudoers entry adds a second invocation line:

```
redflag-broker --grant <path>
```

Same pattern as the helper, same narrowed sudoers philosophy: the agent can spawn the broker, but cannot influence what the broker does after it starts. The broker reads the grant, verifies it independently, and operates within its scope.

### `ProtectSystem=no` blast radius

The broker unit is the most-privileged transient thing on the host: root, full filesystem access, a network connection, and an interactive lifetime. The current helper unit is also root with `ProtectSystem=no` and retains host network access; that is a helper gap, not a safety property the broker design may rely on. The broker's interactive protocol surface and longer lifetime still make its blast radius wider. The tighter gating compensates:

- Grant is short-lived (15 min max recommended TTL)
- Grant is scoped to one operator, one agent, one scope
- Grant requires documented reason + RBAC authority
- Server-side recording captures everything relayed through the channel
- Session is logged to a tamper-evident audit trail
- The host is treated as suspect after break-glass use (see §Post-session posture)

If this blast radius is unacceptable for a deployment, that's a valid deployment choice — don't ship Tier 4. The architecture doesn't require it; Tiers 1–3 cover most ops.

---

## Three verifications (defense in depth)

The grant is verified at three points, by three different components:

| Verifier | When | What it checks |
|----------|------|---------------|
| **Agent** | Before spawn | Signature against keyring, agent_id matches this host, not expired |
| **Broker** | At startup | Signature against keyring (independently), expiry, scope, grant_id not consumed |
| **Server** | At session-open | Grant signature, grant_id not already opened, RBAC still valid, not expired |

Three verifications by three independent components. A forged grant would need to pass all three. A replayed grant would fail the server's second check (grant_id already consumed). A stolen grant would fail the agent's machine-binding check (agent_id mismatch).

---

## Trust chain

```
┌─────────────────────────────────────────────────────────────┐
│  Human Operator                                             │
│  - Requests break-glass session (reason required)           │
│  - Higher authority than package approval                   │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│  RedFlag Server (authority)                                 │
│  - Validates operator has Tier 4 permission (RBAC)          │
│  - Validates operator can access this specific agent        │
│    (RBAC with agent-scoping, not just a boolean)            │
│  - Requires documented reason for the session               │
│  - Mints Ed25519-signed session grant                       │
│  - Grant is scoped: one agent, one operator, one scope      │
│  - Records grant_id, refuses second session-open            │
│  - Streams relay through server (primary audit record)      │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│  RedFlag Agent (gatekeeper, unprivileged)                   │
│  - Receives grant, confirms agent_id is this host           │
│  - Verifies signature (same keyring as package tokens)      │
│  - Writes grant to root-owned tmpfile                       │
│  - Spawns broker via sudo systemd-run                       │
│  - Agent is out of the loop after spawn                     │
└─────────────────────┬───────────────────────────────────────┘
                      │
┌─────────────────────▼───────────────────────────────────────┐
│  Session Broker (privileged, network-capable, Rust)         │
│  - Reads grant file, verifies signature independently       │
│  - Records grant_id in consumed set (replay guard)          │
│  - Opens WebSocket to server; grant is the client cred      │
│  - Server validates grant again at session-open (3rd check) │
│  - Enforces hard-stop: min(expires_at, start + max_duration)│
│  - Emits receipt to server on session end                   │
│  - Disposable: spawned per-session, no persistent state     │
└─────────────────────────────────────────────────────────────┘
```

---

## Audit trail

The audit model has two layers, and the distinction matters:

### Primary: server-side recording

The server independently records everything relayed through the session channel:
- Every command the operator typed (captured at the relay layer, before reaching the broker)
- Every output frame the broker sent back (captured at the relay layer)
- Session metadata: operator_id, agent_id, grant_id, start/end timestamps, reason

This is the authoritative audit record. It exists on a machine the broker cannot reach. A compromised broker can still act outside the channel (see §Honest limitations), but the server's copy captures what was *relayed* — and the broker's ability to act undetected is bounded by the grant's short lifetime.

### Secondary: on-host corroboration

The broker writes a hash-chained log locally:

```
/var/lib/redflag/sessions/<grant_id>/
├── audit.log          # hash-chained: each line includes prev_hash
├── receipt.json       # structured result (agent_id, operator, duration, exit_code)
└── receipt.sig        # Ed25519 signature over receipt (broker's ephemeral key)
```

The hash chain works as follows:
- Each log entry: `{timestamp}: {prev_hash}: {event}`
- The chain starts from a genesis hash derived from the grant_id
- At session end, the broker generates a **per-session ephemeral Ed25519 keypair**, signs the final receipt with it, and includes the ephemeral public key in the receipt
- The server records the ephemeral public key alongside the receipt

**Why an ephemeral key, not the authority key?** The authority key is the system's highest-value secret. The broker is the most-attackable process on the host (root, network-capable, running untrusted operator input). Giving it the authority key means a compromised broker can mint tokens for any operation on any host. The ephemeral key signs *only this receipt* and is discarded with the session. The server can verify the receipt signature using the embedded ephemeral public key, confirming the receipt was produced by the broker that held this grant — without the broker ever holding the authority key.

The on-host log is corroboration: useful for local debugging, forensics on a live host, and proving to the host operator what happened during a session they may not have witnessed. But it is not the primary audit record, because the author of the log (the broker, running as root) could theoretically have written anything. The server's relay recording is the one that matters for accountability.

### Hash chain structure (on-host corroboration)

```
genesis = sha256(grant_id)
entry_0 = { "ts": <unix>, "prev": genesis, "event": "session_open", "operator": <id> }
entry_1 = { "ts": <unix>, "prev": sha256(entry_0), "event": "input", "data": "<command>" }
entry_2 = { "ts": <unix>, "prev": sha256(entry_1), "event": "output", "data": "<result_frame>" }
...
entry_n = { "ts": <unix>, "prev": sha256(entry_n-1), "event": "session_close", "reason": "<expir|disconnect|close>" }
receipt = { "grant_id", "agent_id", "operator_id", "start_ts", "end_ts", "duration_s",
            "commands_relayed": <count>, "exit_code": <int>, "ephemeral_pubkey": "<hex>" }
receipt.sig = ed25519_sign(ephemeral_priv, sha256(receipt))
```

---

## Honest limitations

The following are real gaps, not TODOs. They are accepted design tradeoffs, not bugs to fix:

**The broker is the author of the on-host audit trail.** A compromised broker can omit commands from its local log (omission doesn't break a hash chain the way mutation does). This is why the server-side relay recording is the primary record and the on-host log is corroboration. The broker cannot reach the server's copy.

**What the operator typed and what the broker ran may differ.** The server sees operator input at the relay layer; the broker sees the shell's actual execution. A compromised broker could execute something different from what was relayed. The honest claim is: "every command relayed through the session is recorded server-side; a compromised broker can still act outside the channel, which is why the grant is short-lived and the host is treated as suspect after break-glass use." Root can do anything, including lie about what it did.

**`ProtectSystem=no` + network = wide blast radius.** The broker is the most-privileged transient on the host. The tighter gating (short-lived, scoped, reason-documented, RBAC-controlled, server-recorded) compensates. If a deployment finds this unacceptable, don't ship Tier 4 — Tiers 1–3 cover most ops.

**Desktop scope is not production-ready.** Moonlight/Sunshine integration (broker relay, server-side stream recording, Sunshine service management) is a large implementation. The grant format and broker architecture accommodate it, but the actual streaming, rendering, and connection management should not be assumed close to shipping.

---

## Grant replay protection

Grant theft between mint and spawn is a real vector on a compromised host. The replay guard works like the helper's token-ID recording:

1. **Broker records consumed grant_ids.** At startup, after verifying the grant, the broker writes `grant_id` to a consumed set (root-owned, same location as the audit log). If the grant_id is already in the set, the broker refuses to start.
2. **Server refuses second session-open.** When the broker opens a WebSocket and presents the grant, the server checks `grant_id` against its DB. If a session was already opened for this grant_id, the server rejects the connection.
3. **Grant_id is a UUID v4.** Unpredictable, so an attacker cannot guess valid grant_ids to probe.

The enforcement points are: broker (local replay guard) and server (global replay guard). The agent does not enforce replay — it's out of the loop after spawn.

---

## Sequence: break-glass session

```
1. Operator clicks "Request Shell Session" in dashboard
   - Required: reason text, target agent, session scope
   - Server validates RBAC (operator has Tier 4 permission for this agent)
   
2. Server mints session grant
   - Signs grant with authority key
   - Stores grant in DB with status: pending
   - Records grant_id (replay guard)
   
3. Agent polls, receives grant
   - Verifies signature against keyring
   - Confirms agent_id matches this host
   - Writes grant to root-owned tmpfile
   
4. Agent spawns broker
   - sudo systemd-run --pipe redflag-broker --grant <tmpfile>
   
5. Broker starts, verifies grant independently
   - Reads tmpfile, deletes it
   - Verifies signature (same Ed25519 check)
   - Records grant_id in consumed set
   - Checks expiry and scope
   - Opens WebSocket to server; presents grant as client credential
   
6. Server validates grant again (third verification)
   - Checks signature (defense in depth)
   - Checks grant_id not already opened
   - Checks RBAC still valid (not revoked since mint)
   - Checks not expired
   - Opens relay channel
   
7. Live session
   - Operator types commands in browser
   - Server relays to broker via WebSocket (server records input)
   - Broker executes, streams output back (server records output)
   - Broker hash-chains events in on-host corroboration log
   
8. Session ends (grant expires, operator closes, or explicit disconnect)
   - Server closes relay at hard-stop: min(expires_at, start + max_duration)
   - Broker generates ephemeral keypair, signs receipt
   - Broker emits receipt to server
   - Server records receipt, closes grant
   - On-host: audit.log + receipt.json + receipt.sig written
   
9. Agent is unaffected — it moved on after step 4
```

---

## The streamer binary (opt-in)

The streamer is a **separate Rust binary** that handles the Moonlight protocol and WebRTC streaming to the browser. It is not shipped by default — installed only on hosts where the operator wants remote desktop capability.

### Lineage

Forked from `moonlight-web-stream` (590 stars, Rust+TypeScript, GPL-3.0). That project bridges Sunshine/Moonlight to a browser via WebRTC. RedFlag's streamer forks the Rust components and strips the independent auth:

| moonlight-web-stream | RedFlag streamer |
|---------------------|-----------------|
| Own user/password auth | Removed — RedFlag grants replace it |
| SQLite user DB | Removed — RedFlag server owns identity |
| Standalone web server | Removed — RedFlag server relays |
| `moonlight-common-rust` (Moonlight client) | **Kept** — the protocol engine |
| `webrtc` crate (browser streaming) | **Kept** — the transport to browser |
| Streamer subprocess model | **Kept** — one subprocess per session |
| WebRTC + WebSocket fallback | **Kept** — works in restrictive networks |
| TURN server support | **Kept** — for NAT traversal |

### What it does

1. Receives connection parameters from the broker (Sunshine host/port, session ID, encoding prefs)
2. Connects to Sunshine via Moonlight protocol (ENet, UDP)
3. Negotiates screen capture, GPU encoding, input injection with Sunshine
4. Streams the video/audio to the browser via WebRTC (or WebSocket fallback)
5. Forwards keyboard/mouse/gamepad input from browser back to Sunshine
6. Reports session metadata back to the broker for audit

### What it does NOT do

- **No grant verification** — that's the broker's job. The streamer trusts the broker to have verified the grant.
- **No audit logging** — the broker handles the audit trail. The streamer is a protocol engine, not a security component.
- **No persistent state** — spawned per-session, exits when the session ends.
- **No Sunshine management** — the broker starts/stops Sunshine. The streamer just connects to it.

### Dependencies

| Crate | Purpose |
|-------|---------|
| `moonlight-common-rust` | Moonlight client protocol (git dependency) |
| `webrtc` | WebRTC peer connection to browser |
| `tokio` | Async runtime |
| `actix-web` (optional) | Minimal HTTP for WebRTC signaling |

### Security posture

The streamer runs as root (via `systemd-run`) because it needs to inject input and capture audio. This is the same privilege level as the broker. The attack surface is:

- **Moonlight protocol parsing** — complex, but well-audited in moonlight-common-rust
- **WebRTC stack** — large dependency, most likely place for a vuln
- **Input injection** — root-level keyboard/mouse control

A vulnerability in the streamer can compromise the host during an active session. This is accepted because:
- The streamer only runs during an active, grant-scoped session
- The session is time-boxed (max_duration)
- The session is recorded (server-side relay recording)
- The host is treated as suspect after break-glass use

The broker remains unaffected by streamer vulnerabilities — it's a separate binary with separate dependencies.

---

## The RedFlag streaming page (web UI)

The RedFlag dashboard needs a new page: **Streaming** (or **Remote Desktop**). This is the operator-facing viewer that connects to the streamer via WebRTC.

### Page requirements

| Feature | Notes |
|---------|-------|
| WebRTC video/audio decode | Browser-native, H.264/AV1 via WebDecoder API |
| Keyboard input | Keyboard Lock API (requires HTTPS secure context) |
| Mouse input | Pointer Lock API, relative mouse movement |
| Gamepad input | Gamepad API — works with USB and Bluetooth controllers paired to the **operator's machine** (not the agent host) |
| Clipboard sync | Bidirectional copy/paste via WebRTC data channel |
| Display selection | Multi-monitor support (pick which display to view) |
| Quality controls | Resolution, framerate, bandwidth negotiation |
| Session info | Grant ID, operator, time remaining, agent name |
| Disconnect button | Clean session teardown |

### Controller support

The browser's Gamepad API supports USB and Bluetooth controllers connected to the **operator's machine** — not the agent host. The operator's controller input is captured in the browser, sent via WebRTC data channel to the streamer, which injects it into Sunshine as gamepad input.

This means:
- The operator can use a Bluetooth Xbox/PlayStation/Switch controller at their desk
- The input is forwarded to the agent host as if the controller were plugged in there
- No Bluetooth stack on the agent host is required — Sunshine sees emulated gamepad input

For an RMM break-glass context, keyboard and mouse are the primary input methods. Gamepad support is a bonus for workstation-use scenarios (CAD, media production) but not a core requirement.

### Page architecture

```
┌─────────────────────────────────────────────────┐
│  RedFlag Dashboard (React SPA)                  │
│                                                 │
│  ┌─────────────────────────────────────────┐    │
│  │  Streaming Page                          │    │
│  │                                          │    │
│  │  ┌──────────────────────────────────┐   │    │
│  │  │  WebRTC Video Canvas              │   │    │
│  │  │  (H.264/AV1 decode via browser)   │   │    │
│  │  └──────────────────────────────────┘   │    │
│  │                                          │    │
│  │  ┌──────────┐ ┌──────────┐ ┌────────┐  │    │
│  │  │ Quality  │ │ Displays │ │ Session│  │    │
│  │  │ Controls │ │ Selector │ │  Info  │  │    │
│  │  └──────────┘ └──────────┘ └────────┘  │    │
│  └─────────────────────────────────────────┘    │
│                                                 │
└──────────────────────┬──────────────────────────┘
                       │ WebRTC (video + data channel)
                       ▼
┌─────────────────────────────────────────────────┐
│  RedFlag Server (relay)                         │
│  - WebRTC signaling (offer/answer/ICE)          │
│  - Relay video/audio/input data channels        │
│  - Session recording at relay layer             │
└──────────────────────┬──────────────────────────┘
                       │ WebSocket
                       ▼
┌─────────────────────────────────────────────────┐
│  RedFlag Streamer (on agent host)               │
│  - Moonlight protocol to Sunshine               │
│  - WebRTC peer to browser (via server relay)    │
│  - Input injection (keyboard, mouse, gamepad)   │
└──────────────────────┬──────────────────────────┘
                       │ ENet (UDP)
                       ▼
┌─────────────────────────────────────────────────┐
│  Sunshine (on agent host, managed by broker)    │
│  - Screen capture (GPU)                         │
│  - Video encoding (NVENC/VAAPI/AMF)             │
│  - Audio capture                                │
└─────────────────────────────────────────────────┘
```

---

## Post-session posture

After a break-glass session, the host is treated as suspect. This is not optional — it's a consequence of granting root shell access to a remote operator. The server should:

1. **Flag the agent** in the dashboard (visual indicator: "break-glass session used")
2. **Recommend a closure reconciliation** — the operator should verify the host's package closure against the signed manifest and reconcile any drift
3. **Optionally trigger an automatic scan** — re-scan the host's subsystems to capture post-session state

This is not an enforcement mechanism — a determined attacker who had root during the session can defeat post-session scanning. It's a signal to the operator that the host's trust posture should be re-verified.

---

## Prerequisite: RBAC with agent-scoping

Tier 4 grants require more than a boolean "has Tier 4 permission." For MSP deployments, an operator with Tier 4 should not get a root shell on every tenant's hosts. The RBAC substrate must support:

- **Per-operator agent scoping** — which agents can this operator break-glass into?
- **Per-operator scope restrictions** — can this operator request `shell`, or only `desktop_support`?
- **Reason auditing** — every Tier 4 grant carries a documented reason, queryable by auditors

The current system is single-admin with an inert `/admin` latch. The session broker design is complete and ready, but **cannot ship credibly until RBAC with agent-scoping lands** — a break-glass path without role-gated minting is just "anyone can get a root shell on any host."

The RBAC work is tracked separately. The session broker should be built to the grant format above but remain gated behind the RBAC substrate.

---

## Cross-references

- **Capability token model** → [security/05-supply-chain-gate](../security/05-supply-chain-gate.md) (the primitive this extends)
- **Helper architecture** → [components/04-helper](04-helper.md) (the sibling binary this parallels)
- **Trust boundaries** → [security/01-trust-boundaries](../security/01-trust-boundaries.md) (the trust matrix this adds to)
- **Pull-only doctrine** → [security/01-trust-boundaries](../security/01-trust-boundaries.md) §Doctrine (why the broker opens its own connection, not the agent)
- **Standalone mode** → [security/06-standalone-authority](../security/06-standalone-authority.md) (standalone mint pattern, same spawn mechanism)
- **GATE-005** → `docs/tasks/GATE-005-helper-trusted-input-hardening.md` (delimiter-injection lesson that informed the canonical message format)
- **moonlight-web-stream** → `Projects/moonlight-web-stream/` (forked from — Rust+TS, Sunshine-to-browser bridge, 590 stars)
- **Sunshine capture** → `Projects/Sunshine/` (C++ — screen capture, GPU encode, service model, vendored for capture engine)
- **MeshCentral relay** → `Projects/MeshCentral/meshrelay.js` (relay pair-up pattern, session recording format, rights model)
- **RustDesk relay** → `Projects/RustDesk/` (Rust — rendezvous pattern, tokio async model)

---

*Last reviewed: 2026-06-30*
