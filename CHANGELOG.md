# Changelog

All notable changes to RedFlag are documented here.

Format: version, date, then grouped by category (Added, Changed, Removed, Fixed, Security).

> **Status: alpha.** Every release before **v0.3.0** ships as a prerelease. Schema, APIs,
> and config can still change without backward-compatibility shims (there are no live
> field clients yet). **v0.3.0 is the planned first stable release.**

---

## v0.2.9.3 (July 2026)

### Added
- Native Windows installer (`RedFlagSetup.msi`) — installs the server binary as a
  Windows service with a forward-only upgrade guard. Built with `wixl` (msitools),
  not the official WiX Toolset .NET CLI, which does not work when the compiler runs
  on Linux. CI (`release.yml`) builds it via `apt-get install msitools && wixl`.
- Native (non-docker) server config loading — the server previously only read OS
  environment variables, which only ever worked because docker-compose's `env_file:`
  injected them. A native install now reads a flat config file
  (`REDFLAG_CONFIG_FILE` or a per-OS default path), additive and inert for existing
  docker deployments.

### Changed
- Registration-token expiry ceiling raised from 7 days to 90 days (matching the
  existing refresh-token precedent); the "Agents & Enrollment" settings page is
  unified into one enroll flow instead of toggled panels, and the server signing-key
  section moved out of the per-token detail pane (it wasn't per-token).
- Release manifest correctly catalogues the server binary as `kind: binary` instead
  of `kind: docker` — every release was already building and packaging it, it just
  wasn't gate-checked or documented as a real install artifact.

### Fixed
- The release build's server compile step (and three `Makefile` targets) passed
  `cmd/server/main.go` as a single-file argument, silently excluding `wire.go`
  (added 2026-06-11) from the build. Broken since `v0.2.9.1`. Now builds the package
  directory (`./cmd/server/`), matching how CI's own build-verification step already
  did it correctly.

### Security / Design
- Windows privileged-mutation helper (SEC-030) elevation model decided: a
  Scheduled Task run once as SYSTEM, ACL-delegated to the agent's service account,
  provisioned at agent-install time — mirrors the Linux `systemd-run` transient-unit
  pattern. Design of record in `RAF/components/04-helper.md`. Implementation not
  started.

## v0.2.9.1 (June 2026)

### Added
- Arch Linux (pacman) update scanner wired into the agent loop. The scanner
  (`checkupdates` from pacman-contrib) was implemented but never registered —
  now resolves subsystem config, circuit breaker, and orchestrator registration
  matching the apt/dnf pattern. Server-side support (scheduler, subsystem
  defaults, repology aliases, capability gate) was already present.

### Known limitations
- pacman updates skip OSV.dev supply-chain checks — Arch Linux is not yet a
  supported OSV ecosystem. Tracked as GATE-006 in `supply_chain.go`. Install
  safety relies on the capability gate + hash verification until OSV adds Arch.

## v0.2.9.0 (June 2026)

### Added
- Desktop tray ships for Windows — cross-compiled via cargo-xwin, installed by
  the Windows installer with per-user autostart (registry Run key).
- Standalone tray actions: `trigger_scan` and `approve_update` Tauri invoke
  commands wired to the local API (`/v1/actions/trigger-scan`,
  `/v1/actions/approve-update`).
- Unified settings page (`Agents & Enrollment`) replaces separate Token and
  Agent Management pages.

### Changed
- Desktop tray no longer child-spawned by the agent service on Linux — relies
  solely on XDG autostart, killing the double-launch.
- Self-update staging paths are platform-aware (`constants/paths.go`) instead
  of hardcoded `/var/lib/redflag/...`.
- Consumer helper invocation platform-gated: `sudo systemd-run` on Linux,
  direct child process elsewhere.
- Desktop server route expanded to `/desktop/:platform/:arch` for multi-OS
  binary serving.
- Windows tray reaches the server image by fetching the signed exe from the
  newest release (hash-verified against the manifest), not by cross-compiling
  Tauri-for-Windows in every from-source build — that drags in the ~9GB MSVC
  sysroot. CI builds it once; from-source servers download the verified
  artifact. Absent/offline degrades to no tray (optional component).
- `signalDesktopRestart` works on Windows (`taskkill /F /IM`) instead of
  no-opping.

### Fixed
- Desktop tray window_open now correctly tracks visibility: set `false` on
  close-to-tray, set `true` on left-click tray icon.

### Removed
- Dead `NewEnforcer` (nil-logger wrapper) in kernel package — refactor
  droppage from the LoopContext migration.
- Dead `NewMigrationExecutorWithEvents` — the log-only constructor is the
  intended one for early-boot migration.
- Orphaned service methods in `service/windows.go` (renewTokenIfNeeded,
  reportSystemInfo, reportLogWithAck, getConfigPath).

## v0.2.8.4 (June 2026)

### Security
- Supply chain: RedFlag now gates its own dependencies and ships the verdict signed —
  self-attestation posture covering dep-scan, build provenance, and an install guard.
- Crypto: forward-only key-path ceiling; the privileged helper compares artifact hashes
  in constant time.
- `/admin` route group latched behind `RequireAdmin` (SEC-026) — inert under the current
  single-admin model, live the moment RBAC lands.

### Added
- Desktop tray updates route through the privileged helper like every other install path.
- RedFlag self-tracking: startup seeds a default upstream row for
  `codeberg.org/Fimeg/RedFlag`, tracks prereleases during alpha, and surfaces a
  dashboard update banner with the operator-run rebuild command.

### Changed
- Web: interactive/brand surface recolored to steel blue, split cleanly from `danger` so
  red reads as error again (the red scale is unchanged under `danger`).
- Web: client errors log at the boundary (axios interceptor, ErrorBoundary, global
  handlers) instead of through a toast-coupled wrapper.

### Removed
- Premature drift-to-update apply hooks: tracked-software drift remains an
  identification surface until the operator install path is scoped.

### Fixed
- Web: 15 dashboard correctness/UX defects from the UI/UX audit — Docker filter cards that
  matched nothing, duplicated retry/cancel hooks, WebSocket reconnect leaks, notification
  dedup, and non-navigable notifications among them.
- OSV resilience and capability-token serialization hardening.

---

## v0.2.8.2 (June 2026)

### Security
- Agent-facing URLs (install scripts, registration responses, fleet-join) no longer
  trust the request Host header. The operator-configured `REDFLAG_PUBLIC_URL` wins;
  the Host header is only a logged fallback for unconfigured installs.
- Setup wizard reads the bootstrap database password from its environment instead
  of a hardcoded literal (shipped default kept as fallback).

### Fixed
- Malformed agent-supplied metadata could panic handler goroutines (rapid-polling
  fields, buffered event metadata, timeout params) — all assertions now guarded.
- Scanner-timeout admin endpoints panicked on every call: `user_id` was asserted
  as a UUID but the auth middleware stores a string.
- Route auditor now sees the auth middleware instances (AUDIT-002); retention
  sweep wired onto the background runner (RETAIN-001 wiring).
- Offline-agent and refresh-token cleanup tickers moved onto the managed task
  runner (shutdown, panic isolation, /health/tasks visibility); upstream syncer
  and reconciler are now stopped on shutdown; `Stop()` is idempotent on all three.
- Manual sync-now reports real failures instead of unconditional success; an OSV
  vulnerability parse failure is logged and still fails closed.

### Removed
- ~760 lines of never-wired lifecycle/build service scaffolding
  (`AgentLifecycleService`, `ConfigService`, `BuildService`, `ArtifactService`,
  `AgentBuildHandler`).

## v0.2.8.1 (June 2026)

### Added
- Retention sweep (RETAIN-001): scheduled pruning of aged rows from append-only
  history tables. Three operator-tunable horizons (metrics/events/audit), 0 = keep
  forever.
- Web: shared FilterBar with URL-synced filter state on the Agents and History
  pages; vitest + jsdom test infrastructure.

### Security
- Fleet-join TOTP seed stored as AES-256-GCM ciphertext at rest instead of a
  SHA-256 hash (migration 058) — hash-only storage could never verify a time code
  without treating the seed as a second cleartext shared secret.

### Fixed
- Process scan commands were created without a Source and hit a database check
  constraint every time (thanks QiTechCo).
- 12-finding code-review fan-out across server, agent, and web: URL filter sync
  race, NULL handling for absent machine IDs at registration, config upgrade
  merging for nested keys, route-audit recorder replacing unsafe reflection.

## v0.2.8.0 (June 2026)

### Added
- Process explorer: on-demand `/proc` scanning with osquery-parity detail (command line,
  cwd, environment size, open sockets with inode correlation, capabilities, namespaces),
  plus a dedicated settings page with lazy route loading.
- Inventory scanner interface with a Docker inventory path; inventory and security-event
  report handlers on the server.
- Self-update capability tokens for the agent, helper, and desktop binaries — binary
  self-upgrade now flows through the same Ed25519-signed, hash-pinned token gate as
  package installs.
- Setup accepts an operator-supplied signing keypair (with validation) instead of only
  generating one — supports bring-your-own-key deployments.
- TeeLogger wired through the agent loop, migration executors, and validator: structured
  dual-output logging (local + server event stream) on previously local-only paths.
- CI cross-compilation matrix: linux-arm64, windows-amd64, darwin-arm64. Helper skipped
  on Windows (Unix-only APIs), darwin via cargo-zigbuild, aarch64 linker pinned through
  `helper/.cargo/config.toml`.

### Fixed
- `rpmEVRAhead` returned true for equal versions when only an explicit epoch-0 prefix
  differed (`0:2.0-1` vs `2.0-1`) — packages already at target were perpetually flagged
  upgradeable on every DNF check.
- Desktop self-update burned its replay token before install, so a transient failure
  (disk full, backup error) permanently blocked further desktop updates. Replay check
  now runs before install; the token is recorded as consumed only after success.
- Orchestrator constructor could carry a nil logger on the Windows service scan path;
  it now defaults to a log-only TeeLogger.
- `/proc/stat` field index bug and ProcessCaps data-collection limits; process scan
  dedup; NaN guards and safe parsing in the process explorer UI.
- Security event insert path corrected; `GetAgentByID` signature mismatch.

---

## v0.2.7.1 (June 2026)

### Added
- CI/CD pipeline via Gitea Actions: `ci.yml` (go vet, go test -race, cargo test + clippy,
  web build, AI-attribution check, action-pins check) and `release.yml` (version gate,
  binary self-report verification, Docker image verification, Gitea-native release API).
- Release gate enforces tag == versions.go == docker-compose == Cargo.toml, CHANGELOG entry
  exists, forward-only tag ordering, and tag ancestry on `public`.
- Guided release script (`scripts/release.sh`): interactive, verifies every assumption
  before tagging or pushing.
- Supply-chain consumer and OSV coverage: 10 new tests for `agent/internal/supplychain/`.

### Fixed
- Screenshot capability now granted via `AmbientCapabilities=CAP_SYS_PTRACE` in the
  service template — survives binary self-upgrade. File-cap restoration alone was
  insufficient for units that predated the template line.
- Helper self-upgrade reconciles the unit drop-in (`10-capabilities.conf`) before
  restarting the agent, so fleet units that only ever self-upgrade still get the
  capability grant. Never adds `CapabilityBoundingSet` — that strips setuid caps from
  sudo inside the unit and kills package discovery.
- Tray socket access chain (`/var/lib/redflag` → `agent` → `localapi`) now enforced
  on upgrades, not just fresh installs.
- Web UI is built and staged into `server/internal/webui/dist` before the server compile
  in the release pipeline — previous tarballs shipped a server with an empty embed.
- CI `typescript-check` job replaced with `web-build` (full `npm run build`), so bundle
  failures surface in CI, not at release time.

### Removed
- Dead `version-consistency` CI job (could never trigger on branch-only push events).
- `scripts/polkit/` and `scripts/redflag-screenshot.sh` (abandoned pkexec approach).
- `scripts/build-secure-agent.sh` and its Makefile target (bare `go build`, no version
  injection, misleading name).

---

## v0.2.6.6 (June 2026)

### Fixed
- Windows process logging now writes `agent.log` before the console stream, so
  service-mode runs still produce file diagnostics when `stderr` is unavailable
  or invalid under Windows Service Manager.
- Agent startup logs a `process_logger_initialized` marker after the durable log
  sink is configured.
- Windows CPU telemetry fallback now parses PowerShell CIM JSON structurally,
  restoring core/thread counts on hosts where `wmic` is unavailable.

---

## v0.2.6.5 (June 2026)

### Fixed
- Windows agent startup now wires the standard logger to
  `C:\ProgramData\RedFlag\logs\agent.log`, so service-mode diagnostics are
  available without relying on Event Viewer rendering.

---

## v0.2.6.4 (June 2026)

### Fixed
- Windows one-liner self-authenticates again (`irm | iex` path repaired).
- Install script served as CRLF for Windows compatibility.
- Install script bakes reachable host into URL, not server bind address.
- Agent install URL preserves port for non-localhost hosts.
- Windows installer tolerates missing Ed25519 verifier on cold start.

---

## v0.2.6.2 (June 2026)

### Changed
- OSV scanning moved from approval to detection — `enqueueOSVChecks` runs on the
  scan-report path, writes results to metadata. Approval reads persisted verdict
  instead of re-scanning inline.
- Soak gate (`GATE-005`) promoted to a real policy: `supply_chain.soak_window_days`
  and `soak_enforcement` resolve env → config → DB → default. Was env-only.
- Age gate (`package_age.go`) wired to DB config via `GetSupplyChainGateConfig`.
  Added opt-in `block_unknown_age` (default false = sovereignty).

### Removed
- Dead soak override scaffolding: dropped `soak_window_hours_override` column,
  `version_soak_overrides` table, and `SoakWindowHoursOverride` model field.
  Migration 053.

---

## v0.2.6.1 (June 2026)

### Added
- RECONCILE-001: scan-set closure (close-by-absence) reconciler. Packages that
  vanish from a successful scan are closed as `not_applicable`, fixing out-of-band
  false positives.

---

## v0.2.6.0 (June 2026)

### Added
- FEAT-002: metadata pipeline — agent → server → metadata transport for upstream
  intelligence, CVE details, and package provenance.
- GATE-005: version soak-gating configuration.
- BRIDGE-001: auto-discovery bridge (Repology, container registry, exact match).
- Docker enrichment pipeline: container image detail, update history pagination.
- Filter/search primitives — composable UI components and hooks.

### Fixed
- SPA nav hygiene + history crosslinks.
- Code review batch 1 + dead store setting cleanup.

---

## v0.2.5.2 (June 2026)

### Added
- SETTINGS-001: reversible token encryption + one-liner restore.

### Fixed
- Heartbeat auto-queue treats duplicate-pending as benign (ETHOS #4 — idempotency).

---

## v0.2.5.1 (June 2026)

### Added
- Lifecycle history section on update detail page: shows `update_version_history`
  entries with status badges, version transitions, failure reasons, timestamps.
- Failed state recovery: `failed` is no longer terminal — transitions to `pending`
  (reopen), `installed` (resolve), or `ignored`. `IsTerminal()` means scan-stable,
  not transition-locked.
- Reopen/resolve endpoints: `POST /updates/:id/reopen` and `POST /updates/:id/resolve`.
  Replaced broken `RetryUpdate` which was structurally blind to capability-token installs.
- Failure metadata on live row: `failure_reason` and `failed_by` stamped on transition
  into failed state.

### Changed
- History page renamed to Fleet Activity, uses `GetFleetActivity` endpoint.

### Fixed
- History 500 error on load.
- Dead unified history substrate removed.

---

## v0.2.3.5 (June 2026)

### Added
- Unified agent + helper upgrade: closure carries both binaries, helper installs
  agent on verify.
- `update_logs.result` extended with `started`/`partial`/`running` states — fixes
  agent-report badge semantics.

### Security
- Path traversal fixes across file-serving endpoints.
- File permissions tightened (result files 0644 so agent can read back from root-owned helper).
- Sudoers and polkit scope narrowed.
- Staging cleanup.

### Fixed
- Self-update and gated installs work on fresh hosts (first-time registration path).

---

## v0.2.3.1 (June 2026)

### Security
- Supply chain gate hardened: a known vulnerability anywhere in the resolved dependency
  closure is a full stop at approval time. `ApproveUpdate` returns `409` and mints nothing.
  Override requires an explicit operator reason in the request body — waives the vulnerability
  judgment only; signing and hash verification stay non-negotiable. Every override journaled
  as a `supply_chain_override` security event.
- Auto-confirm shares the `ClosureCleared` predicate with manual approval — the two paths
  cannot drift on what counts as a clean closure.

### Fixed
- Ack tracking: acks clear on result-recorded, not command lifecycle status. Eliminates
  34-deep recycling loops on long-running command chains.
- Timeouts, cancels, dropped acks/receipts, and failed actions all land in unified history
  instead of dying on stdout.

---

## v0.2.3.0 (May 2026)

### Changed
- OSV checks switched to batch endpoint (`/v1/querybatch`, 100 per POST) with global
  concurrency cap (4 concurrent batches). 300 packages = 3 HTTP calls instead of 300.
- Auto-confirm frisks the whole dependency closure for CVEs, not just the top-level package.

### Fixed
- DNF dry-run success detection: `--assumeno` exits non-zero even on a clean resolve.
  Gate on `Transaction Summary` presence; reject `Nothing to do` / `No match` / `Error:`
  explicitly instead of trusting exit code.
- Web logout clears zustand persist key alongside tokens, fixing stale JWT survival across
  server reinstalls (JWT_SECRET rotation).

---

## v0.2.2.0 (May 2026)

### Added
- Package state machine enforced across all transitions: typed `PackageStatus`,
  `ValidateTransition` guards, guarded UPDATE with `WHERE status = $current`.
  Migration 047 aligns all existing rows. (LIFECYCLE-001)
- Lifecycle orchestrator foundation: timer-driven auto-advance, stuck-state recovery for
  `checking_dependencies` and `installing`, auto-approval policy support. (LIFECYCLE-003)
- Vulnerability dashboard: per-agent and fleet-wide OSV finding visibility.

---

## v0.2.1.1 (May 2026)

### Added
- Agent self-upgrade via helper: the privileged helper now performs agent binary swaps
  (backup, install, chmod, systemctl restart) under an `agent-self` capability token.
  The agent stages the verified binary; the helper re-verifies the hash before installing.
  No agent sudo for cp/chmod/systemctl.
- OSV.dev supply-chain checks at discovery time: async, deduped per server lifetime.
  Results written to `current_package_state.metadata` for immediate UI visibility.
- OSV.dev startup backfill: unchecked packages get queried on server boot (best-effort).
- apt and dnf added to OSV.dev ecosystem mapping (Debian, AlmaLinux).
- Docker handler uses dedicated `DockerQueries` with proper image/container separation.
- Staging page: LiveOperations renamed to Staging, shows packages awaiting dependency
  review alongside in-flight operations. Loading and error states added.
- Server-side status filter for the Updates package list (HAVING clause on aggregation).
- Docker severity displayed from actual data instead of hardcoded "medium".

### Changed
- apt discovery runs unprivileged: sandbox opts redirect lists/cache/state/log to an
  agent-writable temp dir, matching dnf's model. No apt sudo grants in sudoers.
- Docker commands no longer prefixed with `sudo` (agent uses docker group membership).
- Sudoers template: removed all Docker grants (docker group), all self-update grants
  (helper does it), and apt discovery grants (unprivileged). Agent's only sudo is the
  single `systemd-run --pipe` helper invocation line.
- Polling loop recalculates interval after processing commands, not before (rapid-polling
  takes effect in the same cycle it's enabled).
- `UpdateCurrentStateInTx` exported for single-row writes outside batch transactions.
- Update result values normalized: `updated` -> `success`, `rollback` -> `success`.
- Drift-to-update bridge writes `UpdateEvent` via `UpsertCurrentState` instead of the
  removed `UpsertUpdate`/`UpdatePackage` path.
- `AgentDockerImage` model fields aligned with agent's `DockerReportItem` JSON tags.

### Removed
- `UpdatePackage` model and its `UpsertUpdate`/`ListUpdates` query methods (dead code
  from the pre-state-table era).

### Security
- Agent holds zero sudo for self-upgrade, Docker, and apt discovery. The helper is the
  only privileged path and it verifies capability-token signatures and artifact hashes
  before every operation.

---

## v0.2.1.0 (May 2026)

### Added
- Helper privilege split: `redflag-helper` invoked via `sudo systemd-run --pipe` as its
  own transient service, escaping the agent's `ProtectSystem=strict` sandbox.
- Token-is-the-command: all package-manager operations routed through discovery
  (`DiscoveryRunner`) or mutation (`consumer.go -> systemd-run -> redflag-helper`).
- `EcosystemConfig` registry: adding a new ecosystem means one config entry + discovery
  interface. Mutation is automatic via the token path.
- Hash verification fail-closed: empty expected hash returns error.

### Changed
- `Installer` interface shrunk from 7 methods to 4: `IsAvailable`, `GetPackageType`,
  `DryRun`, `VerifyHash`.
- `apt-get` replaced with `apt` throughout.
- Sudoers narrowed: agent can only run discovery commands + helper invocation.

### Removed
- `SecureCommandExecutor` (replaced by `DiscoveryRunner` + helper).
- Direct mutation methods from `DNFInstaller` and `APTInstaller`.

---

## v0.2.0.7 (May 2026)

### Security
- Refresh-token rotation with accept-previous-once crash-recovery grace.
- Machine-bound renewal: stolen `config.json` replayed from another host -> 403.
- Typed sentinel errors for auth failures in polling loop.
- Revoked refresh tokens and machine-ID mismatches surface as critical events.
- Signing-disabled path removed from orchestrator; unsigned fallback removed.

---

## v0.2.0.6 (May 2026)

### Added
- Agent cold-start trust root: signed release manifest verified before first execution.
- Supply-chain hash registry (Layer 1): expected SHA-256 stored server-side, verified
  by agents before install.
- Drift-to-UpdatePackage bridge: drifted bindings automatically create pending updates.
- Upstream tracking UI with release-source adapters (GitHub, Gitea, GitLab, Bitbucket).
- Attention panel: surfaces offline agents, failed updates, EOL drift, upstream movement.

---

## v0.2.0.0 (May 2026)

### Added
- Maintenance windows for scheduling/gating installs.
- Docker image scanning and update management.
- Multi-agent fleet overview dashboard.
