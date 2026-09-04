# Key Rotation Support

**Ed25519 signing key rotation without downtime.**

---

## Overview

RedFlag supports rotating the server's Ed25519 signing key while maintaining agent trust continuity via the TOFU (Trust On First Use) model.

---

## Key Storage

**Server-side:**
- Private key: `REDFLAG_SIGNING_PRIVATE_KEY` (env var or Docker secret)
- Public key: Stored in database table `server_public_keys`
- Key format: Ed25519 (32-byte curve25519)

**Agent-side:**
- Public key cached at registration (`~/.config/redflag/server_keys.json`)
- Cached keys validated against active keys in database
- TOFU: First key accepted becomes trusted forever

---

## Rotation Process

### Server-Side Rotation

```
1. Generate new key pair (Ed25519)
2. INSERT new public key into server_public_keys (status = 'pending')
3. Update server config to use new private key
4. Sign next command with new key
5. Agents verify signature against new key (fails old key check)
6. Agents accept new key as valid (TOFU update)
7. Old key marked deprecated in database
8. Old keys removed after grace period
```

**Implementation:**
- `server/internal/services/signing.go:SetPrimaryKey()`
- `server/internal/database/migrations/035_server_public_keys.up.sql`

---

## Agent Verification Flow

```
1. Agent receives signed command
2. Extract public key from signature (if v3 format)
3. Check if key exists in cached trusted keys
4. If not cached → fetch from server_public_keys table
5. Validate key signature against command
6. Accept key as trusted (TOFU)
7. Update local cache
```

**Implementation:**
- `agent/internal/crypto/pubkey.go` (TOFU key caching)
- `agent/internal/crypto/verification.go:VerifyCommandWithTimestamp()`

---

## Key Lifecycle States

| State | Description | Database Field |
|-------|-------------|----------------|
| `active` | Currently signing commands | `status = 'active'` |
| `pending` | Newly added, awaiting agent adoption | `status = 'pending'` |
| `deprecated` | Old key, still accepted | `status = 'deprecated'` |
| `revoked` | Compromised or removed key | `status = 'revoked'` |

---

## Security Considerations

### During Rotation
- Old key remains `active` until new key is confirmed by agents
- Grace period prevents agents from being orphaned
- No downtime for command dispatch

### Key Compromise
- Immediately revoke compromised key
- Force agents to re-fetch public key list
- Affected commands can be replayed until key revocation propagates

### Forward-Only Enforcement at the Agent Key Path

A rotated-out or revoked key must stop being trusted. Two agent-side rules make
that hold even when the agent cannot reach the server (SEC-028):

- **Bounded stale cache.** When the public-key fetch fails (network down), the
  agent keeps serving its last cached key only within a bounded staleness
  window. Past the window — or when the cache age cannot be established (no
  metadata sidecar) — it fails closed rather than trusting a possibly
  rotated-out key indefinitely. Acceptance within the window is surfaced at
  ERROR, not WARNING: it is a degraded-trust state, not routine.
  - The **window length** is operator policy: security setting
    `command_signing.stale_key_max_age_hours` (default 168h / 7d), delivered
    fleet-wide via `GET /api/v1/agents/:id/config` and overridable per
    deployment for sites with long offline windows.
  - The **existence of a fail-closed ceiling is doctrine, not a knob.** The
    agent clamps any configured value to `[1h, 30d]` (`SetStaleKeyMaxAge`) and
    the server rejects out-of-range writes (1–720h validation). No setting and
    no tampered local config can disable the ceiling or set it to infinite.

- **Active-set refusal.** When a command names a `key_id` the server's active
  set does not contain, the agent refuses verification (returns an error that
  the command handler surfaces as a verification failure) instead of falling
  back to the primary cached key. A key the server has rotated out is dead.

**Implementation:**
- `agent/internal/crypto/pubkey.go` (`FetchAndCacheServerPublicKey`, `SetStaleKeyMaxAge`)
- `agent/internal/crypto/verification.go:CheckKeyRotation()`
- `server/internal/services/security_settings_service.go` (`command_signing.stale_key_max_age_hours` default + validation)

### TOFU Limitations
- Compromised initial key → all future keys trusted
- Mitigation: Monitor agent registration patterns
- Mitigation: Periodic key rotation limits exposure window

---

## Footer: Assumptions & Connections

**Assumption:** Key rotation is a rare operation — expected once per year or less.

**Connection:** Rotation process (`verification/03-key-rotation.md`) complements TOFU caching (`verification/02-agent-verification.md`).

**Connection:** `server_public_keys` table enables rotation without code changes.

**Connection:** Agent-side key caching (`agent/internal/crypto/pubkey.go`) implements TOFU.

---

*Last reviewed: 2026-05-26*
