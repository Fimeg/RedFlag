# File Mappings

**Complete mapping of architectural concepts to source files.**

---

## Core Files

| Concept | File | Purpose |
|---------|------|---------|
| ETHOS principles | `RAF/core/01-ethos.md` | Core identity and principles |
| Architecture decisions | `RAF/core/02-architecture-decisions.md` | Key architectural choices |
| Server component | `RAF/components/01-server.md` | Server breakdown |
| Trust boundaries | `RAF/security/01-trust-boundaries.md` | Trust boundary matrix |
| Auth stack | `RAF/security/02-authentication-stack.md` | Four-layer auth |
| Machine binding | `RAF/security/04-machine-binding.md` | Hardware verification |
| Signing pipeline | `RAF/verification/01-signing-pipeline.md` | Ed25519 signing |
| Agent verification | `RAF/verification/02-agent-verification.md` | TOFU key caching |
| Key rotation | `RAF/verification/03-key-rotation.md` | Key rotation support |
| Replay protection | `RAF/verification/04-replay-protection.md` | Multi-layer defense |
| Registration flow | `RAF/flows/01-registration.md` | TOFU registration |
| Command execution | `RAF/flows/02-command-execution.md` | Polling and dispatch |
| Agent upgrade | `RAF/flows/03-agent-upgrade.md` | Self-upgrade flow |
| Heartbeat | `RAF/flows/04-heartbeat.md` | Health monitoring |
| Capability advertisement | `RAF/flows/05-capability-advertisement.md` | Dynamic scanner reporting |
| Windows Updates | `RAF/scanners/01-windows-updates.md` | WUA integration |
| Docker Scanner | `RAF/scanners/02-docker-scanner.md` | Docker image scanning |
| APT Scanner | `RAF/scanners/03-apt-scanner.md` | APT package manager |
| DNF Scanner | `RAF/scanners/04-dnf-scanner.md` | DNF package manager |
| Process Scanner | `RAF/scanners/05-process-scanner.md` | On-demand /proc scanning |
| Process Scan Flow | `RAF/flows/07-process-scan.md` | Process inventory and drill-down |

---

## Source Files

### Server

| File | Purpose |
|------|---------|
| `server/cmd/server/main.go` | Entry point, route registration |
| `server/internal/api/handlers/agents.go` | Agent CRUD, registration, rebind |
| `server/internal/api/handlers/subsystems.go` | Subsystem CRUD, enable/disable with scheduler eviction |
| `server/internal/api/handlers/agent_updates.go` | Update approval, trigger |
| `server/internal/api/handlers/downloads.go` | Binary distribution |
| `server/internal/api/handlers/setup.go` | Setup wizard, key generation |
| `server/internal/api/middleware/auth.go` | JWT validation |
| `server/internal/api/middleware/machine_binding.go` | Hardware verification |
| `server/internal/database/db.go` | DB connection, migrations |
| `server/internal/database/queries/subsystems.go` | Scanner sync queries |
| `server/internal/database/queries/docker.go` | Docker image queries |
| `server/internal/services/signing.go` | Ed25519 signing |
| `server/internal/services/build_orchestrator.go` | Binary signing |
| `server/internal/services/update_nonce.go` | Replay protection (update nonces) |
| `server/internal/services/timeout.go` | Timeout configuration |
| `server/internal/api/handlers/processes.go` | Process scan endpoints (report, list, detail, trigger) |
| `server/internal/database/queries/processes.go` | Process snapshot queries |
| `server/internal/models/process.go` | Process data models |
| `server/internal/database/migrations/055_create_process_tables.up.sql` | Process tables schema |
| `server/internal/scheduler/scheduler.go` | Subsystem job scheduling, priority queue |
| `server/internal/scheduler/queue.go` | Priority queue (heap) for scheduled jobs |

### Agent

| File | Purpose |
|------|---------|
| `agent/internal/agent/loop.go` | Polling loop implementation |
| `agent/internal/system/machine_id.go` | Machine ID generation |
| `agent/internal/registration/service.go` | Agent registration |
| `agent/internal/config/config.go` | Config management |
| `agent/internal/client/client.go` | API client with retry |
| `agent/internal/orchestrator/docker_scanner.go` | Docker scanner + registry client |
| `agent/internal/orchestrator/storage_scanner.go` | Storage/disk scanner |
| `agent/internal/orchestrator/system_scanner.go` | System metrics scanner |
| `agent/internal/orchestrator/orchestrator.go` | Scanner orchestration, circuit breaker integration |
| `agent/internal/orchestrator/command_handler.go` | Receipt/ack tracking |
| `agent/internal/orchestrator/update_handler.go` | Update handler |
| `agent/internal/circuitbreaker/circuitbreaker.go` | Circuit breaker |
| `agent/internal/retry/retry.go` | Exponential backoff |
| `agent/internal/crypto/pubkey.go` | Public key caching (TOFU) |
| `agent/internal/crypto/verification.go` | Signature verification |
| `agent/internal/scanner/apt.go` | APT scanner |
| `agent/internal/scanner/dnf.go` | DNF scanner |
| `agent/pkg/windowsupdate/client.go` | Windows Update client |
| `agent/internal/system/machine_id.go` | Machine ID generation (Linux) |

### Web

| File | Purpose |
|------|---------|
| `web/src/App.tsx` | Main app + routing |
| `web/src/pages/Agents.tsx` | Agent management |
| `web/src/pages/Updates.tsx` | Update management |
| `web/src/pages/Settings.tsx` | Settings pages |
| `web/src/pages/Dashboard.tsx` | Dashboard |
| `web/src/components/AgentHealth.tsx` | Health monitoring |
| `web/src/components/primitives/` | UI primitive library — FilterBar, SearchInput, FilterDropdown, FilterPill, FilterCountButton, SortableTable, StateBadge, CommandCard, CommandStatusBadge, Modal, PageState, Pagination, StatCard, ScreenshotCard, MetricItem, ProcessTable |
| `web/src/hooks/useFilterUrl.ts` | URL-synced filter state |
| `web/src/hooks/useQueryParser.ts` | key:value query string parser |
| `web/src/hooks/useMultimodalFilter.ts` | Composed multimodal filter |
| `web/src/hooks/useDebounce.ts` | Generic debounce |
| `web/src/hooks/useColumnSort.tsx` | Reusable column sort |
| `web/src/hooks/useCommands.ts` | TanStack Query hook |
| `web/src/hooks/useAgents.ts` | Agent data hook |

---

## Database Tables

| Table | Purpose |
|-------|---------|
| `agents` | Agent records |
| `agents_metadata` | Metadata (available_scanners, heartbeat_source, last_heartbeat) |
| `agent_subsystems` | Enabled scanners (idempotent sync) |
| `agent_commands` | Pending commands |
| `update_logs` | Update execution history |
| `system_events` | Operational events (scans, heartbeats) |
| `refresh_tokens` | Refresh token lifecycle |
| `registration_tokens` | One-time registration tokens |
| `server_public_keys` | Ed25519 keys (rotation support) |
| `security_settings` | Policy configuration |
| `tracked_software` | Agent ↔ tracked_software bindings |

---

## Footer: Assumptions & Connections

**Assumption:** File mappings are living documents — update when code changes.

**Connection:** Core files (`RAF/core/`) provide the foundational principles for all other sections.

**Connection:** Security files (`RAF/security/`) define trust boundaries that all flows must respect.

**Connection:** Verification files (`RAF/verification/`) implement the cryptographic guarantees.

**Connection:** Flow files (`RAF/flows/`) show how components interact end-to-end.

**Connection:** Scanner files (`RAF/scanners/`) document platform-specific integrations.

---

*Last reviewed: 2026-05-26*
