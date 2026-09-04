# Security Model

The software that patches your fleet runs as root on every box. XZ Utils came through a build pipeline. SolarWinds came through an update. The update manager is part of your attack surface — most homelab tooling ignores that. RedFlag treats it as the attack surface it is.

This document is the operator-facing trust model. The architecture behind it — why the design landed where it did, and the pitfalls we think are still out there — is published in the RedFlag Architecture Framework (RAF).

---

## Command Signing

Every command the server issues is Ed25519-signed. Agents verify the signature, check the nonce, validate the timestamp, and reject anything they've seen before. The signing key never leaves your server. You can read the security model in the code, not in marketing copy.

Every command includes a signed nonce with a 10-minute validity window. The agent tracks executed nonces and rejects replays, including from an attacker who intercepted a valid command.

Agent-server communication runs over HTTPS. The Ed25519 signing model is a defense-in-depth layer on top of that — commands can't be forged or replayed even if traffic is somehow intercepted or TLS is terminated at a proxy. The signing model doesn't assume the transport is trustworthy. Cert pinning and enforced TLS verification are on the roadmap.

**Command verification is enabled and strict by default. The server cannot lower enforcement remotely.** Verification runs on the agent (`agent/internal/orchestrator/command_handler.go`) and fails closed. Changing the local enforcement posture (`REDFLAG_AGENT_COMMAND_SIGNING_ENABLED`, `REDFLAG_AGENT_COMMAND_ENFORCEMENT_MODE` in `agent/internal/config/config.go`) requires administrative access to the host itself. The server settings surface exposes only the bounded stale-key tolerance: signing availability follows the provisioned Ed25519 service, and the host owns its verification posture.

---

## Machine Binding

Agents register with a one-time token plus a hardware fingerprint. The server stores the fingerprint; future check-ins that don't match the registered machine are rejected. A stolen `config.json` doesn't work on a different machine.

**Machine-bound renewal.** The token-renewal endpoint checks `X-Machine-ID` against the registered host, exactly as command endpoints do. A stolen config cannot mint access tokens from an unregistered machine — a mismatch returns 403 with a logged `machine_id_mismatch` security event. The agent surfaces this as a critical event, not a quiet backoff.

---

## Key Management

On first connect, the agent fetches and caches the server's Ed25519 public key (TOFU). Every subsequent command is verified against it. The `signing_keys` table supports multiple concurrent active keys for zero-downtime rotation: a new key is promoted to primary while the previous key remains active (still verifies commands) until the operator deprecates it through the dashboard. Agents cache keys by `key_id` fingerprint and re-fetch when they see an unknown signer — no coordinated agent restart required. The roster and deprecation controls live at Settings → Security → Key Management.

Setup accepts an operator-supplied signing keypair — bring-your-own-key deployments are first-class.

---

## Refresh-Token Rotation

90-day TTL. Each renewal mints a new refresh token and marks the old one consumed. Replaying a consumed token whose successor was also consumed means theft — the server revokes the entire token family and logs a security event. Agent crash-before-save is covered by accept-previous-once grace: a consumed token whose successor is still unconsumed gets a fresh one, not a revocation.

The failure mode is detection, not silent coexistence: a leaked token is only useful until the legitimate agent next renews, and using a stale one burns the whole family loudly.

---

## The Supply-Chain Gate

For DNF and APT, the agent runs the package-manager dry-run and resolves artifact hashes from that host's signed repository metadata. The top-level artifact hash is mandatory. Successfully resolved dependency hashes are reported too, but an unresolved dependency is currently logged and omitted rather than blocking the whole report. The server checks the reported entries against OSV.dev and can mint an Ed25519-signed capability binding that exact resolved set to one host and operation.

A known vulnerability among the entries checked is a full stop: the operator must override with a documented reason, or the token is never minted. The override waives that vulnerability judgment only; it does not bypass capability validation or local artifact verification where a local artifact is present.

The privileged Rust helper independently validates the token version and validity window, host `agent_id`, pinned-key Ed25519 signature, and replay state. It constructs a fixed package-manager argv plan, invokes no shell, and clears the inherited environment. If a closure entry points to an existing local file, the helper rehashes that file and denies a mismatch; a `source=mirror` entry without a readable matching file also denies. A normal `source=registry` entry with no readable local file is bound into the signed capability but is **not rehashed helper-side** before APT or DNF fetches and installs it.

The Linux invocation currently uses a transient `systemd-run --wait` unit with `ProtectSystem=no`. It does not set a private network namespace or otherwise enforce network isolation. Complete transitive hash resolution, local custody and re-verification of every installed artifact, then a network-isolated helper are the intended boundary and remain unfinished.

**Current boundary, honestly:** the capability-token gate covers dnf and apt today. Docker, winget, and Windows Update still execute through the signed-command path without the helper — gating them is designed but not yet built. The gaps are documented in the RAF, not hidden.

---

## Our Own Dependencies

RedFlag holds itself to the standard it enforces on the fleet. Every push runs
dependency vulnerability scanning in CI: `govulncheck` (reachability-based) on the Go
server and agent, `npm audit` on the web tree, `cargo audit` on the Rust helper. A
reachable vulnerability that isn't explicitly accepted fails the build — the same
fail-closed posture the binary takes at runtime.

Some findings have no fix to take. RedFlag links the Docker engine library as a
*client* (for container scanning) and inherits daemon-side Moby advisories that carry
no patched version. We don't bury those: each one is a documented entry in a
machine-readable exception register, every entry naming the advisory and the reason
it's accepted. That register is the source of truth — it's surfaced **in the app**, so
RedFlag's own residual exposure is visible the same way fleet exposure is, and can't
quietly rot in a doc nobody re-reads. When an upstream fix ships, the dependency is
bumped and the entry is removed; CI warns on an exception that no longer applies.

The build substrate itself is recorded on every run (toolchain and engine versions) so
"what built this" is never a mystery. Enforcing a minimum-patched floor on that
substrate is the next layer.

### Accepted Dependency Exceptions

The following vulnerabilities are known, accepted, and documented in the
machine-readable `.govulncheck-allow` register. All are daemon-side Docker/Moby
advisories that do not affect RedFlag because it uses the Docker client only for
`Ping`, `SecretList`, and container scanning — never for the daemon's plugin
privilege or AuthZ paths.

| GO ID | Summary | Rationale |
|-------|---------|-----------|
| GO-2026-4883 | Moby off-by-one in plugin privilege validation | Daemon-side; client-only usage |
| GO-2026-4887 | Moby AuthZ plugin bypass via oversized request bodies | Daemon-side; client-only usage |

None of these have published fixes. When upstream patches ship, the dependency
will be bumped and the entries removed.

---

## Visibility

**Security Health** is surfaced as a dashboard panel on each agent — signing status, nonce protection, machine binding violations, command validation — so the posture is visible without digging through logs. All operations are logged with full context, sanitized against log injection (ANSI stripping, control character replacement, field truncation) with the content preserved.

---

## Reporting a Vulnerability

Found something? I want to know, and you won't get lawyered at for looking.

- **Email:** casey@samaritansolutions.net — use "RedFlag Security" in the subject
- Please include reproduction steps and the version (`v*` tag or commit)
- No live deployment exists outside the dev environment yet, so there's no embargo theater — but a heads-up before public disclosure is appreciated so a fix can land first

Good-faith research against your own RedFlag deployment is explicitly welcome. That's what self-hosted means.
