# Replay Protection

**Multi-layer defense against command replay attacks.**

---

## Overview

RedFlag implements defense-in-depth against replay attacks at three layers:
1. **Nonce validation** (temporal binding for update commands)
2. **Command deduplication** (disk-based ID tracking)
3. **Timestamp validation** (command age limits)

---

## Layer 1: Update Nonce Validation

**Purpose:** Bind update commands to specific time window.

**Mechanism:**
- Server generates nonce with max age = 2× check-in interval
- Nonce embedded in `update_agent` command signature
- Agent must present valid nonce within expiry window
- Server validates nonce age before execution

**Implementation:**
- Server: `server/internal/services/update_nonce.go`
- Validation: `server/internal/middleware/machine_binding.go:validateNonce()`
- Nonce expiry: 2× `check_in_interval` (default 10 minutes)

**Security Model:**
```
Command dispatched → Nonce generated (age=0)
Agent receives → Nonce cached (age < maxAge/2)
Agent executes → Nonce validated (age < maxAge)
Command executed → Nonce consumed (single-use)
```

---

## Layer 2: Command Deduplication

**Purpose:** Prevent duplicate execution after agent restart.

**Mechanism:**
- Executed command IDs persisted to `executed_commands.json`
- 4-hour max age window (aligns with command TTL)
- Atomic writes prevent corruption

**Implementation:**
- Storage: `agent/internal/orchestrator/executed_commands.json`
- Load: `loadExecutedCommands()`
- Add: `executedIDs.Add(cmd.ID)`
- Save: `saveExecutedCommands(executedIDs)`

**Deduplication Flow:**
```
Command received → Check executed IDs
If duplicate → Reject with security event
If unique → Add to executed IDs
Execute command
```

---

## Layer 3: Timestamp Validation

**Purpose:** Reject stale commands regardless of nonce.

**Mechanism:**
- Commands must have `created_at` within TTL window
- Default TTL: 4 hours
- Server rejects commands older than TTL

**Implementation:**
- Validation: `server/internal/handlers/agents.go:validateCommandTimestamp()`
- TTL: 4 hours (configurable via `security_settings.command_ttl_hours`)

---

## Combined Protection Flow

```
┌─────────────────────────────────────────────────────────────┐
│                    Command Execution                         │
├─────────────────────────────────────────────────────────────┤
│  1. Agent polls server for commands                          │
│  2. Server returns signed command with nonce                 │
│  3. Agent validates nonce age (Layer 1)                      │
│  4. Agent checks executed_commands.json (Layer 2)            │
│  5. Agent executes command                                   │
│  6. Agent records command ID in executed_commands.json       │
│  7. Command times out (Layer 3) → rejected on retry          │
└─────────────────────────────────────────────────────────────┘
```

---

## Circuit Breaker Integration

**Purpose:** Prevent replay during scanner failures.

**Mechanism:**
- Circuit breaker per subsystem (APT, DNF, Winget, WUA, Docker)
- Opens after N failures in T window
- Blocks all scanner commands while open

**Implementation:**
- `agent/internal/circuitbreaker/circuitbreaker.go`
- Failure threshold: 5 failures in 60 seconds
- Open duration: 5 minutes

---

## Footer: Assumptions & Connections

**Assumption:** Replay attacks are rare — protection is defense-in-depth, not primary security.

**Connection:** Nonce validation (`verification/04-replay-protection.md`) complements machine binding (`security/02-authentication-stack.md`).

**Connection:** Command deduplication (`verification/04-replay-protection.md`) implements ETHOS #4 (idempotency).

**Connection:** Circuit breaker (`verification/04-replay-protection.md`) implements ETHOS #3 (assume failure).

**Connection:** Nonce service (`server/internal/services/update_nonce.go`) ties to security settings (`security_settings.operational`).

---

*Last reviewed: 2026-05-26*
