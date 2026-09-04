# Authentication Stack

**Four-layer authentication: registration tokens → JWT → refresh tokens → machine binding.**

---

## Layer 1: Registration Tokens

**Purpose:** One-time enrollment tokens

**Format:** Random 64-character hex string

**Lifecycle:**
1. Server generates token with `max_seats` count
2. Admin distributes token to operators
3. Agent uses token to register
4. Server marks token as used and increments `seats_used`
5. Token is revoked after use

**Endpoint:** `POST /api/v1/agents/register`

**Request:**
```json
{
  "hostname": "server-01",
  "os_type": "linux",
  "os_version": "6.19",
  "machine_id": "sha256-fingerprint...",
  "public_key": "ed25519-public-key...",
  "available_scanners": ["apt", "dnf", "docker"]
}
```

**Response:**
```json
{
  "agent_id": "uuid-4",
  "jwt_token": "...",
  "refresh_token": "...",
  "server_public_key": "..."
}
```

**Cross-references:**
- [flows/01-registration](../flows/01-registration.md) (registration flow)
- [security/03-refresh-tokens](03-refresh-tokens.md) (refresh tokens)

---

## Layer 2: JWT Access Tokens

**Purpose:** Short-lived access tokens for API calls

**Issuer:** `"redflag-agent"` for agents, `"redflag-web"` for web

**Duration:** 24 hours

**Algorithm:** HS256

**Claims:**
```json
{
  "sub": "agent-uuid",
  "iss": "redflag-agent",
  "exp": 1234567890,
  "iat": 1234567890
}
```

**Validation:**
```go
func AuthMiddleware() gin.HandlerFunc {
    return func(c *gin.Context) {
        authHeader := c.GetHeader("Authorization")
        if authHeader == "" {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
            c.Abort()
            return
        }

        tokenString := strings.TrimPrefix(authHeader, "Bearer ")
        if tokenString == authHeader {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization format"})
            c.Abort()
            return
        }

        token, err := jwt.ParseWithClaims(tokenString, &AgentClaims{}, func(token *jwt.Token) (interface{}, error) {
            return []byte(JWTSecret), nil
        })

        if err != nil || !token.Valid {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
            c.Abort()
            return
        }

        if claims, ok := token.Claims.(*AgentClaims); ok {
            // Validate issuer to prevent cross-type token confusion
            if claims.Issuer != "" && claims.Issuer != JWTIssuerAgent {
                c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token type"})
                c.Abort()
                return
            }
            c.Set("agent_id", claims.AgentID)
            c.Next()
        } else {
            c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token claims"})
            c.Abort()
        }
    }
}
```

**Cross-references:**
- [security/01-trust-boundaries](01-trust-boundaries.md) (trust boundary matrix)
- [flows/02-command-execution](../flows/02-command-execution.md) (polling loop)

---

## Layer 3: Refresh Tokens

**Purpose:** Long-lived authentication for polling, with rotation and reuse detection

**Format:** 64-character hex string (32 bytes crypto/rand)

**Storage:** SHA-256 hash in database; rotation lineage via `family_id`, `consumed_at`, `superseded_by` (migration 045)

**Duration:** 90 days (bumped on each renewal, not on every check-in)

**Lifecycle:**
1. Generated at registration with a fresh `family_id` (root of the rotation chain)
2. Agent calls `POST /renew` only when its JWT expires (~24h) — not on every poll
3. Each renewal mints a **successor** token in the same family and marks the parent `consumed`
4. Server returns the new refresh token alongside the new JWT; agent persists it to `config.json`
5. Accept-previous-once grace: an agent that crashed before persisting the new token can retry with the consumed old one — the server sees the successor is still unconsumed and re-issues
6. Reuse detection: a consumed token presented after its successor is also consumed → entire family revoked, security event logged, both parties locked out

**Machine binding:** Renewal requires `X-Machine-ID` to match the registered host (same as command endpoints). A stolen `config.json` cannot mint access tokens from an unregistered machine.

**Instance lock:** A `flock` (Unix) or named kernel mutex (Windows) prevents two agent processes from sharing the same `config.json` on the same host, serializing renewal at the process level.

**Endpoint:** `POST /api/v1/agents/renew`

**Request:**
```json
{
  "agent_id": "uuid",
  "refresh_token": "64-char-hex",
  "agent_version": "0.2.0.7"
}
```
Headers: `X-Machine-ID` (required), `Content-Type: application/json`

**Response:**
```json
{
  "token": "new-jwt...",
  "refresh_token": "new-64-char-hex..."
}
```
The agent must persist `refresh_token` to disk; if it crashes before doing so, accept-previous-once grace recovers on the next attempt.

**Cross-references:**
- [flows/01-registration](../flows/01-registration.md) (registration flow)
- [flows/02-command-execution](../flows/02-command-execution.md) (renewal in polling loop)

---

## Layer 4: Machine Binding

**Purpose:** Bind JWT to specific hardware

**Method:** SHA-256 hash of machine-id + hostname (no boot-id)

**Validation:** Middleware checks `X-Machine-ID` header matches DB

**Failure modes:**
- Machine ID mismatch → 403 Forbidden
- Agent row deleted → 401 Unauthorized
- Update in progress → Validates nonce

**Middleware:** `server/internal/api/middleware/machine_binding.go`

**Cross-references:**
- [security/01-trust-boundaries](01-trust-boundaries.md) (trust boundary matrix)
- [flows/01-registration](../flows/01-registration.md) (machine ID generation)

---

## Footer: Assumptions & Connections

**Assumption:** Trust On First Use (TOFU) model — agent caches server public key at registration and uses it for all future verification.

**Connection:** [security/01-trust-boundaries](01-trust-boundaries.md) (trust boundary matrix)

**Connection:** [security/04-machine-binding](04-machine-binding.md) (hardware-bound auth)

**Connection:** [verification/01-signing-pipeline](../verification/01-signing-pipeline.md) (Ed25519 signing)

**Connection:** [verification/02-agent-verification](../verification/02-agent-verification.md) (command verification)

---

*Last reviewed: 2026-06-14*