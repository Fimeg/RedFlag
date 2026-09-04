# Pacman Scanner and Resolver

**Arch update discovery stays unprivileged; approved mutation crosses the signed helper envelope.**

---

## Discovery

| Property | Value |
|----------|-------|
| Method | `checkupdates --color=never` |
| Platform | Arch Linux and pacman derivatives |
| Output | `pkgname oldver -> newver` |
| Root required | No |
| Requirements | `pacman`, `pacman-contrib`, `fakeroot` |

`agent/internal/scanner/pacman.go` runs pacman-contrib's `checkupdates` through
`DiscoveryRunner`. `checkupdates` synchronizes a private database and compares it with the
installed local database. The Agent has no `pacman -Sy` sudo grant and discovery never
refreshes live sync state.

Epoch-bearing versions remain opaque pacman versions. The parser preserves a line such as
`fakeroot 1:1.37.2-1 -> 1:1.37.2-2`; it does not split or reinterpret the epoch.

---

## Standalone closure resolution

`agent/internal/installer/pacman_resolver_linux.go` owns the approval-time resolver:

1. create an Agent-private operation directory, sync database, and cache;
2. symlink only the installed `local` database into that private database;
3. run `fakeroot pacman -Sy` against the private database with a cleared, C locale;
4. resolve the requested exact `name=version` transaction and repository identities;
5. download the transaction with `pacman -Sw` into the private cache;
6. inspect each archive with locale-pinned `pacman -Qp`, require its adjacent detached
   signature, and hash both files;
7. keep the cache alive through helper mint and execution, then remove it.

The resolver returns an error rather than guessing when the requested root is absent, the
repository join is missing, an archive or signature is not regular, identity output is
ambiguous, or the transaction is empty.

---

## Privileged path

The Agent encodes each package as `name@version` plus an exact JSON payload containing:

- repository;
- whether this action is the one requested root;
- archive cache path and SHA-256;
- detached-signature cache path and SHA-256.

The standalone helper will not sign a pacman envelope without every detached signature.
Before mint it copies Agent paths into a root-owned `0700` operation directory, verifies
both hashes, checks archive identity with `pacman -Qp`, verifies the detached signature
through the Arch keyring, and uses pacman's `vercmp` against installed state. Execution
repeats those checks, atomically consumes the authorization, and invokes:

```text
/usr/bin/pacman -U --noconfirm -- <staged archives...>
```

Source and exchange files are opened without following the final symlink. Helper exchange
directories are walked component-by-component with `openat(..., O_NOFOLLOW)` and result
files are create-new, so an Agent-controlled path cannot redirect a root write through a
swapped directory or pre-existing link.

There is no `--needed`: a successful receipt cannot conceal a package-manager skip. The
helper requires exactly one requested root. `upgrade` requires that root to be installed at
a strictly older version; `install` requires it to be absent. Equal versions remain valid
dependencies, while every older target is refused.

---

## Limits

- AUR packages are not scanned or resolved.
- Requested-root identity lives in the signed pacman payload rather than the common
  manifest, so other backends do not inherit pacman semantics accidentally.
- RedFlag currently has no OSV ecosystem mapping for Arch. Local approval records
  `unsupported` and requires an explicit override reason.
- Package signature verification proves the artifact against the local Arch keyring. It
  does not add reproducible-build or transparency-log evidence.

---

## Cross-references

- [security/05-supply-chain-gate](../security/05-supply-chain-gate.md)
- [security/06-standalone-authority](../security/06-standalone-authority.md)
- [components/04-helper](../components/04-helper.md)
- `protocol/README.md`

---

*Last reviewed: 2026-09-01*
