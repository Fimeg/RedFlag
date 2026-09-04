# Standalone Authority (Local Mode)

Status: implemented for local Agent startup, scanning, APT/DNF capability approval,
and pacman envelope approval. Fleet join remains fail-closed and unfinished.

Companion to [05-supply-chain-gate](05-supply-chain-gate.md). Standalone changes where
authority lives; it does not create a pretend off-host boundary on one machine.

---

## The Boundary That Exists

Fleet mode has a real host boundary:

```
operator → Server authority → Agent consumer → root helper
```

Standalone has only a privilege boundary:

```
Desktop user → local Agent → fixed sudoers invocation → root helper and local key
```

The local Ed25519 private key is root-owned and unreadable by the Agent. That stops the
long-running Agent from copying or directly using key material, and the helper still
constrains every privileged action to a typed protocol. It does **not** make a compromised
Agent independent of the signer: the Agent is intentionally allowed to invoke the fixed
mint command and supplies the gate evidence the helper validates. The helper does not
re-run OSV or cryptographically prove that a human supplied the operator label.

The honest threat table is therefore:

| Property | Fleet | Standalone |
|---|---|---|
| Host-bound signed authorization | yes | yes |
| Replay protection | yes | yes |
| Fixed argv, cleared environment, no shell | yes | yes |
| Pacman archive hash, identity, and detached signature verified by root helper | yes when fleet envelopes land | yes |
| APT/DNF local artifacts rehashed when a closure entry names a path | yes | yes |
| Human authority survives Agent compromise | yes, authority is off-host | **no** |
| Signing key survives managed-host compromise | yes, authority is off-host | **no** |
| Out-of-band mutation by local root prevented | **no, kernel enforcement is not wired** | **no** |
| OSV judgment independently reproduced by helper | no | no |

Standalone still materially narrows mutation. It does not turn same-host signing into
proof that the host was uncompromised.

---

## Provisioned State

`scripts/provision-standalone-authority.sh` runs as root after the base Agent, helper,
`redflag-agent` user, and `redflag-local` group exist. It is idempotent and must never run
on a fleet-enrolled host.

Provisioning creates or verifies:

- a stable UUIDv4 in the Agent config via `redflag-agent --init-standalone`;
- the same UUID in root-owned `/etc/redflag/agent_id` for independent target binding;
- `/etc/redflag/authority_local.key`, root-owned `0600`;
- the public half in the helper's root-owned trusted keyring;
- Agent-owned exchange directories for mint requests, tokens, mutation requests,
  envelopes, and receipts; and
- exact sudoers command shapes for legacy token mint, envelope mint, and envelope
  execution.

`Config.IsStandalone()` is true only when a stable Agent ID exists and registration,
access, and refresh tokens are all absent. Partial fleet material is neither standalone
nor registered and startup refuses it.

The standalone Agent starts the local API and kernel monitor, scans on startup, and then
scans on its bounded local interval without contacting a Server. Pacman is included in
that local scanner set.

---

## APT and DNF Flow

```
Desktop
  → POST /v1/actions/approve-update
  → Agent dry-run and closure hash resolution
  → Agent OSV query over the reported closure
       non-clear verdict requires a recorded reason
  → Agent writes MintRequest
  → root helper mint
       validates host, operation, closure shape, evidence freshness, and reason
       signs a short-lived capability with authority_local.key
  → normal Agent consumer
  → root helper verifies, replay-claims, and executes one fixed APT/DNF plan
  → PolicyResult returns to Desktop
```

Age and soak fields are recorded as `not_applicable`; standalone has no local registry
history for those policies yet. Registry artifacts without local paths remain signed in
the closure but are not rehashed helper-side, and the transient helper unit retains host
network access.

---

## Pacman Flow

Pacman does not use the legacy capability payload:

```
Desktop + required reason for unavailable OSV coverage
  → Agent resolves exact official-repository transaction in a private database/cache
  → Agent hashes every archive and detached signature
  → Agent writes EnvelopeMintRequest + MutationManifest
  → root helper mint-envelope
       secure openat read from the fixed exchange directory
       copies artifacts into root custody
       verifies exactly one requested root, hashes, package identity, repository, detached signatures, and versions
       requires an upgrade root to move strictly forward or an install root to be absent
       signs one host-bound MutationEnvelope
  → Agent writes the envelope into the fixed execution exchange
  → root helper execute-envelope
       verifies signature, time, target, replay, custody, identity, signatures, and versions again
       executes one fixed pacman -U plan without --needed
  → joined MutationReceipt returns to Desktop
```

Exchange files use exact UUID filenames, no-follow directory traversal, create-new
outputs, and filename joins between request and response. Stale unprivileged outputs are
removed before the helper runs; the helper refuses symlinked or out-of-directory paths.

OSV currently has no usable Arch package mapping. The verdict is `unsupported`, never
`clear`, and the operator reason cannot waive any cryptographic or package-identity check.

---

## Audit State

Mint and execution decisions write the helper's local security journal. The local API
does not yet expose that root journal as a dedicated approval-history endpoint, and no
fleet upload exists. Desktop can show the joined result returned by the current request;
durable local browsing remains unfinished.

The Desktop-provided operator name comes from the session environment and is asserted,
not peer-attested. `SO_PEERCRED`, polkit, or another fresh step-up mechanism remains an
open authority improvement.

---

## Fleet Join Is Not Implemented

The helper has an explicit local-key retirement primitive, but there is no complete,
tested transition that retires local authority, destroys its private key, installs the
Server keyring, registers the Agent, and proves the converged end state. Registration
from a standalone config therefore refuses with an error. Operators must not add fleet
credentials beside the local authority by hand.

The required future transition is one-way and idempotent:

1. stop local mutation intake;
2. retire and destroy the local private authority;
3. replace the helper trust set with Server authority keys;
4. register and persist complete fleet credentials;
5. prove that local mint sudoers and key material are absent; and
6. resume as a fleet Agent with no local fallback.

Until that workflow and its matrix tests exist, “join this standalone machine to a
fleet” is a named gap, not a supported lifecycle.

---

## Non-Goals

- No local mint authority beside fleet credentials.
- No doctrinal switch that disables signing, replay, hashes, identity, signatures, or
  forward-only enforcement.
- No package-manager command in Desktop or Agent sudoers.
- No network listener for minting.
- No claim that same-host Ed25519 recreates the fleet trust boundary.

---

## Cross-References

- [05-supply-chain-gate](05-supply-chain-gate.md) — capability and envelope contracts
- [../components/04-helper](../components/04-helper.md) — root validation and execution
- [../components/05-desktop](../components/05-desktop.md) — credential-less request surface
- [../scanners/06-pacman-scanner](../scanners/06-pacman-scanner.md) — discovery and exact transaction resolution

---

*Last reviewed: 2026-09-01*
