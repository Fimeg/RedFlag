# Process Scanner

**On-demand /proc filesystem scanning for process inventory and drill-down detail.**

---

## Component Details

| Property | Value |
|----------|-------|
| Method | Direct `/proc` filesystem reads (no subprocess spawns) |
| Platform | Linux only (stub on other platforms) |
| Execution time | ~200ms for 200-process snapshot; ~50ms per drill-down |
| Trigger | On-demand when user opens the Processes tab in the dashboard |
| Data model | 25+ fields per process (osquery parity) + 7 related data types |
| Storage | Dedicated tables: `agent_process_snapshots`, `agent_processes`, `agent_process_related` |
| Retention | Last 10 snapshots per agent (auto-cleanup) |

---

## Architecture

The process scanner follows the existing command-dispatch pattern:

1. **Dashboard** opens Processes tab → `POST /api/v1/agents/:id/processes/scan`
2. **Server** creates a signed `scan_processes` command (with dedup check)
3. **Agent** polls for commands, receives `scan_processes`, calls `system.GetFullProcessSnapshot()`
4. **Agent** reports snapshot to `POST /api/v1/agents/:id/process-scan`
5. **Server** stores snapshot + processes + related data, cleans up old snapshots
6. **Dashboard** reads latest snapshot via `GET /api/v1/agents/:id/processes`

On drill-down (clicking a process row):
1. **Dashboard** requests `GET /api/v1/agents/:id/processes/:processId`
2. **Server** returns process + all related data (open files, sockets, pipes, env, memory map, namespaces, listening ports)

---

## Data Collection

### List Scan (`GetFullProcessSnapshot`)

Reads `/proc/[pid]/stat`, `/proc/[pid]/status`, `/proc/[pid]/exe`, `/proc/[pid]/cmdline`, `/proc/[pid]/cwd`, `/proc/[pid]/cgroup`, `/proc/[pid]/io` for every PID. No related data collected at this stage.

**Fields (25+):** PID, Name, Path, Cmdline, Cwd, State, UID, GID, EUID, EGID, User, Group, TTY, TTYName, CPUSecondsUser, CPUSecondsSystem, CPUPercent, RSSBytes, VMSBytes, MemPercent, Threads, Nice, StartTimeSeconds, ParentPID, ProcessGroupID, ElevationStatus, OnDisk, DiskBytesRead, DiskBytesWritten, Cgroup, Unit, ContainerID, ContainerRuntime, CapabilitiesEffective

### Drill-Down (`GetProcessDetail`)

Adds related data from a single `/proc/[pid]/fd/` walk (consolidated from three separate traversals):

| Data Type | Source | Cap (configurable) |
|-----------|--------|---------------------|
| Open files | `/proc/[pid]/fd/` symlink targets | `max_open_files` (default 2000) |
| Open sockets | `/proc/[pid]/net/tcp`, `tcp6`, `unix` | `max_sockets` (default 500) |
| Open pipes | `/proc/[pid]/fd/` pipe inodes | `max_pipes` (default 500) |
| Environment keys | `/proc/[pid]/environ` (keys only, no values) | `max_env_keys` (default 200) |
| Memory map | `/proc/[pid]/maps` | `max_memory_map` (default 2000) |
| Namespaces | `/proc/[pid]/ns/` symlinks | `max_namespaces` (default 50) |
| Listening ports | Socket inode correlation with `/proc/net/tcp` | `max_listening_ports` (default 100) |
| Capabilities | `CapEff` mask read during the list scan, decoded into names here | — |

### Key Implementation Detail: Ownership Attribution

`/proc/[pid]/cgroup` answers who owns a process, which is the join between the process
list and the service, container, and package inventories. Without it a process list is a
task manager: a name, a number, and no way to ask what put it there.

`parseCgroupOwner` produces four fields from that one file:

| Field | Meaning |
|---|---|
| `cgroup` | the path the attribution was read from, kept so a surprising answer can be checked |
| `unit` | innermost systemd unit — `nginx.service`, `app-firefox-4090.scope` |
| `container_id` | full container ID as the kernel spells it |
| `container_runtime` | `docker`, `podman`, `containerd`, `crio`, `lxc` |

Both cgroup generations are handled. v2 is the single `0::<path>` line. v1 has one line per
controller and the paths can disagree, so the `name=systemd` hierarchy wins when present —
it is the one that carries unit and container scopes.

Container detection covers both cgroup drivers, because the same runtime writes different
paths depending on how it was configured: systemd driver gives `docker-<id>.scope`,
`libpod-<id>.scope`, `cri-containerd-<id>.scope`; cgroupfs driver gives the bare ID under
`/docker/` or `/kubepods/.../pod<uid>/`. LXC carries a name rather than a hash. A scope
whose suffix is not hex of length 12 or 64 is a unit, not a container — `docker-notahash.scope`
attributes as a unit.

**The Docker join is a prefix match, not equality.** `container_id` here is the full ID
from the kernel; the Docker scanner reports `c.ID[:12]` in its inventory
(`agent/internal/orchestrator/docker_scanner.go`). A consumer joining the two compares
prefixes. Truncating the kernel's answer to match one scanner's display width would be
fabricating uniformity, which the mutation protocol forbids for the same reason.

Kernel threads have cgroup `/` and stay unattributed, correctly — they have no unit, no
container, and no package. On a 321-process Arch desktop that is 172 of them, and every
userland process attributes.

### Key Implementation Detail: Effective Capabilities

`CapEff` in `/proc/[pid]/status` is the mask that decides what a process may actually do
to the machine — mount filesystems, load modules, trace another process, rewrite the
network stack. UID alone does not answer that: a non-root process can hold
`CAP_NET_ADMIN`, and a root process in a container usually holds far less than the full
set.

The mask is read during the list scan, because it costs nothing — the status file is
already open. It is expanded into names only on drill-down: fully privileged processes
hold all 41, and 41 strings on every row of a 300-process inventory is payload nobody
reads. `decodeCapabilities` is a pure function over the hex mask, so it is fixture-tested
without a machine.

A bit past `CAP_LAST_CAP` is reported as `CAP_<n>` rather than dropped. A newer kernel
than this table should produce an unfamiliar name, not silence.

Verification against a known constant: Docker's default container mask, `a80425fb`,
decodes to exactly the fourteen capabilities Docker documents itself as granting.

### Key Implementation Detail: Socket Inode Correlation

Listening ports are per-process, not system-wide. The scanner collects socket inodes from `/proc/[pid]/fd/` symlinks (`socket:[12345]`), then matches them against inode numbers in `/proc/net/tcp` and `/proc/net/tcp6`. Only LISTEN state (0A) entries whose inode matches a process socket are included.

### Key Implementation Detail: IPv6 Address Parsing

The kernel stores IPv6 addresses in `/proc/net/tcp6` as 4 little-endian 32-bit words (`%08X%08X%08X%08X`). The parser reverses bytes within each 4-byte group (not across the entire 16-byte address) and uses `%02x` for zero-padded output.

### Key Implementation Detail: `/proc/[pid]/stat` Field Indices

After stripping `pid (comm)`, the `fields` array is 0-indexed from field 3:
- `[0]`=state, `[1]`=ppid, `[2]`=pgrp, `[3]`=session, `[4]`=tty_nr
- `[11]`=utime, `[12]`=stime, `[16]`=nice, `[19]`=starttime

---

## Configurable Caps

Data collection limits are server-controlled via `ProcessExplorerConfig` (stored in security settings under `operational` category). Delivered to agents on check-in. Set to 0 for no cap.

| Setting | Default | Purpose |
|---------|---------|---------|
| `process_explorer_max_open_files` | 2000 | Open file descriptors per process |
| `process_explorer_max_sockets` | 500 | Open sockets per process |
| `process_explorer_max_pipes` | 500 | Open pipes per process |
| `process_explorer_max_memory_map` | 2000 | Memory map entries per process |
| `process_explorer_max_namespaces` | 50 | Namespace entries per process |
| `process_explorer_max_env_keys` | 200 | Environment variable keys per process |
| `process_explorer_max_listening_ports` | 100 | TCP listening ports per process |

**UI:** Settings → Process Explorer (`/settings/process-explorer`)

---

## Implementation Files

### Agent

| File | Purpose |
|------|---------|
| `agent/internal/system/process_detail.go` | Types: `FullProcess`, `ProcessOpenFile`, `ProcessOpenSocket`, `ProcessOpenPipe`, `ProcessMemoryMap`, `ProcessNamespace`, `ProcessListeningPort`, `ProcessCaps`, `FullProcessSnapshot` |
| `agent/internal/system/process_owner.go` | `ProcessOwner` and `parseCgroupOwner` — cgroup attribution, platform-independent and fixture-tested |
| `agent/internal/system/process_capabilities.go` | `decodeCapabilities` and the capability-bit table — platform-independent and fixture-tested |
| `agent/internal/system/process_detail_linux.go` | Linux `/proc` reader: `getFullProcessSnapshot()`, `getProcessDetail()`, `walkProcFD()`, `readListeningPorts()`, `parseHexAddr()` |
| `agent/internal/system/process_detail_other.go` | Stub for non-Linux platforms |
| `agent/internal/handlers/processes.go` | `HandleScanProcesses` — command handler |
| `agent/internal/client/client.go` | `ReportProcessScan` — reports to server |

### Server

| File | Purpose |
|------|---------|
| `server/internal/database/migrations/055_create_process_tables.up.sql` | Schema: `agent_process_snapshots`, `agent_processes`, `agent_process_related` |
| `server/internal/models/process.go` | Server-side models |
| `server/internal/database/queries/processes.go` | `ProcessQueries` — insert, query, cleanup |
| `server/internal/api/handlers/processes.go` | `ProcessHandler` — 4 endpoints |
| `server/internal/models/command.go` | `CommandTypeScanProcesses` constant |

### Web

| File | Purpose |
|------|---------|
| `web/src/types/process.ts` | TypeScript interfaces |
| `web/src/hooks/useProcesses.ts` | React Query hooks |
| `web/src/components/ProcessesTab.tsx` | Main tab with sortable table |
| `web/src/components/ProcessDetailModal.tsx` | 6-tab modal (Overview, Network, Files, Environment, Memory, Namespaces) |
| `web/src/hooks/useProcessExplorer.ts` | Settings hooks |
| `web/src/pages/settings/ProcessExplorer.tsx` | Settings UI |

---

## Security Considerations

- **Environment variable values are never transmitted.** Only key names are collected. Env vars may contain secrets (API keys, database passwords).
- **No subprocess spawns.** All data comes from direct `/proc` reads — no `ps`, `lsof`, or similar commands.
- **On-demand only.** The scan command is only issued when a user opens the Processes tab. No background broadcasting.
- **Command dedup.** The server checks for existing pending `scan_processes` commands before creating a new one.

---

*Added: 2026-06-10 · ownership attribution and effective capabilities 2026-08-31*
