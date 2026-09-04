# ETHOS Principles

**Core identity of RedFlag — security-first, error-transparent, resilient.**

---

## The Five Principles

### 1. Errors are History

**Never silence errors.** Every error is logged with full context using the standard format:

```
[TAG] [system] [component] message
```

**Examples:**
- `[security] [system] [auth] JWT validation failed: expected issuer "redflag-agent", got "redflag-web"`
- `[reliability] [agent] [polling] Server unavailable: exponential backoff to 5m`
- `[operation] [server] [scheduler] Job skipped: scanner unavailable for platform`

**Anti-pattern:**
```go
// BAD
if err != nil { return nil }  // Silent failure

// GOOD
if err != nil {
    logSecurityEvent(errors.Wrap(err, "command dispatch failed"))
    return nil, err
}
```

---

### 2. Security is Non-Negotiable

**No unauthenticated endpoints ever.** Every route must be classified by its trust boundary.

**Authentication layers:**
1. **Public** — No auth (registration tokens, install scripts)
2. **Agent-auth** — JWT + Machine ID binding
3. **Web-auth** — Admin JWT
4. **Admin-only** — Web-auth + admin role claim

**Rule:** If you can't answer "who is this?" and "are they authorized?", the endpoint is not authorized.

---

### 3. Assume Failure; Build for Resilience

**Circuit breakers, retries, graceful degradation.** Don't assume connectivity, storage, or computation will succeed.

**Patterns:**

| Pattern | Implementation | When to Use |
|---------|----------------|-------------|
| Circuit Breaker | `agent/internal/circuitbreaker/circuitbreaker.go` | External APIs, scanners |
| Retry with Backoff | `agent/internal/retry/retry.go:calculateDelay()` | Server unavailable |
| At-Least-Once Delivery | `pending_acks.json` + retry | Command dispatch |
| Buffering | `events_buffer.json` | Network partition |
| Atomic Operations | Database transactions | State changes |

**ETHOS alignment:**
- If a scanner fails 5 times in 60s → Open circuit breaker
- If server returns 502 → Backoff (10s → 20s → 40s → ... → 5min)
- If command dispatch fails → Log, retry on next poll, don't silently drop

---

### 4. Idempotency is a Requirement

**All operations safe to repeat.** Running an operation 3x produces the same result as running it once.

**Idempotent patterns:**
- **Database:** INSERT ... ON CONFLICT DO NOTHING (UPSERT)
- **Deduplication:** `executed_commands.json` persists executed IDs
- **Reconciliation:** Poll-based sync (`syncAvailableScanners`) re-runs safely
- **Key rotation:** `SetPrimaryKey()` atomically transitions within a transaction

**Anti-pattern:**
```go
// BAD — Not idempotent
DELETE FROM agents WHERE id = ?  // Running twice is a bug

// GOOD — Idempotent
DELETE FROM agents WHERE id = ? AND deleted_at IS NULL  // Safe to repeat
```

---

### 5. No Marketing Fluff

**Technical accuracy over buzzwords.** Banned words: "robust", "seamless", "enhanced", "enterprise-ready", "future-proof".

**Replace with:**
- "resilient" instead of "robust"
- "transparent" instead of "seamless"
- "comprehensive" instead of "enhanced"
- "self-hosted" instead of "enterprise-ready"

**Banned emojis in logs** — logs must be plain text for parsing.

---

## ETHOS Cross-References

- **Errors are History** → `flows/04-heartbeat.md` (error transparency in polling loop)
- **Security is Non-Negotiable** → `security/02-authentication-stack.md` (four-layer auth)
- **Assume Failure** → `verification/04-replay-protection.md` (circuit breakers + nonce validation)
- **Idempotency** → `flows/05-capability-advertisement.md` (syncAvailableScanners diff operation)
- **No Marketing Fluff** → `reference/02-glossary.md` (technical definitions)

---

## Footer: Assumptions & Connections

**Assumption:** ETHOS principles are enforced via pre-commit hooks and code review checklist.

**Connection:** Each principle maps to a specific RAF section — violations surface as structural pattern breaches (RAF §11).

**Connection:** `verification/04-replay-protection.md` implements ETHOS #3 (#4) at the agent boundary.

**Connection:** `flows/05-capability-advertisement.md` implements ETHOS #4 (idempotent scanner sync).

---

*Last reviewed: 2026-06-14*
