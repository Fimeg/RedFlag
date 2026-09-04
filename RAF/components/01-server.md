# Server Component

**Central Go service that handles API, database, command signing, and binary distribution.**

---

## Package Structure

```
server/
├── cmd/server/
│   └── main.go                 # Entry point, route registration, service initialization
├── internal/
│   ├── api/
│   │   ├── handlers/           # HTTP handlers (50+ files)
│   │   │   ├── agents.go       # Agent CRUD, commands
│   │   │   ├── auth.go         # JWT management
│   │   │   ├── agent_updates.go # Update approval
│   │   │   ├── docker.go       # Docker integration
│   │   │   ├── downloads.go     # Binary distribution
│   │   │   └── ...
│   │   └── middleware/         # Authentication & authorization
│   │       ├── auth.go         # JWT validation
│   │       ├── machine_binding.go # Hardware verification
│   │       ├── rate_limits.go   # Throttling
│   │       └── require_admin.go # Admin checks
│   ├── database/
│   │   ├── db.go              # Connection + migration runner
│   │   ├── migrations/         # Numbered SQL migrations (001–058)
│   │   └── queries/            # SQL queries (sqlx)
│   ├── models/                 # Go structs for all entities
│   ├── scheduler/              # Background job scheduling
│   └── services/               # Business logic
│       ├── signing.go          # Ed25519 operations
│       ├── build_orchestrator.go # Binary signing
│       └── update_nonce.go     # Update nonces for agent verification
└── internal/version/           # Build-time version injection
```

---

## Core Responsibilities

### 1. HTTP API

**Routes organized by trust boundary:**

| Trust Boundary | Group | Middleware | Example Routes |
|----------------|-------|------------|----------------|
| Public | `public` | None | `/api/v1/install/*`, `/api/v1/downloads/*`, `/api/v1/agents/register` |
| Agent | `agent-auth` | `AuthMiddleware + MachineBindingMiddleware` | `/api/v1/agents/:id/commands`, `/api/v1/agents/:id/reports` |
| Web | `web-auth` | `WebAuthMiddleware` | `/api/v1/dashboard/*`, `/api/v1/settings/*`, `/api/v1/agents/:id/processes` |
| Admin | `admin-only` | `WebAuthMiddleware + AdminRoleMiddleware` | `/api/v1/admin/*` |

**Cross-references:**
- [security/01-trust-boundaries](../security/01-trust-boundaries.md) (full trust boundary matrix)
- [security/02-authentication-stack](../security/02-authentication-stack.md) (auth layers)

---

### 2. Database Management

**PostgreSQL connection:**
- Connection pooling: 25 max open, 5 idle
- Queries: sqlx for parameterized queries

**Migrations:** numbered SQL migrations (001–058, with lettered sub-steps 009b, 012b, 023a, 025b), idempotent DDL. Notable:
- 042: `capability_tokens` table (supply chain gate)
- 045: refresh-token rotation lineage (`family_id`, `consumed_at`, `superseded_by`)
- 047: package state machine enforcement (`PackageStatus` CHECK constraint)
- 055: process explorer tables (`agent_process_snapshots`, `agent_processes`, `agent_process_related`)

**Cross-references:**
- [deployment/01-docker-stack](../deployment/01-docker-stack.md) (database configuration)
- [testing/01-test-pyramid](../testing/01-test-pyramid.md) (migration test coverage)

---

### 3. Command Signing

**Ed25519 signing service:**

```go
// services/signing.go
func SignCommand(cmd *Command, privateKey *ed25519.PrivateKey) (*Signature, error) {
    // v3 format: "{agent_id}:{id}:{command_type}:{sha256(params)}:{unix_timestamp}"
    message := fmt.Sprintf("%s:%s:%s:%s:%d",
        cmd.AgentID, cmd.ID, cmd.Type, hash(params), time.Now().Unix())

    signature := ed25519.Sign(*privateKey, []byte(message))

    return &Signature{
        Signature: hex.EncodeToString(signature),
        KeyID:     cmd.KeyID,
        SignedAt:  time.Now(),
    }, nil
}
```

**Cross-references:**
- [verification/01-signing-pipeline](../verification/01-signing-pipeline.md) (signing service)
- [verification/02-agent-verification](../verification/02-agent-verification.md) (verification flow)

---

### 4. Binary Distribution

**Download endpoint:**

```go
// handlers/downloads.go
func DownloadAgent(w http.ResponseWriter, r *http.Request) {
    // 1. Parse version from query
    version := r.URL.Query().Get("version")

    // 2. Fetch signed package from DB
    signedPackage := getSignedPackageByVersion(version)

    if signedPackage == nil {
        // No signature header (v0.2.0.5 issue: version="latest" doesn't match)
        w.Header().Set("X-Content-SHA256", checksum)
        return
    }

    // 3. Serve binary with signature header
    w.Header().Set("X-Content-Signature", signedPackage.Signature)
    w.Header().Set("X-Content-SHA256", signedPackage.Checksum)
    w.Write(binaryData)
}
```

**Cross-references:**
- [flows/03-agent-upgrade](../flows/03-agent-upgrade.md) (agent download flow)
- [security/04-machine-binding](../security/04-machine-binding.md) (download authentication)

**Known issues:**
- BUG-003: `version="latest"` doesn't match any signed package → signature header not set
- FIX: Update install script to use specific version or sign "latest" package

---

### 5. Agent Scheduler

**Background job scheduler:**

- Runs every 10 seconds (check interval)
- Loads enabled subsystems from `agent_subsystems` table at startup only
- Creates scan commands for each subsystem via worker pool
- Supports per-scanner interval from DB row, with fallback to defaults
- **Job eviction:** `DisableSubsystem` removes the job from the in-memory priority queue immediately (ARC-012), so disabling takes effect without restart
- Checks maintenance windows before creating install commands

**Cross-references:**
- [flows/05-capability-advertisement](../flows/05-capability-advertisement.md) (capability advertisement integration)
- [components/02-agent](02-agent.md) (agent polling loop)
- [core/02-architecture-decisions](../core/02-architecture-decisions.md) §12 (scheduler job eviction)

---

## Key Services

| Service | File | Responsibility |
|---------|------|----------------|
| SigningService | `services/signing.go` | Ed25519 key management, command signing, binary signing, capability token signing |
| CapabilityMinter | `services/capability_minter.go` | Build, sign, persist capability tokens; enforces signing-enabled gate |
| BuildOrchestrator | `services/build_orchestrator.go` | Binary retrieval, signing, storage |
| SupplyChainService | `services/supply_chain.go` | OSV batch checks, closure verification, `ClosureCleared` |
| TimeoutService | `services/timeout.go` | Stuck-state recovery for active command/package states |
| Orchestrator | `orchestrator/orchestrator.go` | Lifecycle auto-advance, policy evaluation, workflow coordination |
| NonceService | `services/update_nonce.go` | Replay attack prevention (update nonces) |
| TimezoneService | `services/timezone.go` | Time handling for distributed agents |
| ProcessHandler | `api/handlers/processes.go` | On-demand process scan endpoints (trigger, report, list, detail) |
| RouteAuditor | `routeaudit/auditor.go` | Boot-time audit: every route carries its boundary's auth or the server refuses to start |
| RetentionService | `services/retention.go` | Scheduled pruning of append-only history tables (metrics/events/audit horizons, operator-tunable, 0 = keep forever) |

---

## Cross-References

- **HTTP API** → [security/01-trust-boundaries](../security/01-trust-boundaries.md)
- **Database** → [deployment/01-docker-stack](../deployment/01-docker-stack.md)
- **Command signing** → [verification/01-signing-pipeline](../verification/01-signing-pipeline.md)
- **Binary distribution** → [flows/03-agent-upgrade](../flows/03-agent-upgrade.md)
- **Scheduler** → [flows/05-capability-advertisement](../flows/05-capability-advertisement.md)

---

*Last reviewed: 2026-06-14*