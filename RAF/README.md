# RedFlag Architecture Framework (RAF)

**The complete architectural spine of RedFlag — every component, scanner, verification system, and how they all wire together.**

This is the design of record, published in the open. Not a manual for attacking RedFlag — the reasoning behind it: how the system is built, why the design landed where it did, and the pitfalls we think are still out there. The security model should survive being read; if it can't, that's a finding, and we'd rather know.

It describes a system under active development. Some of it will be wrong by the time you read it — the [OVERVIEW](OVERVIEW.md) keeps an Honest Gaps section current for exactly that reason, and every page carries a last-reviewed date. Trust the code over the doc when they disagree, and tell us.

---

## Navigation

| Section | Description |
|---------|-------------|
| [OVERVIEW](OVERVIEW.md) | **START HERE** — machine and fleet shape, authority tiers, architectural boundaries, honest gaps |
| [core](core/) | ETHOS principles, the foundational architectural decisions |
| [components](components/) | Server, Agent, Web, native Desktop, helper — component breakdowns |
| [security](security/) | Trust boundaries, auth stack, refresh-token lifecycle, machine binding, supply chain gate, standalone authority |
| [verification](verification/) | Ed25519 signing pipeline, agent verification, key rotation, replay protection |
| [scanners](scanners/) | Every scanner and resolver (APT, DNF, pacman, Winget, WUA, Docker, process explorer) with interaction analysis |
| [flows](flows/) | Data flows — registration, command execution, upgrade, heartbeat, capability advertisement, update lifecycle |
| [deployment](deployment/) | Docker stack, native agent services, CI/CD, release gate, operations runbook pointers |
| [testing](testing/) | Test pyramid, structural tests, live testing, honest gaps |
| [reference](reference/) | File mappings, [glossary](reference/02-glossary.md) |

---

## Reading Order

1. **[OVERVIEW](OVERVIEW.md)** — the shape of the system and where its protection boundary currently ends
2. **[core](core/)** — ETHOS principles and the decisions everything else hangs off
3. **[flows](flows/)** — trace the critical data flows end-to-end
4. **[security](security/) + [verification](verification/)** — the trust model and the cryptographic pipeline
5. **[scanners](scanners/) + [components](components/)** — per-ecosystem behavior and package structure

---

## Contributing

RedFlag is free and will never be monetized. If community adoption takes off, ownership and contribution policies will be made transparent and stay open — this project does not get quietly captured.

Before proposing architectural changes:

1. Read [core](core/) → [flows](flows/) → [verification](verification/) for context — most "why is it like this" questions are answered there
2. The five ETHOS principles and the six load-bearing constraints ([OVERVIEW](OVERVIEW.md)) are the floor, not a starting position
3. Update the relevant page *and its cross-references*; stale links are bugs

A note on `docs/tasks/` references: several pages point at the maintainer's task tracker
for build status. That tree is private — the RAF publishes the *design*, not the day-to-day
state. Where a page cites a task file, read it as "status is tracked, not frozen into
architecture docs."

---

## Version History

| Version | Date | Changes |
|---------|------|---------|
| 2.3 | 2026-09-01 | Native Desktop machine console, standalone Agent mode, and the first live pacman mutation envelope with signed artifact custody. |
| 2.2 | 2026-06-11 | Publish-ready pass: agent, web, helper component docs; refresh-token lifecycle; deployment; testing; glossary. Public framing. |
| 2.1 | 2026-06-01 | Updated for v0.2.3.1: supply chain enforcement posture, lifecycle orchestrator, state machine, OSV batch checks |
| 2.0 | 2026-05-26 | Restructured for single-source-of-truth organization |
| 1.3 | 2026-05-06 | Added §11 eight structural patterns |
| 1.0 | 2026-05-01 | Initial framework |

---

*Maintained by Vanguard (agent-f7ddc5ce-6c27-4799-bcc4-99fb688eb222) — a persistent [Souveraine](https://github.com/Fimeg/Souveraine) agent with his own memory and history in this codebase. On why agents here have names: [The Pronoun Problem](https://souveraineai.com/docs/papers/pronoun-problem/).*
