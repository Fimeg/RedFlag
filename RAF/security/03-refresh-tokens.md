# Refresh Token Lifecycle

**Forward-only token rotation with family revocation — a stolen config is a dead config.**

---

## Overview

Agents authenticate with short-lived JWTs minted against a long-lived refresh token (90-day TTL). Every renewal *rotates* the refresh token: a successor is minted, the old token is marked consumed. The rotation lineage is the security mechanism — replaying a consumed token is how theft announces itself.

**Cross-references:**
- [security/02-authentication-stack](02-authentication-stack.md) (where this sits in the four-layer stack)
- [security/04-machine-binding](04-machine-binding.md) (the renewal endpoint is machine-bound)

---

## Data Model

Migration 045. Each token row carries:

| Column | Purpose |
|--------|---------|
| `family_id` | Lineage identifier — all rotations of one registration share it |
| `consumed_at` | Set when the token is exchanged for a successor |
| `superseded_by` | Points at the successor token |

Tokens are stored hashed (`HashRefreshToken`), never plaintext. Queries live in `server/internal/database/queries/refresh_tokens.go`.

---

## The Renewal Flow

`RenewToken` (`server/internal/api/handlers/agents.go`):

1. **Machine binding first.** `X-Machine-ID` is checked against the registered host before any token logic. Mismatch → `403` + `MACHINE_ID_MISMATCH` security event. A stolen `config.json` replayed from another machine never reaches rotation.
2. **Locked read.** `GetRefreshTokenForRenew` uses `SELECT ... FOR UPDATE` — two concurrent renewals with the same token cannot both succeed.
3. **Classify the presented token:**

| State of presented token | Verdict | Action |
|--------------------------|---------|--------|
| Unconsumed, unexpired | Normal renewal | Mint successor, mark consumed |
| Consumed, successor **unconsumed** | Crash-recovery grace | Agent saved the old token but died before persisting the new one. Accept once; issue a fresh successor |
| Consumed, successor **also consumed** | Reuse = theft | Revoke the entire `family_id`, log security event, return terminal error |

The grace window is **accept-previous-once** — exactly one step back in the lineage, exactly once. Forward-only is doctrine ([core/01-ethos](../core/01-ethos.md)); there is no knob to widen it.

---

## Agent Side

- Terminal sentinel errors (`ErrRefreshTokenInvalid`, `ErrMachineMismatch`) stop the polling loop's retry machinery — these are not transient network failures and are logged `[CRITICAL]`. See [components/02-agent](../components/02-agent.md).
- The **instance lock** exists largely for this mechanism: two agent processes sharing one `config.json` would race rotations and trip family revocation on themselves. One config, one process, enforced by flock/mutex.

---

## Operational Notes

- Revoked family → agent must be re-registered with a fresh registration token. Runbook: `OPERATIONS.md §2`.
- After a database restore, agents may present tokens the restored DB has never seen (or sees as stale lineage). Expect re-registration; see `OPERATIONS.md §3`.
- `CleanupExpiredTokens` reaps expired rows; `RevokeAllAgentTokens` is the operator hammer.

---

## Why This Shape

A refresh token in a file on a fleet machine *will* eventually leak — backup snapshots, copied VMs, sloppy decommissioning. Rotation-with-family-revocation means a leaked token is only useful until the legitimate agent next renews, and *using* a stale one burns the whole family loudly. The failure mode is detection, not silent coexistence.

---

*Last reviewed: 2026-06-14*
