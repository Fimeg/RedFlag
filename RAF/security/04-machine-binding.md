# Machine Binding

**Hardware-bound authentication for agent-to-server communication.**

---

## Overview

RedFlag ties each agent to specific hardware via a machine ID fingerprint. This prevents config file theft from being used on unauthorized machines.

---

## Machine ID Generation

**Linux:**
- Uses `machineid` library to compute SHA-256 hash
- Fallbacks: `/sys/class/dmi/id/product_uuid`, `/var/lib/dbus/machine-id`
- Combined with hostname for uniqueness

**Windows:**
- Uses `MachineIdentifier` class from Windows API
- Retrieves system-wide hardware identifier

**macOS:**
- Uses IOKIT framework to read hardware identifiers
- Combines multiple hardware sources for uniqueness

**Implementation:**
- `agent/internal/system/machine_id.go` (uses `machineid` library — Linux dbus/machine-id, macOS IOKIT, Windows registry)

---

## Binding Flow

```
1. Agent computes machine ID at startup
2. Agent includes X-Machine-ID header in requests
3. Server validates against stored machine ID
4. Mismatch → authentication failure
5. Match → request proceeds
```

**Middleware:**
- `server/internal/middleware/machine_binding.go:MachineBindingMiddleware()`

---

## Binding States

| State | Description | Trigger |
|-------|-------------|---------|
| `registered` | Machine ID stored, valid | Registration complete |
| `pending` | Hardware change detected | Machine ID mismatch |
| `unbound` | No machine ID provided | Missing header |
| `revoked` | Admin action | Manual revocation |

---

## Rebinding Endpoint

**Purpose:** Allow legitimate hardware changes.

**Endpoint:** `POST /api/v1/agents/:id/rebind`

**Requirements:**
- Admin authentication (WebAuthMiddleware)
- Valid nonce for rebind operation
- Machine ID update logged to audit trail

**Flow:**
```
1. Admin approves rebind request
2. Server updates machine ID for agent
3. Event logged to history table
4. Agent can resume normal operations
```

**Implementation:**
- Handler: `server/internal/handlers/agents.go:RebindAgentMachineID()`
- Validation: Nonce + admin auth + machine ID update

---

## Security Considerations

### Machine ID Spoofing
- Attacker cannot forge valid X-Machine-ID without hardware access
- Server-side validation prevents spoofed headers
- Binding checked on every authenticated request

### Hardware Changes
- SSD replacement → new machine ID
- Motherboard swap → new machine ID
- Cloud instance restart → same machine ID (persistent storage)

**Mitigation:** Rebind flow for legitimate changes

### Key Compromise
- Stolen agent config + machine ID = unauthorized access
- Mitigation: Machine binding ties config to hardware
- Mitigation: Key rotation limits exposure window

---

## Footer: Assumptions & Connections

**Assumption:** Machine ID is stable for the lifetime of the hardware configuration.

**Connection:** Machine binding (`security/04-machine-binding.md`) implements ETHOS #2 (security is non-negotiable).

**Connection:** Rebinding endpoint (`security/04-machine-binding.md`) complements nonce validation (`verification/04-replay-protection.md`).

**Connection:** Machine binding middleware (`server/internal/middleware/machine_binding.go`) enforces binding on agent routes.

**Connection:** TOFU model (`verification/02-agent-verification.md`) works with machine binding for trust continuity.

---

*Last reviewed: 2026-05-26*
