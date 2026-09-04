// SEC-025: Fleet-join endpoint. A standalone host calls this to migrate to
// fleet mode. Two-factor: registration token (server-originated) + TOTP code
// proving live possession of the seed enrolled at token creation. The seed is
// held server-side encrypted and never travels on this channel. On success
// the agent is registered and the server returns its authority key(s) so the
// host can retire its local authority.
package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/config"
	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/Fimeg/RedFlag/server/internal/security"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// FleetJoinHandler handles standalone-to-fleet migration (SEC-025).
type FleetJoinHandler struct {
	db               *sqlx.DB
	tokenQueries     *queries.RegistrationTokenQueries
	agentQueries     *queries.AgentQueries
	signingPublicKey  string
	config           *config.Config
}

// NewFleetJoinHandler creates a FleetJoinHandler.
func NewFleetJoinHandler(db *sqlx.DB, tokenQueries *queries.RegistrationTokenQueries, agentQueries *queries.AgentQueries, signingPublicKey string, cfg *config.Config) *FleetJoinHandler {
	return &FleetJoinHandler{
		db:               db,
		tokenQueries:     tokenQueries,
		agentQueries:     agentQueries,
		signingPublicKey: signingPublicKey,
		config:           cfg,
	}
}

// FleetJoinRequest is the standalone host's join-fleet submission. The TOTP
// seed never crosses this channel — the server holds it encrypted from token
// creation and only the 6-digit code travels here.
type FleetJoinRequest struct {
	RegistrationToken string `json:"registration_token" binding:"required"`
	TOTPCode          string `json:"totp_code" binding:"required"`

	// Standard agent registration fields (same as RegisterAgent).
	Hostname             string            `json:"hostname" binding:"required"`
	OSType               string            `json:"os_type" binding:"required"`
	OSVersion            string            `json:"os_version"`
	OSArchitecture       string            `json:"os_architecture"`
	AgentVersion         string            `json:"agent_version"`
	MachineID            string            `json:"machine_id"`
	PublicKeyFingerprint string            `json:"public_key_fingerprint"`
	AvailableScanners    []string          `json:"available_scanners"`
	Metadata             map[string]interface{} `json:"metadata"`
}

// FleetJoinResponse is the server's response on successful fleet join.
type FleetJoinResponse struct {
	AgentID           uuid.UUID `json:"agent_id"`
	RefreshToken      string    `json:"refresh_token"`
	SigningPublicKey  string    `json:"signing_public_key"`
	ServerURL         string    `json:"server_url"`
}

// JoinFleet handles POST /api/v1/fleet-join. Validates the registration token
// and TOTP code, registers the agent, and returns the server's authority key.
func (h *FleetJoinHandler) JoinFleet(c *gin.Context) {
	var req FleetJoinRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Step 1: Validate registration token.
	tokenInfo, err := h.tokenQueries.ValidateRegistrationToken(req.RegistrationToken)
	if err != nil || tokenInfo == nil {
		log.Printf("[SECURITY] [server] [fleet-join] invalid_token error=%v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired registration token"})
		return
	}

	// Step 2: Check that this token requires TOTP (fleet-join tokens only).
	if len(tokenInfo.TOTPSeedEncrypted) == 0 {
		log.Printf("[SECURITY] [server] [fleet-join] no_totp_required token_id=%s", tokenInfo.ID)
		c.JSON(http.StatusBadRequest, gin.H{"error": "this token does not require 2FA — use the standard registration endpoint"})
		return
	}

	// Step 3: Validate the TOTP code against the server-held seed. The seed
	// was stored encrypted at token creation and never crosses this channel;
	// a valid code proves the host holds the seed right now.
	seed, err := h.tokenQueries.DecryptTOTPSeed(tokenInfo)
	if err != nil {
		log.Printf("[ERROR] [server] [fleet-join] seed_decrypt_failed token_id=%s error=%q", tokenInfo.ID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fleet join failed"})
		return
	}
	if !security.ValidateTOTPCode(seed, req.TOTPCode) {
		log.Printf("[SECURITY] [server] [fleet-join] invalid_totp token_id=%s", tokenInfo.ID)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid TOTP code"})
		return
	}

	// Step 5: Check machine ID isn't already registered (same as RegisterAgent).
	if req.MachineID != "" {
		existing, err := h.agentQueries.GetAgentByMachineID(req.MachineID)
		if err == nil && existing != nil && existing.ID.String() != "" {
			c.JSON(http.StatusConflict, gin.H{
				"error":             "machine ID already registered to another agent",
				"existing_agent_id": existing.ID.String(),
			})
			return
		}
	}

	// Step 6: Register the agent (same transaction pattern as RegisterAgent).
	tx, err := h.db.Beginx()
	if err != nil {
		log.Printf("[ERROR] [server] [fleet-join] tx_begin_failed error=%q", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fleet join failed"})
		return
	}
	defer tx.Rollback()

	// Only set MachineID/PublicKeyFingerprint when non-empty; the Agent model
	// stores them as *string so nil maps to SQL NULL, matching the standard
	// registration path and avoiding unique-index violations that &"" would
	// cause when the machine_id column has a partial UNIQUE index.
	var machineID *string
	if req.MachineID != "" {
		machineID = &req.MachineID
	}
	var pubKeyFP *string
	if req.PublicKeyFingerprint != "" {
		pubKeyFP = &req.PublicKeyFingerprint
	}

	agent := &models.Agent{
		ID:                   uuid.Must(uuid.NewV4()),
		Hostname:             req.Hostname,
		OSType:               req.OSType,
		OSVersion:            req.OSVersion,
		OSArchitecture:       req.OSArchitecture,
		AgentVersion:         req.AgentVersion,
		CurrentVersion:       req.AgentVersion,
		MachineID:            machineID,
		PublicKeyFingerprint: pubKeyFP,
		LastSeen:             time.Now().UTC(),
		Status:               "online",
		Metadata:             models.JSONB{},
	}

	if req.Metadata != nil {
		for k, v := range req.Metadata {
			agent.Metadata[k] = v
		}
	}

	createQuery := `
		INSERT INTO agents (
			id, hostname, os_type, os_version, os_architecture,
			agent_version, current_version, machine_id, public_key_fingerprint,
			last_seen, status, metadata
		) VALUES (
			:id, :hostname, :os_type, :os_version, :os_architecture,
			:agent_version, :current_version, :machine_id, :public_key_fingerprint,
			:last_seen, :status, :metadata
		)`
	if _, err := tx.NamedExec(createQuery, agent); err != nil {
		log.Printf("[ERROR] [server] [fleet-join] create_agent_failed error=%q", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register agent"})
		return
	}

	// Mark token as used.
	var tokenSuccess bool
	tokenHash := queries.HashRegistrationToken(req.RegistrationToken)
	if err := tx.QueryRow("SELECT mark_registration_token_used($1, $2)", tokenHash, agent.ID).Scan(&tokenSuccess); err != nil || !tokenSuccess {
		log.Printf("[ERROR] [server] [fleet-join] mark_token_failed error=%v success=%v", err, tokenSuccess)
		c.JSON(http.StatusBadRequest, gin.H{"error": "registration token could not be consumed"})
		return
	}

	// Generate refresh token.
	refreshToken, err := queries.GenerateRefreshToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate refresh token"})
		return
	}
	refreshTokenExpiry := time.Now().UTC().Add(90 * 24 * time.Hour)
	refreshTokenHash := queries.HashRefreshToken(refreshToken)
	familyID := uuid.Must(uuid.NewV4())
	if _, err := tx.Exec("INSERT INTO refresh_tokens (agent_id, token_hash, expires_at, family_id) VALUES ($1, $2, $3, $4)",
		agent.ID, refreshTokenHash, refreshTokenExpiry, familyID); err != nil {
		log.Printf("[ERROR] [server] [fleet-join] create_refresh_token_failed error=%q", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store refresh token"})
		return
	}

	// Create subsystems for available scanners.
	if len(req.AvailableScanners) > 0 {
		for _, scanner := range req.AvailableScanners {
			sub := models.AgentSubsystem{
				AgentID:         agent.ID,
				Subsystem:       scanner,
				Enabled:         true,
				AutoRun:         true,
				IntervalMinutes: 60,
			}
			subQuery := `INSERT INTO agent_subsystems (agent_id, subsystem, enabled, auto_run, interval_minutes) VALUES (:agent_id, :subsystem, :enabled, :auto_run, :interval_minutes)`
			if _, err := tx.NamedExec(subQuery, sub); err != nil {
				log.Printf("[WARNING] [server] [fleet-join] create_subsystem_failed scanner=%s error=%q", scanner, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		log.Printf("[ERROR] [server] [fleet-join] commit_failed error=%q", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "fleet join failed"})
		return
	}

	log.Printf("[INFO] [server] [fleet-join] agent_registered agent_id=%s hostname=%s machine_id=%s",
		agent.ID, agent.Hostname, req.MachineID)

	c.JSON(http.StatusCreated, FleetJoinResponse{
		AgentID:          agent.ID,
		RefreshToken:     refreshToken,
		SigningPublicKey: h.signingPublicKey,
		ServerURL:        resolveServerURL(c, h.config, "fleet-join"),
	})
}
