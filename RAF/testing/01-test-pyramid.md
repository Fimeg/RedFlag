# Testing

**What's tested, how, and where the honest gaps are.**

---

## Shape

~98 Go test files across the repo (61 server, 37 agent), plus Rust tests in the helper. Coverage concentrates where the security model lives — verification, token lifecycle, scanners with hostile input — rather than chasing a percentage.

| Layer | What it covers | Examples |
|-------|----------------|----------|
| Unit (Go) | Crypto verification, replay protection, backoff, machine-id derivation, scanner parsers | `agent/internal/crypto/*_test.go`, `winget_parser_test.go`, `windows_ghost_test.go` |
| Unit (Rust) | Helper token verification, hash checks | `helper/` cargo tests |
| Cross-language contract | Current closure-token vectors plus the dormant mutation manifest/authorization envelope — Rust and Go must produce byte-identical canonical bytes, hashes, and signatures | shared `protocol/testdata/` fixture + helper/server/agent tests |
| Structural | Tests that assert properties of the *source*, not behavior — e.g. `token_renewal_transaction_test.go` asserts the renewal handler's transactional shape; `ethos_exempt_test.go` polices logging discipline | server + agent |
| Migration | Idempotency and schema invariants | `server/internal/database/queries/*_test.go` |
| CI | `go vet`, `go test -race`, `cargo test` + clippy, `tsc --noEmit` on every push | `.gitea/workflows/ci.yml` |

The structural-test category is unusual and deliberate: where a rule matters more than any single behavior (transaction boundaries, ETHOS logging), a test reads the source and fails on regression. Cheaper than a linter plugin, louder than a comment.

---

## Manual / Live Testing

- A live Fedora agent runs against the dev stack continuously — DNF scanning, replay protection observed firing in production logs.
- The supply-chain gate completed a live end-to-end run 2026-06-05: real package, install → hash-pin → token mint → helper verify+execute → receipt.
- Windows agents test against physical machines (intermittently available) — Windows coverage leans harder on unit tests of parsers and the WUA layer as a result.

---

## Honest Gaps

- **No web UI tests.** The dashboard is exercised by hand. TypeScript compilation (`tsc --noEmit`) is the only automated check.
- **No end-to-end suite.** The live-agent loop substitutes for one; it does not run in CI.
- **Windows paths are under-exercised live** relative to Linux — see manual testing above.
- Test counts are a proxy, not a coverage claim. Nothing here should be read as "battle-tested"; the README says the same.

---

*Last reviewed: 2026-06-11*
