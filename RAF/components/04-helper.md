# Helper Component

**A privileged, short-lived Rust verifier, local minter, and executor for capability-gated mutation.**

---

## Doctrine

The helper (`helper/src/main.rs`) is the only RedFlag path allowed to mutate packages on gated ecosystems. It reads trust inputs from root-owned pinned files and performs exactly one operation per invocation: the operation described by a valid capability token or `MutationEnvelope`. It accepts no shell text and inherits no environment. Everything else is a typed denial.

The current Linux transient unit is **not network-isolated**. APT and DNF can reach their configured registries while the helper runs. Networkless execution remains the design target after RedFlag can stage and re-verify every required artifact locally.

Deny-by-default is the architecture, not a configuration: every failure path returns a `Denial` with a distinct exit code and a `log_security` entry. There is no flag that weakens verification. See [security/05-supply-chain-gate](../security/05-supply-chain-gate.md) for the token contract this enforces.

---

## Invocation

On Linux the agent invokes it through `sudo systemd-run --wait --property=ProtectSystem=no`, passing fixed exchange-file paths. This avoids fd passing through dbus while escaping the agent service's own `ProtectSystem=strict` mount sandbox. No `PrivateNetwork` or equivalent property is set. The agent holds zero direct install sudo; its privileged route is an exact helper invocation. Fleet installers grant only the live capability-token execution shape. Standalone provisioning adds local mint and pacman-envelope command shapes and must not coexist with fleet credentials. See [components/02-agent](02-agent.md).

### Windows Invocation (SEC-030, decided 2026-07-01)

Windows has no `systemd-run` equivalent for spinning up an ad-hoc transient privileged unit, so the helper is invoked through a **Scheduled Task, configured to run once as SYSTEM**. The agent's own service account has no standing right to mutate anything — it only holds a delegated "AllowedToRun" ACE on this one task definition (`schtasks /run /tn RedFlagHelper`), the direct Windows analogue of the Linux sudoers line that grants exactly one `systemd-run` invocation and nothing else.

Two elevation-model alternatives were considered and rejected:
- **A persistent, always-running elevated service watching a staging path.** Rejected outright — a standing elevated process is strictly more attack surface than what it replaces, failing "no standing daemon with broad rights" on its face.
- **A Windows Service the agent starts/stops per-operation.** Viable in principle (mirrors `systemd-run`'s transient-unit shape more closely) but means authoring and auditing a custom SCM service dispatcher/control handler in Rust — real new lifecycle code in a security-critical component. Scheduled Task reuses a well-understood, heavily-audited OS primitive instead of building one.

The task definition itself is provisioned **at agent-install time** by the (already-elevated) install script, the same moment Linux drops its sudoers entry — not self-provisioned by the agent on first gated operation, which would just relocate the "who grants the first elevation" problem rather than solve it.

The ACL lockdown described here is the v1 cut, not the final word — Casey's call ("that'll do for now"). Revisit if a real gap in the ACE-delegation model surfaces (see `docs/tasks/SEC-030-windows-privileged-mutation-helper.md` Open Questions for what's still unresolved: WUA's COM-driven install path, rollback ownership parity with Linux's `.bak` handling).

---

## The Verification Pipeline

`run()` executes, in order — any failure stops the operation:

1. **Token shape and time** — reject an unsupported token version, a not-yet-valid token, or an expired token.
2. **Host binding** — compare the token's `agent_id` with an independently read local identity.
3. **Trust-input validation and keyring load (SEC-021)** — root-owned, non-symlinked, non-writable trust paths; pinned Ed25519 public keys from `/etc/redflag/trusted-keys`.
4. **Closure hash and signature** — recompute the canonical closure hash and verify the signed message against the selected pinned key. Go and Rust tests pin the byte contract.
5. **Local artifact hashes** — `verify_artifacts()` rehashes entries that name a local file. A mirror entry must name a readable matching file. A normal registry entry with no local file remains signed but is not rehashed here.
6. **Fixed plan** — `build_plan()` maps the signed package type and operation to fixed package-manager argv. APT/DNF insert POSIX `--` before package values; unsupported pairs deny.
7. **Replay record** — `replay_check_and_record()` records the token ID before execution; a token runs at most once even across a crash.
8. **Execute** — `execute_plan()` invokes the fixed argv directly, without a shell, after `env_clear()` and a fixed `PATH`.
9. **Receipt** — the helper writes a structured `PolicyResult`; the agent reports it and the server reconciles lifecycle state.

---

## Binary Self-Update Path

Agent, helper, and desktop binaries update through the same gate as packages: `stage_and_verify_binary` (hash check before anything moves) → `atomic_replace_binary` (rename, never write-in-place; failed swap leaves `<binary>.bak`). During agent upgrades, `reconcile_agent_unit_dropin()` heals fleet systemd units to the current template — this is how pre-`AmbientCapabilities` units get fixed without manual fleet surgery ([deployment/01-docker-stack](../deployment/01-docker-stack.md)).

---

## Mutation Envelopes and Pacman

`verify-envelope` remains inspection-only: it verifies shape, target, trust key,
signature, time, lifetime ceiling, and backend payload without consuming replay state or
executing.

`execute-envelope` is live for the pacman backend. Before the local standalone authority
signs, `mint-envelope` requires every action to carry an exact official-repository
archive and detached signature. The helper copies those files into root custody and
checks:

1. archive and signature SHA-256;
2. package name and version read from the archive with `pacman -Qp`;
3. the detached package signature with `pacman-key --verify`; and
4. forward-only movement against the installed version with `vercmp`.

The signed backend payload marks exactly one requested root. An `upgrade` must move that
root strictly forward; an `install` must introduce an absent root. Dependencies may already
be satisfied, but none may move backward.

Execute mode verifies the signed envelope, replay-claims the authorization, repeats the
custody and package checks, and invokes one fixed `pacman -U --noconfirm -- ...` plan. The
resulting `MutationReceipt` joins authorization, decision,
exit code, and the number of fully verified actions; exit zero cannot mean pacman silently
skipped an already-satisfied transaction.

Fleet pacman envelope delivery is not implemented yet. The executor exists; the Server
does not yet mint and deliver this envelope to a fleet Agent.

## Standalone Mint Mode

The helper carries two local-authority protocols: legacy `MintRequest` / `MintedToken`
for APT and DNF, and `EnvelopeMintRequest` / `MutationEnvelope` for pacman. Both load a
root-owned, owner-and-mode-validated Ed25519 key with no-follow semantics. Exchange-file
reads walk fixed directories with `openat`; writes are create-new and no-follow. Request,
response, envelope, and receipt filenames must join on the same UUID.

The helper validates evidence shape and freshness, but it does not independently re-run
OSV or attest the human operator. Root-owned key material therefore constrains the
privileged protocol without reproducing fleet authority against a compromised Agent.
Design of record: [security/06-standalone-authority](../security/06-standalone-authority.md).

---

## Why a Separate Binary, Why Rust

- **Privilege separation:** the long-running, network-facing agent stays unprivileged; the privileged thing is short-lived and single-purpose.
- **Narrow execution surface:** token fields select from fixed argv templates. They are not interpreted as shell input, and the child gets a cleared environment.
- **Isolation still to land:** registry-backed APT/DNF operations currently retain network access. The intended end state stages every authorized artifact locally, verifies it, and runs the helper without a network namespace.
- **Small audit surface:** one file, explicit pipeline, typed denials. The binary is meant to be read.

---

*Last reviewed: 2026-09-01*
