# Heartbeat System

**Agent health monitoring via system event polling.**

---

## Overview

RedFlag monitors agent health through a dedicated heartbeat mechanism. Agents periodically report status, and the server tracks operational state.

---

## Heartbeat Types

| Type | Trigger | Source | Purpose |
|------|---------|--------|---------|
| `manual` | Agent-initiated status check | Agent | Regular health report |
| `system` | Auto-triggered at dispatch | Server | Confirm command receipt |
| `command` | Command execution complete | Agent | Report execution status |
| `stuck` | Timeout exceeded | Server | Alert on stalled operations |

**Implementation:**
- `agent/internal/orchestrator/system_scanner.go`
- `server/internal/services/timeout.go`

---

## Heartbeat Flow

```
1. Agent executes command
2. Agent triggers heartbeat (system or command)
3. Server receives heartbeat event
4. Server updates agent metadata (heartbeat_source, last_heartbeat)
5. Server updates operational state (update_stuck_minutes countdown)
6. Dashboard displays current status
```

**Implementation:**
- Agent: `agent/internal/orchestrator/system_scanner.go:queueSystemHeartbeat()`
- Server: `server/internal/handlers/agents.go:HandleSystemHeartbeat()`
- Metadata: `agent.metadata.heartbeat_source`

---

## Timeout Configuration

**Operational timeouts:**
- `operational.update_stuck_minutes`: Default 5 minutes
- `operational.agent_update_timeout_minutes`: Default 15 minutes
- Configured via `security_settings` table

**Watchdog behavior:**
- If command exceeds `update_stuck_minutes` → mark as stuck
- If update exceeds `agent_update_timeout_minutes` → trigger rollback
- Timeout events logged to history table

---

## Footer: Assumptions & Connections

**Assumption:** Heartbeat is a best-effort mechanism — agent crashes will be detected on next poll cycle.

**Connection:** Heartbeat system (`flows/04-heartbeat.md`) implements ETHOS #3 (assume failure).

**Connection:** Timeout configuration (`flows/04-heartbeat.md`) ties to security settings (`security_settings.operational`).

**Connection:** System events (`flows/04-heartbeat.md`) published to history table for audit trail.

**Connection:** Auto-heartbeat gates (`flows/04-heartbeat.md`) controlled by `policy.auto_heartbeat_enabled`.

---

*Last reviewed: 2026-05-26*
