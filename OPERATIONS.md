# RedFlag Operations Runbook

Operator procedures for RedFlag deployments. Scope: hardware rebind, agent credential
renewal/rotation and the instance lock, disaster recovery, signing key rotation.

---

## 1. Hardware Change / Machine Rebind

When an agent is migrated to new hardware (motherboard swap, VM rebuild, disk image clone), the recorded `machine_id` will diverge from what the agent now reports. The agent will be rejected by `MachineBindingMiddleware` and every command will be denied.

### Symptoms
- Agent shows online but every command returns `403 unauthorized` in the server log: `[WARN] [server] [middleware] machine_id_mismatch`.
- Token renewal also fails: `POST /api/v1/agents/renew` returns `403` and the agent logs a terminal `ErrMachineMismatch` (see §2). A rebind that does not also restore a usable refresh token will leave the agent unable to renew.
- `security_events` table has rows of type `MACHINE_ID_MISMATCH` for the affected agent.

### Procedure
1. Verify the agent's host. Do **not** rebind unless you can confirm the agent is on the expected hardware.
2. As an admin, call the rebind endpoint:
   ```bash
   curl -X POST https://<server>/api/admin/agents/<agent-uuid>/rebind-machine-id \
        -H "Authorization: Bearer <admin-jwt>" \
        -H "Content-Type: application/json" \
        -d '{"reason": "hardware replacement 2026-05-24"}'
   ```
3. The endpoint clears the stored machine_id and on next check-in the new value is bound. Audit row written to `security_events`.
4. Restart the agent if it does not recover within one heartbeat window.

The endpoint is rate-limited (`admin_operations`) and requires `WebAuthMiddleware`. There is no agent-side procedure — rebind is admin-driven.

> Do not clone or copy a *running* agent's `config.json` onto a second host as a shortcut. It carries a machine-bound refresh token; the clone will fail the machine check on its first renewal and, if the original keeps running, can trip refresh-token reuse detection and revoke the whole family (§2). Re-enroll the new host instead.

---

## 2. Agent Credentials: Renewal, Rotation, and the Instance Lock

An agent holds two credentials in `config.json`: a short-lived access token (JWT) and a long-lived refresh token. The access token is presented on every request; the refresh token is used only at `POST /api/v1/agents/renew` to mint a new access token. Renewal is machine-bound and the refresh token rotates on every use.

### What rotation means operationally
- Each successful renewal returns a **new** refresh token and invalidates the old one. The agent persists the successor to `config.json`.
- A one-time grace accepts the immediately previous token once more, so an agent that renews but crashes before persisting can still recover on restart.
- Presenting a refresh token after its successor has already been used is treated as **reuse** (the fingerprint of a stolen, replayed token). RedFlag revokes the **entire token family** and writes a critical `security_event`. This is forward-only — there is no un-revoke. Recovery is re-enrollment of the agent.
- Renewal is also machine-bound: the refresh token is checked against `X-Machine-ID` before rotation. A stolen `config.json` replayed from another host gets `403` + `MACHINE_ID_MISMATCH`, never a token.

### Symptoms of a revoked family
- Agent logs a terminal `ErrRefreshTokenInvalid` (or `ErrUnauthorized`) and the polling loop stops — these are not retried.
- `security_events` shows a refresh-token reuse / family-revocation row.
- Recovery: re-enroll the agent (fresh registration). Do **not** attempt to hand-edit tokens back into `config.json`.

### The instance lock
Two agent processes must never share one `config.json` — they would race each other's renewals and trip reuse detection. The agent takes an exclusive instance lock at startup (`agent/internal/instancelock/`): a Unix `flock` on Linux/macOS, a named kernel mutex `Global\RedFlagAgent_v1` on Windows. A second process starting against the same config fails fast.

- Symptom of a lock conflict: the agent exits immediately at startup logging that another instance holds the lock.
- This is also why imaging/cloning a running agent is unsafe (§1): the clone either fails the lock (same host) or fails the machine check and trips reuse (different host).

---

## 3. Disaster Recovery

### What to back up
| Asset | Where | Cadence |
|-------|-------|---------|
| PostgreSQL database (`redflag`) | `pg_dump` of the database container | nightly |
| `.env` / Docker secrets | filesystem outside docker volumes | on change |
| Ed25519 signing private key | offline secure storage (password manager / hardware key) | once, at setup, **and on every rotation** |
| Server TLS material (if terminating at server) | filesystem | on change |

The signing private key is the most critical asset. Without it the server cannot sign new agent commands and all existing agents will continue accepting only the old key until their next public-key fetch.

### Restore order
1. Provision a host with the same Docker stack version.
2. Restore the `.env` (or recreate secrets) — including `REDFLAG_SIGNING_PRIVATE_KEY`.
3. Start the postgres container with restored data volume (or `pg_restore` into a fresh volume).
4. Start the server. Confirm migrations apply cleanly (server logs `[INFO] migrations applied N`).
5. Start agents in batches. Watch `security_events` for `MACHINE_ID_MISMATCH` (clone hosts will need rebind, §1).
6. Verify command flow end-to-end: queue a `scan_apt` (or platform equivalent) to one agent and confirm `agent_commands.status = 'success'`.

### Refresh tokens after a database restore
A restored database carries the refresh-token state as of the backup. An agent that renewed *after* the backup was taken now holds a token the restored server considers already-rotated. On first renewal this can read as reuse and revoke the family (§2). Expect a wave of agents needing re-enrollment proportional to renewal activity between the backup and the restore — keep the backup cadence tight relative to the renewal interval, and treat post-restore re-enrollment as a normal step, not an incident.

### What cannot be restored without the signing private key
- Outgoing commands the server signs. Restoring the database is not enough — agents reject unsigned commands in strict mode.
- If the key is lost: generate a new keypair, write it to the database (`signing_keys` table — see §4), and accept that there will be a transitional window where commands signed with the new key require agents to fetch the new public key before they will execute.

---

## 4. Signing Key Rotation

Implemented via the `signing_keys` table (migration 020). The table holds versioned keys; agents fetch all currently-active public keys on registration and on demand, so two keys can be valid simultaneously for a controlled transition.

### When to rotate
- Suspected compromise of the private key.
- Operator-mandated cadence (recommend annually).
- Personnel change where the prior key holder no longer needs access.

### Procedure
1. **Generate** a new Ed25519 keypair on a clean host (offline if possible):
   ```bash
   cd server
   go run ./cmd/keygen   # if not present, generate via openssl ed25519
   ```
   Output: hex-encoded 64-byte private key and 32-byte public key.

2. **Register the new key** as active alongside the existing one. The signing service has `InitializePrimaryKey()` for first registration; for rotation, insert directly into the `signing_keys` table with the next version number and `status = 'active'`:
   ```sql
   INSERT INTO signing_keys (key_id, version, public_key_hex, status, created_at)
   VALUES (gen_random_uuid(), (SELECT COALESCE(MAX(version), 0) + 1 FROM signing_keys), '<hex>', 'active', NOW());
   ```
   Both old and new keys are active. Agents fetch both via the public-key endpoint.

3. **Swap server signing key**: update `REDFLAG_SIGNING_PRIVATE_KEY` in `.env` (or the Docker secret) to the new private key. Restart server.

4. **Verify**: queue a command to one agent. Confirm the agent verifies the new signature. Repeat across a representative sample of agents.

5. **Revoke the old key** after a transition window (recommend 7 days, longer if you have agents that may be offline):
   ```sql
   UPDATE signing_keys SET status = 'revoked', revoked_at = NOW() WHERE version = <old_version>;
   ```
   Agents will reject commands signed by revoked keys on their next public-key refresh.

6. **Securely destroy** the old private key copies (password managers, escrow, sealed envelopes).

### Rollback
If a rotation causes broad agent failure, revert step 3 (put the old private key back in `.env`, restart server). The old key remains `active` in `signing_keys` until you mark it `revoked`. The transition window exists precisely to allow this rollback.

---

## 5. Supply Chain Checks — Advisory vs. Gate

There are three postures, and they are not the same thing. Don't conflate them.

**Advisory display (fail-open).** The discovery-time OSV badge (`CheckOSVVulnerabilities`) and the package age gate in `warn` mode are informational. If the upstream service is unreachable, the check logs a `[WARNING]` and returns nil — the dashboard just loses a hint. This is deliberate: an OSV.dev outage should not blind you to a clean dashboard, and a warn-mode age finding is a note, not a wall.

**Approval gate (full stop, audited override).** Approving an update is an enforcement point, not advisory. A known vulnerability — top-level *or* anywhere in the resolved dependency closure — is a hard stop: `ApproveUpdate` returns `409` and mints nothing. For capability-gated ecosystems (dnf/apt), a closure OSV could not check (service unreachable) is also a stop — minting over it would trust unverified artifacts. The only way through is an explicit operator override in the request body:

```
POST /api/v1/updates/:id/approve   { "override_supply_chain": true, "override_reason": "<why>" }
```

The reason is required. The override waives the **vulnerability judgment only** — the signed token still binds the real artifact hashes and the executor still verifies signature + hash. There is no skip-verify path; this is not a runtime knob, it is a per-decision human call. Every override writes a `supply_chain_override` `system_event` (component `security`) recording the package, cause, and operator reason. Bulk approve carries **no** blanket override: flagged updates come back in `blocked[]` and must be approved individually, each with its own reason.

The age gate in `block` enforcement is a separate full stop (still env-driven: `REDFLAG_SUPPLY_CHAIN_GATE_ENFORCEMENT`), handled before the vuln gate.

**Auto-confirm (fail-closed).** The automatic confirmation sweep never trusts the void: an unchecked or vulnerable closure is never auto-minted (`models.ClosureCleared`, shared with the approval gate so the two cannot drift). A human is the only path past a flagged closure.

**Coverage:**
- OSV.dev: npm, PyPI, apt (Debian), dnf (AlmaLinux) — coverage varies by ecosystem
- Package age gate: npm (registry.npmjs.org), PyPI (pypi.org)

**Monitoring:** Search server logs for `[SECURITY] [server] [supply_chain]` — `approval_blocked` (a gate stop), `bulk_approval_blocked`, and `gate_overridden` (an operator trusted the void). Overrides also surface in the events API as `supply_chain_override`. Upstream advisory failures stay at `[WARNING]`; grep `supply_chain` / `package_age` for chronic upstream unavailability.

---

## 6. Rate Limiter — In-Memory, Restart Semantics

The API rate limiter is **in-memory only** (per-key sliding windows in the server
process). A server restart clears all counters. This is a known, accepted
characteristic — there is no Redis/DB backing store.

**Restart penalty (SEC-004).** For the first 60 seconds after boot, every limit
runs at **half its configured budget** (minimum 1 request). This makes a forced
restart strictly worse for an attacker trying to reset their counters, while
legitimate agents — each rate-limited under its own key — reconnect comfortably
within half budget.

**Operational notes:**
- A burst of `429`s in the minute after a deploy/restart is the grace penalty
  working, not a misconfiguration. It clears itself at T+60s.
- Limits are operator-tunable at runtime via `/api/v1/admin/rate-limits`; the
  grace penalty halves whatever is configured at request time.
- If you see repeated unexplained server restarts combined with high request
  volume from one source, treat it as a possible counter-reset attempt and
  block at the firewall — the limiter alone cannot fully stop an attacker who
  can crash the server.
