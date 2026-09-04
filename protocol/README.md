# Mutation manifest wire contract

This directory owns the language-independent bytes shared by RedFlag Server, Agent, and
helper. APT and DNF still use the closure-based capability token. Pacman is the first live
`MutationEnvelope` backend: standalone Desktop intent now crosses Agent resolution, local
root mint, helper verification, execution, and a joined receipt without a second package
manager path in QML.

## Envelope

```text
MutationEnvelope
├── MutationManifest
│   ├── protocol_version
│   ├── operation_id
│   ├── target_id
│   ├── backend
│   ├── operation
│   ├── resolved_actions[]
│   │   ├── kind
│   │   ├── identity
│   │   └── payload       exact backend-owned UTF-8 JSON bytes
│   └── evidence[]
│       ├── kind
│       └── digest        SHA-256 hex
└── MutationAuthorization
    ├── protocol_version
    ├── authorization_id
    ├── manifest_hash
    ├── authority_kind
    ├── authority_id
    ├── target_id
    ├── issued_at / not_before / expires_at
    ├── decision
    ├── key_id
    └── signature

MutationReceipt          the response half; unsigned, produced by the executor
├── protocol_version
├── operation_id / manifest_hash / authorization_id   the audit join
├── target_id
├── backend / operation
├── decision / reason
├── executed
├── verified_actions
├── exit_code
├── error
└── timestamp
```

The protocol envelope begins at format `1` in its own namespace. It is not a new numbered
generation of `CapabilityToken`; the two named contracts coexist until a backend migrates.
Unknown format values fail closed.

## Target identity

`target_id` names the body being mutated. For RedFlag today the mapping is normative and
narrow:

> **`target_id` MUST equal the locally provisioned RedFlag `agent_id`** — the identity the
> executor reads for itself from a root-owned, SEC-021-validated file, never from the
> envelope.

The manifest and the authorization each carry it, both inside signed bytes, and a verifier
requires them equal. An executor binds the envelope by comparing them with the host
identity it read independently.

The field keeps a generic name so a later protocol may define another target namespace
deliberately. Until one exists, there is no second namespace and no `body_id` — inventing
vocabulary ahead of the thing it names would weaken a binding that already works.

## Authorization discipline

`authorization_id` MUST be a canonical UUID v4 (8-4-4-4-12 lowercase hex, version nibble
`4`, variant nibble `8`/`9`/`a`/`b`) — the same discipline the standalone mint applies to
`request_id`. The executor atomically records it as a one-shot replay claim before pacman
runs. The UUID shape prevents path or record injection into that claim store.

`expires_at - not_before` MUST NOT exceed **3600 seconds**. That ceiling is derived, not
chosen: it is `DefaultTokenTTL`, the fleet minter's own window. Standalone mint is tighter
still at 600s. Both the signer and the verifier enforce it, so an over-long authorization
cannot be minted, not merely refused at the end.

## Canonical records

Every value is encoded as UTF-8 `decimal_byte_length:value`, with no separator between
fields. Each record begins with a length-prefixed domain string.

Manifest field order:

```text
redflag.mutation-manifest
protocol_version
operation_id
target_id
backend
operation
resolved_action_count
sorted(length-prefixed resolved-action records)
evidence_count
sorted(length-prefixed evidence records)
```

Resolved action field order:

```text
redflag.resolved-action
kind
identity
payload
```

Evidence field order:

```text
redflag.evidence
kind
digest
```

Authorization field order:

```text
redflag.mutation-authorization
protocol_version
authorization_id
manifest_hash
authority_kind
authority_id
target_id
issued_at
not_before
expires_at
decision
key_id
```

`manifest_hash` is lowercase hex SHA-256 over the manifest canonical bytes. The Ed25519
signature is over the authorization canonical bytes. The signature itself is not included
in those bytes; every other authorization field, including `key_id`, is.

## Collection semantics

Resolved actions and evidence are unordered multisets. Their canonical records are sorted
bytewise for hashing. Exact duplicates remain present, increment the count, and change the
hash. No implementation may convert either collection to a set.

A backend that needs ordered execution encodes the sequence inside one signed payload. The
common layer never infers package, WUA, Winget, Docker, or self-update semantics from that
payload.

Provenance is evidence. Cache paths, URLs, repository selectors, WUA identities, and other
execution inputs belong in the exact signed backend payload or must be derived
deterministically from it.

Evidence carries **digests, not prose**. A gate verdict, an operator identity, and an
override reason are the authority's own record and stay in its journal; what travels in
the manifest is a digest binding the decision to the evidence it was made over. The
SHA-256 shape check is what enforces that — reason text cannot be smuggled into a signed
manifest through an evidence value.

## Receipt

The receipt is the response half of the contract. It carries the audit join — operation
ID, manifest hash, authorization ID — so a local enforcement record and a Server history
row join on a tuple neither side had to guess. `decision` and `reason` keep the existing
`PolicyResult` taxonomy rather than inventing a second one.

**The receipt is not signed.** The executor is not a second cryptographic authority; this
is a record produced inside the trust boundary that already ran, or refused, the
operation. Its canonical bytes exist so a ledger can digest one without re-deriving field
order from JSON, not so anyone can verify it.

Receipt field order:

```text
redflag.mutation-receipt
protocol_version
operation_id
manifest_hash
authorization_id
target_id
backend
operation
decision
reason
executed              "true" | "false"
verified_actions
exit_code
error
timestamp
```

Every field is present even when empty: a refusal that happens before the envelope parses
still produces a receipt, and its emptiness is part of the record.

## Golden fixture

`testdata/mutation-golden.json` pins:

- manifest canonical bytes and hash;
- authorization canonical bytes;
- authority key ID;
- deterministic Ed25519 signature;
- action/evidence ordering behavior;
- duplicate retention;
- receipt canonical bytes and digest;
- authorization_id shape and the authorization lifetime ceiling;
- tamper refusal for provenance, execution location, target, backend, resolved action,
  authorization metadata, decision, time window, and unknown formats.

Go tests live in both capability packages. Rust tests live in
`helper/src/mutation_protocol.rs`, and the helper's own envelope-path tests live in
`helper/src/main.rs`.

## Executor status

`redflag-helper verify-envelope --envelope-file <path> [--receipt-file <path>]` verifies an
envelope through the helper's real pipeline — parse, host binding, trusted keyring,
signature, time, lifetime ceiling, backend payload shape — and then refuses with
`backend_not_migrated`. This remains the inspection-only compatibility path and consumes no
replay state.

`redflag-helper mint-envelope --request-file <path> --envelope-out <path>` is the standalone
authority for the first migrated backend. A pacman payload marks exactly one requested root
and binds repository, cache locations, archive hash, detached-signature hash, and the exact
`name@version` identity. Before signing, the helper copies every archive and signature into
a root-owned `0700` operation directory, verifies hashes, archive identity, the Arch package
signature, and installed state through pacman's `vercmp`. An `upgrade` requires the requested
root to be strictly newer; an `install` requires it to be absent; dependencies may be absent,
equal, or newer but never older.

`redflag-helper execute-envelope --envelope-file <path> [--receipt-file <path>]` repeats those
checks over a fresh root-owned stage, atomically consumes `authorization_id`, and invokes one
fixed `pacman -U --noconfirm -- <archives...>` plan. There is no `--needed`, so exit zero
cannot collapse a skipped transaction and an applied one into the same receipt. The receipt
records the joined operation, manifest, authorization, fully verified-action count, and
executor outcome.

The Agent-side resolver and Desktop handoff are wired for standalone pacman in the active
source. Exchange reads walk fixed directories with `openat` and no-follow semantics; helper
writes are create-new and joined by UUID filename. RedFlag has no Arch OSV mapping yet, so
local pacman approval records `unsupported` and requires an explicit reason instead of
inventing a clear advisory verdict.

Every other backend still fails closed with `backend_not_migrated`. Server-side fleet
envelope mint and delivery remain separate work. This document describes source behavior;
CI, packaging, installation, and runtime proof remain distinct states.
