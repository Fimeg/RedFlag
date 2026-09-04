# Agent Registration Flow

**TOFU key caching and hardware-bound registration.**

---

## Overview

Agent registers with server using registration token, hardware fingerprint, and Ed25519 keypair. Server validates machine binding and caches public keys for TOFU model.

**Cross-references:**
- `security/02-authentication-stack.md` (trust boundary matrix)
- `verification/01-signing-pipeline.md` (key generation)
- `verification/02-agent-verification.md` (TOFU key caching)
- `security/04-machine-binding.md` (hardware verification)

---

## Step-by-Step Flow

### 1. Agent Prepares Registration

**File:** `agent/internal/system/machine_id.go` + `agent/internal/registration/service.go`

```go
func (r *RegistrationService) PrepareRegistration() (*RegisterRequest, error) {
    // 1. Generate machine ID (SHA-256 hardware fingerprint)
    machineID, err := system.GenerateMachineID()  // Uses machineid library
    if err != nil {
        return nil, err
    }

    // 2. Generate Ed25519 keypair
    privateKey, publicKey, err := ed25519.GenerateKey(rand.Reader)
    if err != nil {
        return nil, err
    }

    // 3. Detect available scanners
    scanners := scanner.DetectAvailable()

    // 4. Build request
    request := &RegisterRequest{
        Hostname:       hostname,
        OS_Type:        osType,
        OS_Version:     osVersion,
        Machine_ID:     machineID,
        Public_Key:     hex.EncodeToString(publicKey),
        AvailableScanners: scanners,
    }

    return request, nil
}
```

---

### 2. Agent Calls Registration Endpoint

**Endpoint:** `POST /api/v1/agents/register`

**Request:**
```json
{
  "hostname": "server-01",
  "os_type": "linux",
  "os_version": "6.19",
  "machine_id": "sha256-fingerprint...",
  "public_key": "ed25519-public-key...",
  "available_scanners": ["apt", "docker"]
}
```

**Response:**
```json
{
  "agent_id": "uuid-4",
  "server_url": "https://redflag.example.com",
  "jwt_token": "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...",
  "refresh_token": "64-char-hex...",
  "server_public_key": "ed25519-public-key...",
  "config": {
    "check_in_interval": 300,
    "rapid_polling_enabled": false
  }
}
```

---

### 3. Server Validates and Registers

**File:** `server/internal/api/handlers/agents.go`

```go
func (h *AgentHandler) RegisterAgent(w http.ResponseWriter, r *http.Request) {
    // 1. Extract registration token
    token := extractRegistrationToken(r)
    if err := h.validateRegistrationToken(token); err != nil {
        http.Error(w, err.Error(), http.StatusUnauthorized)
        return
    }

    // 2. Validate machine ID (prevent duplicate)
    machineID := r.Header.Get("X-Machine-ID")
    if agent, _ := h.db.GetAgentByMachineID(machineID); agent != nil {
        http.Error(w, "machine already registered", http.StatusConflict)
        return
    }

    // 3. Create agent
    agentID := uuid.New()
    serverURL := r.URL.Query().Get("server_url")
    if serverURL == "" {
        serverURL = os.Getenv("REDFLAG_SERVER_URL")
    }

    h.db.BeginTx(func(tx *sql.Tx) error {
        // 4. Insert agent
        tx.Exec(`
            INSERT INTO agents (id, hostname, os_type, os_version, machine_id, metadata)
            VALUES ($1, $2, $3, $4, $5, $6)
        `, agentID, hostname, osType, osVersion, machineID, "")

        // 5. Create refresh token
        refreshToken := generateRefreshToken()
        refreshTokenHash := sha256.Sum256([]byte(refreshToken))
        tx.Exec(`
            INSERT INTO refresh_tokens (agent_id, hash, expires_at, revoked)
            VALUES ($1, $2, NOW() + INTERVAL '90 days', false)
        `, agentID, hex.EncodeToString(refreshTokenHash[:]))

        // 6. Create platform subsystems (idempotent)
        for _, scanner := range availableScanners {
            tx.Exec(`
                INSERT INTO agent_subsystems (agent_id, name, enabled)
                VALUES ($1, $2, true)
                ON CONFLICT (agent_id, name) DO NOTHING
            `, agentID, scanner)
        }

        return nil
    })

    // 7. Generate JWT
    jwtToken := generateJWT(agentID, "redflag-agent", 24*time.Hour)

    // 8. Return tokens
    json.NewEncoder(w).Encode(map[string]interface{}{
        "agent_id":        agentID,
        "server_url":      serverURL,
        "jwt_token":       jwtToken,
        "refresh_token":   refreshToken,
        "server_public_key": serverPublicKey,
        "config":          config,
    })
}
```

---

### 4. Agent Caches Server Public Key (TOFU)

**File:** `agent/internal/crypto/pubkey.go`

```go
func (a *Agent) CacheServerPublicKey(pubKey []byte) error {
    // Store public key
    os.WriteFile(
        filepath.Join(a.configDir, "server_public_key"),
        pubKey,
        0600,
    )

    // Store metadata
    metadata := fmt.Sprintf(`{"expires_at": "%s"}`,
        time.Now().Add(24*time.Hour).Format(time.RFC3339))
    os.WriteFile(
        filepath.Join(a.configDir, "server_public_key.meta"),
        []byte(metadata),
        0600,
    )

    a.serverPublicKey = pubKey
    return nil
}
```

---

### 5. Agent Saves Configuration

**File:** `/etc/redflag/agent/config.json`

```json
{
  "agent_id": "uuid-4",
  "server_url": "https://redflag.example.com",
  "token": "jwt-access-token",
  "refresh_token": "64-char-hex...",
  "machine_id": "sha256-fingerprint...",
  "check_in_interval": 300,
  "rapid_polling_enabled": false,
  "subsystems": {
    "apt": {"enabled": true},
    "docker": {"enabled": true},
    "system": {"enabled": true}
  }
}
```

---

### 6. Server Validates Machine Binding on Poll

**Middleware:** `server/internal/middleware/machine_binding.go`

```go
func MachineBindingMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // 1. Extract JWT
        claims, err := extractJWTClaims(r)
        if err != nil {
            http.Error(w, "unauthorized", http.StatusUnauthorized)
            return
        }

        // 2. Extract machine ID
        reportedMachineID := r.Header.Get("X-Machine-ID")
        if reportedMachineID == "" {
            http.Error(w, "missing machine ID", http.StatusBadRequest)
            return
        }

        // 3. Validate machine ID matches DB
        dbMachineID, err := getAgentMachineID(claims.AgentID)
        if err != nil {
            http.Error(w, "agent not found", http.StatusUnauthorized)
            return
        }

        if dbMachineID != reportedMachineID {
            http.Error(w, "machine ID mismatch", http.StatusForbidden)
            return
        }

        // 4. Continue
        next.ServeHTTP(w, r)
    })
}
```

---

## Trust Boundaries

| Trust Boundary | Endpoint | Middleware | Notes |
|----------------|----------|------------|-------|
| Public | `POST /api/v1/agents/register` | Registration token | One-time enrollment |
| Public | `GET /api/v1/install/:platform` | Rate limit | Bootstrapping |
| Public | `GET /api/v1/downloads/:platform` | Rate limit | Binary distribution |
| Agent | `GET /api/v1/agents/:id/commands` | `AuthMiddleware + MachineBindingMiddleware` | Requires JWT + correct machine ID |
| Agent | `POST /api/v1/agents/:id/reports` | `AuthMiddleware + MachineBindingMiddleware` | Requires JWT + correct machine ID |

**Cross-references:**
- `security/01-trust-boundaries.md` (full trust boundary matrix)
- `security/02-authentication-stack.md` (auth layers)

---

## Footer: Assumptions & Connections

**Assumption:** Registration is a one-time operation — agent identity is established and persisted.

**Connection:** TOFU caching (`verification/02-agent-verification.md`) enables trust continuity without repeated key exchange.

**Connection:** Machine binding (`security/04-machine-binding.md`) enforces hardware-bound authentication on all subsequent requests.

**Connection:** Capability advertisement (`flows/05-capability-advertisement.md`) updates scanner availability post-registration.

**Connection:** Refresh tokens (`security/03-refresh-tokens.md`) provide long-lived polling authentication.

---

*Last reviewed: 2026-05-26*
