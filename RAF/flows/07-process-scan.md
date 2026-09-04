# Process Scan Flow

**On-demand process inventory scanning and drill-down detail retrieval.**

---

## Overview

The process scan follows the existing command-dispatch pattern (same as heartbeat, storage scan, etc.). It is triggered on-demand when a user opens the Processes tab in the dashboard — not on a background schedule.

---

## Flow: List Scan

```
Dashboard                  Server                    Agent
    │                        │                         │
    │ POST /processes/scan   │                         │
    │───────────────────────>│                         │
    │                        │                         │
    │                        │ dedup check:            │
    │                        │ GetPendingCommands()    │
    │                        │ scan_processes pending? │
    │                        │                         │
    │                        │ create signed command:  │
    │                        │ CommandTypeScanProcesses│
    │                        │ SignCommand()           │
    │                        │                         │
    │  200 {command_id}      │                         │
    │<───────────────────────│                         │
    │                        │                         │
    │                        │  GET /commands (poll)    │
    │                        │<────────────────────────│
    │                        │                         │
    │                        │  200 [scan_processes]   │
    │                        │────────────────────────>│
    │                        │                         │
    │                        │                         │ GetFullProcessSnapshot()
    │                        │                         │ reads /proc for all PIDs
    │                        │                         │
    │                        │  POST /process-scan     │
    │                        │<────────────────────────│
    │                        │                         │
    │                        │ InsertSnapshot()        │
    │                        │ InsertProcesses()       │
    │                        │ InsertRelatedData()     │
    │                        │ CleanupOldSnapshots(10) │
    │                        │                         │
    │                        │  200 OK                 │
    │                        │────────────────────────>│
```

---

## Flow: Drill-Down (Process Detail)

```
Dashboard                  Server                    Agent
    │                        │                         │
    │ GET /processes/:pid    │                         │
    │───────────────────────>│                         │
    │                        │                         │
    │                        │ GetProcessByID()        │
    │                        │ GetProcessRelated()     │
    │                        │                         │
    │  200 {process, related}│                         │
    │<───────────────────────│                         │
```

Note: Drill-down reads from the database (data collected during the list scan). No additional agent command is issued.

---

## Command: `scan_processes`

| Field | Value |
|-------|-------|
| Type | `scan_processes` |
| Parameters | None (agent reads its own /proc) |
| Signed | Yes (Ed25519, same as all commands) |
| Idempotent | Yes (each scan produces a new snapshot) |
| Dedup | Server checks for existing pending command before creating |

---

## Database Schema

### `agent_process_snapshots`
Snapshot header — one per scan.

| Column | Type | Notes |
|--------|------|-------|
| id | UUID | Primary key |
| agent_id | UUID | FK to agents |
| command_id | UUID | FK to agent_commands |
| process_count | INTEGER | Number of processes |
| scanned_at | TIMESTAMPTZ | When the scan ran |
| scan_duration_ms | INTEGER | How long the scan took |

### `agent_processes`
Per-process rows — one per process per snapshot.

| Column | Type | Notes |
|--------|------|-------|
| id | UUID | Primary key |
| snapshot_id | UUID | FK to agent_process_snapshots (CASCADE) |
| agent_id | UUID | Denormalized for query performance |
| pid | INTEGER | Process ID |
| name | TEXT | Process name |
| ... | ... | 25+ fields (see scanners/05-process-scanner.md) |

### `agent_process_related`
Related data — JSONB per relation type per process.

| Column | Type | Notes |
|--------|------|-------|
| id | UUID | Primary key |
| process_id | UUID | FK to agent_processes (CASCADE) |
| relation_type | TEXT | open_file, socket, pipe, environment, memory_map, namespace, listening_port |
| data | JSONB | The related data payload |

**Retention:** `CleanupOldSnapshots(10)` keeps the last 10 snapshots per agent. Cascade deletes processes and related data.

---

## API Endpoints

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| POST | `/api/v1/agents/:id/processes/scan` | Dashboard | Trigger on-demand scan |
| GET | `/api/v1/agents/:id/processes` | Dashboard | Get latest snapshot (filterable, sortable) |
| GET | `/api/v1/agents/:id/processes/:processId` | Dashboard | Get process detail with all related data |
| POST | `/api/v1/agents/:id/process-scan` | Agent | Report scan results |

---

*Added: 2026-06-10*
