# Capability Advertisement

**Dynamic scanner capability reporting from agents to server.**

---

## Overview

Agents dynamically report which scanners they have available. The server maintains a live view of agent capabilities without requiring registration updates.

---

## Capability Detection

**Process:**
```
1. Agent checks each scanner's availability (DetectAvailable())
2. Agent reports available scanners in check-in payload
3. Server diff against existing subsystem rows
4. Server inserts new scanners, updates existing
5. Server removes stale scanners (if configured)
```

**Implementation:**
- Detection: `agent/internal/scanner/*.go:DetectAvailable()`
- Sync: `server/internal/api/handlers/agents.go:syncAvailableScanners()`

---

## Scanner Types

| Scanner | Platform | Detection Method |
|---------|----------|------------------|
| APT | Linux | Check `/var/lib/apt/lists/lock` |
| DNF | Linux | Check `dnf version` availability |
| Winget | Windows | Check `winget` CLI availability |
| WUA | Windows | Check WindowsUpdate Agent service |
| Docker | All | Check Docker socket/CLI availability |
| Package (upstream) | All | Check upstream registry sync capability |

---

## Sync Pattern (Idempotent)

**INSERT new scanners:**
```sql
INSERT INTO agent_subsystems (agent_id, name, enabled)
VALUES ($1, $2, true)
ON CONFLICT (agent_id, name) DO NOTHING
```

**UPDATE existing:**
```sql
UPDATE agent_subsystems SET enabled = true WHERE agent_id = $1 AND name = $2
```

**No DELETE on transient signal:**
- Brief `IsAvailable()=false` doesn't remove scanner
- Removal requires explicit admin action or stale threshold

---

## Footer: Assumptions & Connections

**Assumption:** Capabilities can change post-registration (Docker installed after agent deploy).

**Connection:** Capability sync (`flows/05-capability-advertisement.md`) implements ETHOS #4 (idempotent scanner sync).

**Connection:** `syncAvailableScanners()` (`flows/05-capability-advertisement.md`) called on every agent check-in.

**Connection:** Capability badges (`flows/05-capability-advertisement.md`) rendered in `AgentHealth.tsx` dashboard.

**Connection:** Scanner detection (`agent/internal/scanner/*.go:DetectAvailable()`) called during registration and polling.

---

*Last reviewed: 2026-05-26*
