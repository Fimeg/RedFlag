# RedFlag Desktop

**The native local-machine observation and operations surface. QML is the glass; the Agent and helper own the machine.**

---

## Product role

RedFlag Desktop is a Qt 6 / QML system monitor and local operations console for one
machine. It works in standalone mode and keeps the same domain when the machine joins a
fleet. RedFlag Web asks across machines; Desktop asks here, now, on this body.

The application currently carries eleven native surfaces:

- Overview and Performance;
- Processes, Network, Storage, Services, and Containers;
- installed Software and available Updates;
- Security and History.

The useful unit is the join between those surfaces. A process can carry its systemd unit,
container identity, sockets, namespaces, capabilities, and owning package. Package and
update details lead toward dependency closure, advisory evidence, authorization, and
mutation history instead of remaining a separate updater product.

Souveraine Updater no longer owns a product boundary here. Useful closure and package UX
may be absorbed, but its direct `pkexec pacman` authority must not survive. The Tauri /
React Desktop runtime has also been removed; the fleet React application remains RedFlag
Web.

---

## Authority boundary

```text
RedFlag Desktop (Qt / QML)
        observation + operator intent
                    |
                    v
RedFlag Agent       local source of truth, resolution, gates
                    |
                    v
RedFlag Helper      privileged verifier / bounded executor
                    |
                    v
platform backend    pacman first; other migrated backends follow
```

Desktop never shells out to pacman, Docker, or systemd and never reads a signing key. It
speaks HTTP/1.1 over the Agent local socket
(`/var/lib/redflag/agent/localapi/redflag-agent.sock`; named pipe
`\\.\pipe\RedFlagAgentLocal` on Windows). The user must be admitted to the
`redflag-local` OS group or the kernel refuses the connection.

The current socket still lacks `SO_PEERCRED` attribution and fresh StepUp. Desktop sends
the session username as an assertion, not an attestation. A same-user process able to
reach the socket can express the same intent. The helper constrains what bytes and
operations can execute; it does not prove which human clicked the button.

Fleet enrollment removes local mint authority. Desktop may still display this machine,
but approval belongs to RedFlag Server and the Agent refuses local authorization.

---

## Native structure

```text
desktop/
├── Cargo.toml / build.rs
├── src/
│   ├── main.rs
│   └── bridge/
│       ├── local_api.rs       HTTP over Unix socket / named pipe
│       └── machine.rs         CXX-Qt state and invokable intent
├── qml/
│   ├── Main.qml
│   ├── Theme.qml
│   ├── MetricCard.qml
│   ├── LineGraph.qml
│   └── NavItem.qml
└── icons/icon.png
```

The Rust bridge polls bounded Agent projections. Live resource telemetry runs at one
second with a 300-point in-memory history. Larger inventory and detail reads are explicit
and asynchronous so the QML thread does not become a second scanner.

If the Agent is reachable but lacks newer routes, Desktop names the missing surface as an
Agent version gap rather than presenting an empty healthy machine. Unknown and unsupported
remain distinct from zero.

---

## Local update intent

`POST /v1/actions/approve-update` is the only Desktop package-approval call. APT and DNF
use the closure-capability path. Pacman now uses the mutation-envelope path:

1. the Agent refreshes a private pacman database and resolves one exact dependency
   transaction without touching live package state;
2. it downloads every archive and detached signature into a private cache and records
   exact identity, repository, locations, and hashes in the manifest;
3. the standalone helper stages those bytes under root custody, verifies identity and
   Arch signatures, enforces non-decreasing versions, checks gate evidence, and signs the
   envelope with the root-owned local authority;
4. execution repeats custody, hash, identity, signature, and version checks, atomically
   consumes `authorization_id`, runs one fixed pacman argv, and returns a joined receipt.

OSV has no Arch ecosystem mapping in the current RedFlag gate. Desktop displays
`unsupported` and requires a recorded override reason; it never turns missing advisory
coverage into a green check.

Override uses this same path. It is intent plus a reason inside the same authority and
journal, not a second privileged button.

---

## Lifecycle and proof

The native Linux Desktop is built by Gitea Actions with Qt 6 and CXX-Qt. Windows still has
Agent support but no native Qt/MSVC Desktop artifact until a suitable runner and packaging
path exist. Source presence is not an installed or runtime proof.

Desktop self-update remains a capability-gated helper swap. Installation must also provide
the local socket group, autostart/background presence, and exact helper sudo protocols;
QML does not compensate for incomplete provisioning.

---

## Cross-references

- [security/01-trust-boundaries](../security/01-trust-boundaries.md)
- [security/06-standalone-authority](../security/06-standalone-authority.md)
- [components/02-agent](02-agent.md)
- [components/04-helper](04-helper.md)
- [scanners/06-pacman-scanner](../scanners/06-pacman-scanner.md)

---

*Last reviewed: 2026-09-01*
