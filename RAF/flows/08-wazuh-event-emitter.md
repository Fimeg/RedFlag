# RAF Flow 08 — Wazuh Event Emitter (INTEG-001)

**Status:** Implemented (2026-06-11)
**Source:** `server/internal/integrations/wazuh/`

## What

RedFlag emits security events to a local Wazuh agent's queue socket
(`/var/ossec/queue/sockets/queue`) in ECS (Elastic Common Schema) format.
Outbound-only: the emitter opens a Unix DGRAM socket and writes; it opens no
listener and accepts no inbound traffic. RedFlag's own `security_events` journal
remains the source of truth; Wazuh is a best-effort mirror.

## Architecture

```
SecurityLogger.Log(event)
  → INSERT INTO security_events (authoritative)
  → if sink != nil: sink.Emit(event)  (best-effort mirror)
      → wazuh.Emitter.Emit
        → json.Marshal(toECS(event))
        → prefix "1:redflag:" (Wazuh queue protocol)
        → Unix DGRAM send to /var/ossec/queue/sockets/queue
```

## Design decisions

### Outbound-only, no control surface

The emitter is a writer. It never listens. A compromised Wazuh agent or a
malicious socket cannot send commands or data back into RedFlag. This is the
same trust model as the pull-only agent↔server channel: RedFlag pushes out,
never accepts instructions in.

### Best-effort mirror, not a transaction

The journal write succeeds first, then the sink fires. If the Wazuh socket is
down, the event is dropped and counted — the journal has it. This keeps the
security event path from coupling to an external system's availability.

### Lazy connect, one retry

The DGRAM socket is opened on first Emit, not at startup. If a write fails
(Wazuh agent restarted, socket recreated), one reconnect is attempted. After
that the event is dropped with a rate-limited log warning (≤ 1/minute).

### Opt-in only

Wiring is gated on `REDFLAG_WAZUH_ENABLED=true`. Without it the `Sink` is nil
and no socket is ever opened — zero overhead, zero log noise.

## Event mapping

RedFlag event types → Wazuh custom rule IDs (999xxx user range):

| RedFlag Event | Rule ID | Level |
|---|---|---|
| (unknown / future) | 999001 | 3 |
| CMD_SIGNATURE_VERIFICATION_FAILED | 999002 | 12 |
| UPDATE_NONCE_INVALID | 999003 | 12 |
| UPDATE_SIGNATURE_VERIFICATION_FAILED | 999004 | 12 |
| MACHINE_ID_MISMATCH | 999005 | 12 |
| AUTH_JWT_VALIDATION_FAILED | 999006 | 12 |
| AGENT_REGISTRATION_FAILED | 999007 | 7 |
| UNAUTHORIZED_ACCESS_ATTEMPT | 999008 | 7 |
| CONFIG_TAMPERING_DETECTED | 999009 | 12 |
| ANOMALOUS_BEHAVIOR | 999010 | 7 |
| CMD_SIGNED | 999011 | 3 |
| CMD_SIGNATURE_VERIFICATION_SUCCESS | 999012 | 3 |

## ECS shape

```json
{
  "@timestamp": "2026-06-11T01:00:00Z",
  "event": {
    "kind": "alert",
    "category": ["security"],
    "type": ["info"],
    "module": "redflag",
    "action": "MACHINE_ID_MISMATCH",
    "outcome": "failure",
    "severity": 9
  },
  "rule": {
    "id": "999005",
    "level": 12,
    "description": "machine binding violation"
  },
  "agent": {"id": "f7ddc5ce-..."},
  "message": "machine binding violation",
  "redflag": {},
  "wazuh": {
    "integration": {
      "name": "redflag",
      "category": "security",
      "decoders": ["json"],
      "rules": ["999005"]
    }
  }
}
```

## Operator-side setup

1. Wazuh agent or manager on the same host as the RedFlag server.
2. `REDFLAG_WAZUH_ENABLED=true` in the server environment.
3. Optionally `REDFLAG_WAZUH_SOCKET` to override the socket path (defaults to
   `/var/ossec/queue/sockets/queue`).
4. The Wazuh ruleset (`docs/wazuh-ruleset.xml`) installed on the Wazuh manager
   so custom rule IDs decode to proper alerts.

## Cross-references

- **Trust boundaries** → [../security/01-trust-boundaries](../security/01-trust-boundaries.md) — pull-only
  doctrine section; this emitter is explicitly distinguished from the agent
  control channel.
- **Task spec** → `docs/tasks/INTEG-001-wazuh-event-emitter.md`
- **Verdicts umbrella** → `docs/tasks/INTEG-000-competitive-landscape-verdicts.md`
- **Snipe-IT (next integration)** → `docs/tasks/INTEG-002-snipeit-asset-sync.md`
