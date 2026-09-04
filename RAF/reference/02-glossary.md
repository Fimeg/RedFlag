# Glossary

**The vocabulary of RedFlag, in one place. Terms link to their design-of-record pages.**

---

| Term | Meaning |
|------|---------|
| **Agent** | Stateless Go executor on each managed host. Polls, verifies, executes, reports. Never decides. [components/02-agent](components/02-agent.md) |
| **Capability token** | Ed25519-signed grant describing exactly one operation over the artifact entries it carries. Every carried name/version/hash is signed; the current dnf/apt set can omit a dependency whose hash did not resolve. Minted at approval, executed once. [security/05-supply-chain-gate](security/05-supply-chain-gate.md) |
| **Closure (dependency closure)** | The artifact set reported from a package-manager dry-run. The target is the full named package plus every transitive dependency; today the top-level hash is mandatory while unresolved dependency hashes can be omitted. |
| **Closure hash** | Canonical hash over the closure, embedded in the token's signed message. Computed byte-identically in Go (server) and Rust (helper) — the cross-language contract. |
| **Discovery vs. mutation** | Discovery (scan, dry-run, hash-resolve) runs unprivileged through `DiscoveryRunner`. Mutation happens only through the helper on gated ecosystems. The agent cannot install. |
| **Doctrine / doctrinal** | A guarantee with no configuration knob: signing-required, forward-only versioning, no verification skip path. If it's doctrine, there is nothing to misconfigure. [core/01-ethos](core/01-ethos.md) |
| **Drift detection** | Knowing what *should* be installed vs. what *is*, and bridging the gap into update packages. |
| **ETHOS** | The five principles every change is held to: errors are history, no unauthenticated endpoints, assume failure, idempotency, no marketing fluff in logs. [core/01-ethos](core/01-ethos.md) |
| **Fail-closed** | When a required check fails, the operation doesn't happen: bad token version/time/host/signature/replay state denies, a missing or mismatched mirror artifact denies, and an unknown vulnerability state blocks under configured policy. A normal registry entry without a local path is not currently a required helper-side rehash. |
| **Family revocation** | Refresh tokens form a lineage (`family_id`); replaying a stale token burns the entire family loudly. Theft is detected, not coexisted with. [security/03-refresh-tokens](security/03-refresh-tokens.md) |
| **Forward-only** | No downgrades. Versions move forward; the release gate enforces it; there is no override. |
| **Helper** | The privileged, short-lived Rust executor — the only RedFlag mutation path on gated ecosystems. It uses fixed argv and a cleared environment; its current transient unit is not network-isolated. [components/04-helper](components/04-helper.md) |
| **Legacy command path** | Direct signed-command execution for docker / winget / windows_update — ecosystems the capability gate doesn't cover yet. A documented gap, not a feature. [OVERVIEW](OVERVIEW.md) |
| **Machine binding** | Hardware fingerprint registered at enrollment and checked on every authenticated request, including token renewal. A stolen `config.json` is inert elsewhere. [security/04-machine-binding](security/04-machine-binding.md) |
| **Mutation manifest** | Dormant, backend-neutral description of one resolved state change: target, backend, operation, exact backend payloads, and provenance evidence. Its `target_id` is the RedFlag `agent_id`. Pinned cross-language and verifiable by the helper, but no backend executes through it yet. [security/05-supply-chain-gate](security/05-supply-chain-gate.md) |
| **Mutation receipt** | The response half of the mutation contract: what the executor did with one envelope, carrying the operation/manifest/authorization join for audit. Unsigned by design — the executor is not a second authority. [security/05-supply-chain-gate](security/05-supply-chain-gate.md) |
| **Nonce** | Per-command signed value with a 10-minute window; agents track executed nonces and reject replays. [verification/04-replay-protection](verification/04-replay-protection.md) |
| **OSV** | OSV.dev, the open vulnerability database. Queried for discovered packages and for the resolved entries reported after dry-run; verdicts persist and gate approval. An unresolved dependency omitted from that report is not checked by this path. |
| **RAF** | This document tree — the RedFlag Architecture Framework, the design of record. What the system is, not what's currently on the task list. |
| **Soak gate / age gate** | Time-based supply-chain policies: minimum package age before approval (Shai-Hulud defense) and a version soak window before install. Policies, not doctrine — configurable, with enforcement modes. [security/05-supply-chain-gate](security/05-supply-chain-gate.md) |
| **TOFU** | Trust-on-first-use: the agent caches the server's public key at first connect and verifies everything after against the cached roster, by `key_id`. [verification/03-key-rotation](verification/03-key-rotation.md) |
| **Two execution paths** | Capability gate (dnf, apt — token + helper) and legacy command (everything else, for now). [OVERVIEW](OVERVIEW.md) |

---

*Last reviewed: 2026-08-26*
