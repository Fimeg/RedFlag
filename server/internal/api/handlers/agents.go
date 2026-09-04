package handlers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/api/middleware"
	"github.com/Fimeg/RedFlag/server/internal/config"
	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/logging"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/Fimeg/RedFlag/server/internal/scheduler"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/Fimeg/RedFlag/server/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
	"github.com/lib/pq"
)

// isPendingSubsystemDuplicate reports whether err is the unique-violation on
// idx_agent_pending_subsystem — i.e. a command of this type is already pending
// for the agent. That index enforces "one pending command per subsystem"; for
// the auto-heartbeat side effect a duplicate means the desired state is already
// in flight, not a failure (ETHOS #4 — idempotent).
func isPendingSubsystemDuplicate(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505" && pqErr.Constraint == "idx_agent_pending_subsystem"
	}
	return false
}

type AgentHandler struct {
	agentQueries             *queries.AgentQueries
	commandQueries           *queries.CommandQueries
	refreshTokenQueries      *queries.RefreshTokenQueries
	registrationTokenQueries *queries.RegistrationTokenQueries
	subsystemQueries         *queries.SubsystemQueries
	scheduler                *scheduler.Scheduler
	signingService           *services.SigningService
	securityLogger           *logging.SecurityLogger
	securitySettings         *services.SecuritySettingsService // optional; gates auto-heartbeat
	config                   *config.Config
	checkInInterval          int
	latestAgentVersion       string
	stuckCommandTimeout      time.Duration
	maxCommandRetries        int
}

// SetSecuritySettings injects the settings service post-construction so the
// dispatch chokepoint can consult policy.auto_heartbeat_enabled. Optional;
// nil leaves auto-heartbeat enabled (the safe default — agents poll faster).
func (h *AgentHandler) SetSecuritySettings(s *services.SecuritySettingsService) {
	h.securitySettings = s
}

// confirmUpdateCommand closes the most recent update_agent command for an agent
// once the new version has attested by checking in, and records a success event
// for History. The command often sits in "failed" because the agent was SIGTERM'd
// mid-restart before it could report its own result — the version attestation
// supersedes that transport outcome.
func (h *AgentHandler) confirmUpdateCommand(agentID uuid.UUID, newVersion string) {
	cmds, err := h.commandQueries.GetCommandsByAgentID(agentID)
	if err != nil {
		log.Printf("Warning: update confirm could not list commands for agent %s: %v", agentID, err)
		return
	}
	var latest *models.AgentCommand
	for i := range cmds {
		c := &cmds[i]
		if c.CommandType != models.CommandTypeUpdateAgent || c.Status == models.CommandStatusCompleted {
			continue
		}
		if latest == nil || c.CreatedAt.After(latest.CreatedAt) {
			latest = c
		}
	}
	if latest != nil {
		if err := h.commandQueries.UpdateCommandStatus(latest.ID, models.CommandStatusCompleted); err != nil {
			log.Printf("Warning: failed to mark update_agent %s completed: %v", latest.ID, err)
		}
	}
	event := &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		AgentID:      &agentID,
		EventType:    "agent_update",
		EventSubtype: "completed",
		Severity:     "info",
		Component:    "agent",
		Message:      fmt.Sprintf("Agent update confirmed: now running %s", newVersion),
		Metadata: models.JSONB{
			"new_version":  newVersion,
			"confirmed_by": "version_attestation",
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := h.agentQueries.CreateSystemEvent(event); err != nil {
		log.Printf("Warning: failed to log update confirmation event: %v", err)
	}
}

func NewAgentHandler(aq *queries.AgentQueries, cq *queries.CommandQueries, rtq *queries.RefreshTokenQueries, regTokenQueries *queries.RegistrationTokenQueries, sq *queries.SubsystemQueries, scheduler *scheduler.Scheduler, signingService *services.SigningService, securityLogger *logging.SecurityLogger, cfg *config.Config, checkInInterval int, latestAgentVersion string, stuckCommandTimeout time.Duration, maxCommandRetries int) *AgentHandler {
	if stuckCommandTimeout <= 0 {
		stuckCommandTimeout = 5 * time.Minute
	}
	if maxCommandRetries <= 0 {
		maxCommandRetries = 5
	}
	return &AgentHandler{
		agentQueries:             aq,
		commandQueries:           cq,
		refreshTokenQueries:      rtq,
		registrationTokenQueries: regTokenQueries,
		subsystemQueries:         sq,
		scheduler:                scheduler,
		signingService:           signingService,
		securityLogger:           securityLogger,
		config:                   cfg,
		checkInInterval:          checkInInterval,
		latestAgentVersion:       latestAgentVersion,
		stuckCommandTimeout:      stuckCommandTimeout,
		maxCommandRetries:        maxCommandRetries,
	}
}

// signAndCreateCommand signs a command before storing.
// STRICT MODE: Commands without signatures are rejected (ETHOS #2 Security is Non-Negotiable)
//
// signAndCreateCommand signs a command and stores it for agent delivery.
// Rapid polling is signaled via the poll response flag (RapidPollingConfig),
// not as a separate command — the agent adjusts its interval immediately
// on receipt, no extra round-trip. Toggle via policy.auto_heartbeat_enabled
// in security_settings.
func (h *AgentHandler) signAndCreateCommand(cmd *models.AgentCommand) error {

	// STRICT MODE: If signing service disabled, FAIL FAST
	if h.signingService == nil || !h.signingService.IsEnabled() {
		err := fmt.Errorf("signing service not available - command rejected")
		log.Printf("[ERROR] [server] [signing] command_rejected reason=%q type=%s",
			err, cmd.CommandType)
		if h.securityLogger != nil {
			h.securityLogger.LogUnsignedCommandRejected(cmd)
		}
		return err // DO NOT store unsigned command
	}

	// Sign the command
	signature, err := h.signingService.SignCommand(cmd)
	if err != nil {
		log.Printf("[ERROR] [server] [signing] command_sign_failed error=%q type=%s",
			err, cmd.CommandType)
		return fmt.Errorf("failed to sign command: %w", err)
	}
	cmd.Signature = signature

	// Log successful signing
	if h.securityLogger != nil {
		h.securityLogger.LogCommandSigned(cmd)
	}

	// Store in database
	err = h.commandQueries.CreateCommand(cmd)
	if err != nil {
		return fmt.Errorf("failed to create command: %w", err)
	}

	log.Printf("[INFO] [server] [command] created_signed_command id=%s type=%s",
		cmd.ID, cmd.CommandType)
	return nil
}

// policyAutoHeartbeatEnabled reads policy.auto_heartbeat_enabled. Defaults to
// true if settings aren't wired or the row is missing — the safe default is
// "agent gets fast feedback on state changes."
func (h *AgentHandler) policyAutoHeartbeatEnabled() bool {
	if h.securitySettings == nil {
		return true
	}
	return h.securitySettings.GetPolicyBool("auto_heartbeat_enabled", true)
}


// scannerSubsystemDefaultIntervalMinutes returns the default scheduling interval
// for a per-scanner subsystem created mid-flight via ARC-001 capability advertisement.
// Mirrors registration defaults (RegisterAgent + CreateDefaultSubsystems).
func scannerSubsystemDefaultIntervalMinutes(scanner string) int {
	switch scanner {
	case "winget", "windows":
		return 60
	case "docker":
		return 15
	case "apt", "dnf":
		return 15
	default:
		return 60
	}
}

// syncAvailableScanners reconciles the agent's reported scanners with the
// agent_subsystems table and agent metadata. Idempotent (ETHOS #4): only
// creates rows for scanners that don't already have a subsystem row. Does NOT
// remove existing subsystems — a scanner being briefly unavailable (package
// manager lock, transient detection failure) shouldn't tear down schedules.
func (h *AgentHandler) syncAvailableScanners(agentID uuid.UUID, reported []string) {
	existing, err := h.subsystemQueries.GetSubsystems(agentID)
	if err != nil {
		log.Printf("[ERROR] [server] [arc-001] get_subsystems_failed agent_id=%s error=%q", agentID, err)
		return
	}

	have := make(map[string]bool, len(existing))
	for _, sub := range existing {
		have[sub.Subsystem] = true
	}

	added := []string{}
	for _, scanner := range reported {
		if have[scanner] {
			continue
		}
		sub := models.AgentSubsystem{
			AgentID:         agentID,
			Subsystem:       scanner,
			Enabled:         true,
			AutoRun:         true,
			IntervalMinutes: scannerSubsystemDefaultIntervalMinutes(scanner),
		}
		if err := h.subsystemQueries.CreateSubsystem(&sub); err != nil {
			log.Printf("[ERROR] [server] [arc-001] create_subsystem_failed agent_id=%s scanner=%s error=%q",
				agentID, scanner, err)
			continue
		}
		added = append(added, scanner)
	}

	if len(added) > 0 {
		log.Printf("[INFO] [server] [arc-001] subsystems_synced agent_id=%s added=%v reported=%v",
			agentID, added, reported)
	}

	// Persist the latest reported set into agent metadata. The scheduler
	// (ARC-002) reads this to skip platform-specific scan commands for
	// scanners that aren't on the agent right now, even if a stale
	// subsystem row is still enabled.
	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		log.Printf("[WARNING] [server] [arc-001] get_agent_for_metadata_failed agent_id=%s error=%q", agentID, err)
		return
	}
	if agent.Metadata == nil {
		agent.Metadata = make(models.JSONB)
	}
	scanners := make([]interface{}, 0, len(reported))
	for _, s := range reported {
		scanners = append(scanners, s)
	}
	agent.Metadata["available_scanners"] = scanners
	agent.Metadata["available_scanners_reported_at"] = time.Now().UTC().Format(time.RFC3339)
	if err := h.agentQueries.UpdateAgent(agent); err != nil {
		log.Printf("[WARNING] [server] [arc-001] update_agent_metadata_failed agent_id=%s error=%q", agentID, err)
	}
}

// RegisterAgent handles agent registration
func (h *AgentHandler) RegisterAgent(c *gin.Context) {
	var req models.AgentRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate registration token (critical security check)
	// Extract token from Authorization header or request body
	var registrationToken string

	// Try Authorization header first (Bearer token)
	if authHeader := c.GetHeader("Authorization"); authHeader != "" {
		if len(authHeader) > 7 && authHeader[:7] == "Bearer " {
			registrationToken = authHeader[7:]
		}
	}

	// If not in header, try request body (fallback)
	if registrationToken == "" && req.RegistrationToken != "" {
		registrationToken = req.RegistrationToken
	}

	// Reject if no registration token provided
	if registrationToken == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "registration token required"})
		return
	}

	// Validate the registration token
	tokenInfo, err := h.registrationTokenQueries.ValidateRegistrationToken(registrationToken)
	if err != nil || tokenInfo == nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired registration token"})
		return
	}

	// Validate machine ID and public key fingerprint if provided
	if req.MachineID != "" {
		// Check if machine ID is already registered to another agent. When this
		// fires from a re-run of the install URL on an already-registered host,
		// the operator should be using the install.sh upgrade-in-place path
		// (which reads the local refresh_token and skips this call). A 409 here
		// means either the local config was lost (Reclaim, not yet implemented)
		// or the operator genuinely wants a new identity on the same hardware
		// (Re-register — explicit dashboard action). See docs/AGENT_LIFECYCLE.md.
		existingAgent, err := h.agentQueries.GetAgentByMachineID(req.MachineID)
		if err == nil && existingAgent != nil && existingAgent.ID.String() != "" {
			c.JSON(http.StatusConflict, gin.H{
				"error":              "machine ID already registered to another agent",
				"existing_agent_id":  existingAgent.ID.String(),
				"existing_hostname":  existingAgent.Hostname,
				"existing_last_seen": existingAgent.LastSeen.UTC().Format(time.RFC3339),
				"remediation": "If this host already has /etc/redflag/agent/config.json with a refresh_token, the install URL upgrades in place — re-run it. If local config was lost, no Reclaim path exists yet; remove the existing agent from the dashboard to re-register fresh.",
			})
			return
		}
	}

	// Create new agent. Only set MachineID/PublicKeyFingerprint when non-empty;
	// the Agent model stores them as *string so nil maps to SQL NULL, avoiding
	// unique-index violations that &"" would cause on the machine_id partial
	// unique index WHERE machine_id IS NOT NULL.
	var machineID *string
	if req.MachineID != "" {
		machineID = &req.MachineID
	}
	var pubKeyFP *string
	if req.PublicKeyFingerprint != "" {
		pubKeyFP = &req.PublicKeyFingerprint
	}

	// Device classification (DEVICE-001). Unrecognized values degrade to the
	// conservative default rather than failing registration on the CHECK constraint.
	deviceType := req.DeviceType
	if !models.ValidDeviceType(deviceType) {
		deviceType = "server"
	}
	var deviceModel *string
	if req.DeviceModel != "" {
		deviceModel = &req.DeviceModel
	}
	var osDistro *string
	if req.OSDistro != "" {
		osDistro = &req.OSDistro
	}

	agent := &models.Agent{
		ID:                  uuid.Must(uuid.NewV4()),
		Hostname:            req.Hostname,
		OSType:              req.OSType,
		OSVersion:           req.OSVersion,
		OSArchitecture:      req.OSArchitecture,
		AgentVersion:        req.AgentVersion,
		CurrentVersion:      req.AgentVersion,
		MachineID:           machineID,
		PublicKeyFingerprint: pubKeyFP,
		DeviceType:          deviceType,
		DeviceModel:         deviceModel,
		OSDistro:            osDistro,
		LastSeen:            time.Now().UTC(),
		Status:              "online",
		Metadata:            models.JSONB{},
	}

	// Add metadata if provided
	if req.Metadata != nil {
		for k, v := range req.Metadata {
			agent.Metadata[k] = v
		}
	}

	// F-B2-1 fix: Wrap all DB operations in a single transaction.
	// If any step fails, the transaction rolls back atomically.
	tx, err := h.agentQueries.DB.Beginx()
	if err != nil {
		log.Printf("[ERROR] [server] [registration] transaction_begin_failed error=%q", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "registration failed"})
		return
	}
	defer tx.Rollback()

	// Step 1: Create agent in transaction
	createQuery := `
		INSERT INTO agents (
			id, hostname, os_type, os_version, os_architecture,
			agent_version, current_version, machine_id, public_key_fingerprint,
			device_type, device_model, os_distro,
			last_seen, status, metadata
		) VALUES (
			:id, :hostname, :os_type, :os_version, :os_architecture,
			:agent_version, :current_version, :machine_id, :public_key_fingerprint,
			:device_type, :device_model, :os_distro,
			:last_seen, :status, :metadata
		)`
	if _, err := tx.NamedExec(createQuery, agent); err != nil {
		log.Printf("[ERROR] [server] [registration] create_agent_failed error=%q", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register agent"})
		return
	}

	// Step 2: Mark registration token as used (via stored procedure — expects hash)
	var tokenSuccess bool
	tokenHash := queries.HashRegistrationToken(registrationToken)
	if err := tx.QueryRow("SELECT mark_registration_token_used($1, $2)", tokenHash, agent.ID).Scan(&tokenSuccess); err != nil || !tokenSuccess {
		log.Printf("[ERROR] [server] [registration] mark_token_failed error=%v success=%v", err, tokenSuccess)
		c.JSON(http.StatusBadRequest, gin.H{"error": "registration token could not be consumed"})
		return
	}

	// Step 3: Generate refresh token and store in transaction
	refreshToken, err := queries.GenerateRefreshToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate refresh token"})
		return
	}
	refreshTokenExpiry := time.Now().UTC().Add(90 * 24 * time.Hour)
	tokenHash = queries.HashRefreshToken(refreshToken)
	refreshFamilyID := uuid.Must(uuid.NewV4()) // root of this agent's rotation family (migration 045)
	if _, err := tx.Exec("INSERT INTO refresh_tokens (agent_id, token_hash, expires_at, family_id) VALUES ($1, $2, $3, $4)",
		agent.ID, tokenHash, refreshTokenExpiry, refreshFamilyID); err != nil {
		log.Printf("[ERROR] [server] [registration] create_refresh_token_failed error=%q", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store refresh token"})
		return
	}

	// Step 4: Create platform-specific subsystems based on agent's available scanners
	// This replaces the generic "updates" subsystem with specific ones (apt, dnf, winget, windows)
	if len(req.AvailableScanners) > 0 {
		for _, scanner := range req.AvailableScanners {
			intervalMinutes := 60 // Default 1 hour for update scanners
			sub := models.AgentSubsystem{
				AgentID:         agent.ID,
				Subsystem:       scanner, // apt, dnf, winget, windows, docker
				Enabled:         true,
				AutoRun:         true,
				IntervalMinutes: intervalMinutes,
			}
			if err := h.subsystemQueries.CreateSubsystemWithTx(tx, &sub); err != nil {
				log.Printf("[WARNING] [server] [registration] create_subsystem_failed scanner=%s error=%v", scanner, err)
				// Non-fatal - continue with other subsystems
			}
		}
		// Always add storage and system. Docker is intentionally omitted:
		// it's a scanner, not a generic subsystem, so it's only created
		// when the agent advertises "docker" via AvailableScanners (above).
		// ARC-005 closes the asymmetry by also dropping docker from the
		// fallback default-subsystems path so a docker row is never
		// fabricated for agents that don't actually have Docker.
		genericSubsystems := []struct {
			name     string
			interval int
		}{
			{"storage", 5},
			{"system", 5},
		}
		for _, gen := range genericSubsystems {
			sub := models.AgentSubsystem{
				AgentID:         agent.ID,
				Subsystem:       gen.name,
				Enabled:         true,
				AutoRun:         true,
				IntervalMinutes: gen.interval,
			}
			if err := h.subsystemQueries.CreateSubsystemWithTx(tx, &sub); err != nil {
				log.Printf("[WARNING] [server] [registration] create_generic_subsystem_failed subsystem=%s error=%v", gen.name, err)
			}
		}
	} else {
		// Fallback: create default subsystems if agent didn't report available scanners
		if err := h.subsystemQueries.CreateDefaultSubsystemsWithTx(tx, agent.ID, nil); err != nil {
			log.Printf("[WARNING] [server] [registration] create_default_subsystems_failed error=%v", err)
		}
	}

	// Commit transaction — all DB operations succeed or none do
	if err := tx.Commit(); err != nil {
		log.Printf("[ERROR] [server] [registration] transaction_commit_failed error=%q", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "registration failed"})
		return
	}

	// Generate JWT AFTER transaction commits (not inside transaction)
	token, err := middleware.GenerateAgentToken(agent.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// Return response with both tokens
	response := models.AgentRegistrationResponse{
		AgentID:      agent.ID,
		Token:        token,
		RefreshToken: refreshToken,
		Config: map[string]interface{}{
			"check_in_interval": h.checkInInterval,
			"server_url":        resolveServerURL(c, h.config, "registration"),
		},
	}

	c.JSON(http.StatusOK, response)
}

// GetCommands returns pending commands for an agent
// Agents can optionally send lightweight system metrics in request body
func (h *AgentHandler) GetCommands(c *gin.Context) {
	agentID := c.MustGet("agent_id").(uuid.UUID)

	// Try to parse optional system metrics from request body
	var metrics struct {
		CPUPercent             float64                `json:"cpu_percent,omitempty"`
		MemoryPercent          float64                `json:"memory_percent,omitempty"`
		MemoryUsedGB           float64                `json:"memory_used_gb,omitempty"`
		MemoryTotalGB          float64                `json:"memory_total_gb,omitempty"`
		DiskUsedGB             float64                `json:"disk_used_gb,omitempty"`
		DiskTotalGB            float64                `json:"disk_total_gb,omitempty"`
		DiskPercent            float64                `json:"disk_percent,omitempty"`
		Uptime                 string                 `json:"uptime,omitempty"`
		Metadata               map[string]interface{} `json:"metadata,omitempty"`
		PendingAcknowledgments []string               `json:"pending_acknowledgments,omitempty"`
		ReceivedCommandIDs     []string               `json:"received_command_ids,omitempty"` // Migration 033: command IDs the agent has received but not yet completed
		Version                string                 `json:"version,omitempty"`              // Agent's currently-running version; drives post-update IsUpdating clearance via TimeoutService
		AvailableScanners      []string               `json:"available_scanners,omitempty"`   // ARC-001: per-poll capability advertisement
	}

	// Parse metrics if provided (optional, won't fail if empty)
	err := c.ShouldBindJSON(&metrics)
	if err != nil {
		log.Printf("DEBUG: Failed to parse metrics JSON: %v", err)
	}

	// Process buffered events from agent if present
	if metrics.Metadata != nil {
		if bufferedEvents, exists := metrics.Metadata["buffered_events"]; exists {
			if events, ok := bufferedEvents.([]interface{}); ok && len(events) > 0 {
				stored := 0
				for _, e := range events {
					if eventMap, ok := e.(map[string]interface{}); ok {
						// Extract event fields with type safety
						eventType := getStringFromMap(eventMap, "event_type")
						eventSubtype := getStringFromMap(eventMap, "event_subtype")
						severity := getStringFromMap(eventMap, "severity")
						component := getStringFromMap(eventMap, "component")
						message := getStringFromMap(eventMap, "message")
						
						if eventType != "" && eventSubtype != "" && severity != "" {
							// metadata is optional and agent-supplied; don't trust its shape
							metadata, _ := eventMap["metadata"].(map[string]interface{})
							event := &models.SystemEvent{
								AgentID:      &agentID,
								EventType:    eventType,
								EventSubtype: eventSubtype,
								Severity:     severity,
								Component:    component,
								Message:      message,
								Metadata:     metadata,
								CreatedAt:    time.Now().UTC(),
							}
							
							if err := h.agentQueries.CreateSystemEvent(event); err != nil {
								log.Printf("Warning: Failed to store buffered event: %v", err)
							} else {
								stored++
							}
						}
					}
				}
				if stored > 0 {
					log.Printf("Stored %d buffered events from agent %s", stored, agentID)
				}
			}
		}
	}

	// Debug logging to see what we received
	log.Printf("DEBUG: Received metrics - Version: '%s', CPU: %.2f, Memory: %.2f",
		metrics.Version, metrics.CPUPercent, metrics.MemoryPercent)

	// Always handle version information if provided
	if metrics.Version != "" {
		// Update agent's current version in database (primary source of truth)
		if err := h.agentQueries.UpdateAgentVersion(agentID, metrics.Version); err != nil {
			log.Printf("Warning: Failed to update agent version: %v", err)
		} else {
			// Check if update is available
			updateAvailable := utils.IsNewerVersion(h.latestAgentVersion, metrics.Version)

			// Update agent's update availability status
			if err := h.agentQueries.UpdateAgentUpdateAvailable(agentID, updateAvailable); err != nil {
				log.Printf("Warning: Failed to update agent update availability: %v", err)
			}

			// Get current agent for logging and metadata update
			agent, err := h.agentQueries.GetAgentByID(agentID)
			if err == nil {
				// Log version check
				if updateAvailable {
					log.Printf("[INFO] [server] [agents]Agent %s (%s) version %s has update available: %s",
						agent.Hostname, agentID, metrics.Version, h.latestAgentVersion)
				} else {
					log.Printf("[INFO] [server] [agents]Agent %s (%s) version %s is up to date",
						agent.Hostname, agentID, metrics.Version)
				}

				// Store version in metadata as well (for backwards compatibility)
				// Initialize metadata if nil
				if agent.Metadata == nil {
					agent.Metadata = make(models.JSONB)
				}
				agent.Metadata["reported_version"] = metrics.Version
				agent.Metadata["latest_version"] = h.latestAgentVersion
				agent.Metadata["update_available"] = updateAvailable
				agent.Metadata["version_checked_at"] = time.Now().UTC().Format(time.RFC3339)

				// Update agent metadata
				if err := h.agentQueries.UpdateAgent(agent); err != nil {
					log.Printf("Warning: Failed to update agent metadata: %v", err)
				}

				// Confirm a completed self-upgrade. The agent reporting a version at
				// or past the target it was updating toward is the authoritative
				// success signal — the new binary booted and checked in. Close the
				// loop here instead of waiting for the TimeoutService sweep: clear
				// is_updating, mark the update_agent command completed, emit a History
				// event. current_version was already persisted above.
				if agent.IsUpdating && agent.UpdatingToVersion != nil &&
					utils.IsNewerOrEqualVersion(metrics.Version, *agent.UpdatingToVersion) {
					target := *agent.UpdatingToVersion
					if err := h.agentQueries.ClearAgentUpdating(agentID); err != nil {
						log.Printf("Warning: failed to clear is_updating for agent %s: %v", agentID, err)
					} else {
						h.confirmUpdateCommand(agentID, metrics.Version)
						log.Printf("[INFO] [server] [agents] update_confirmed agent=%s version=%s target=%s",
							agent.Hostname, metrics.Version, target)
					}
				}
			}
		}
	}

	// Update agent metadata with current metrics if provided
	if metrics.CPUPercent > 0 || metrics.MemoryPercent > 0 || metrics.DiskUsedGB > 0 || metrics.Uptime != "" {
		// Get current agent to preserve existing metadata
		agent, err := h.agentQueries.GetAgentByID(agentID)
		if err == nil && agent.Metadata != nil {
			// Update metrics in metadata
			agent.Metadata["cpu_percent"] = metrics.CPUPercent
			agent.Metadata["memory_percent"] = metrics.MemoryPercent
			agent.Metadata["memory_used_gb"] = metrics.MemoryUsedGB
			agent.Metadata["memory_total_gb"] = metrics.MemoryTotalGB
			agent.Metadata["disk_used_gb"] = metrics.DiskUsedGB
			agent.Metadata["disk_total_gb"] = metrics.DiskTotalGB
			agent.Metadata["disk_percent"] = metrics.DiskPercent
			agent.Metadata["uptime"] = metrics.Uptime
			agent.Metadata["metrics_updated_at"] = time.Now().UTC().Format(time.RFC3339)

			// Process heartbeat metadata from agent check-ins
			if metrics.Metadata != nil {
				if rapidPollingEnabled, ok := metrics.Metadata["rapid_polling_enabled"].(bool); ok {
					if rapidPollingUntil, ok := metrics.Metadata["rapid_polling_until"].(string); ok {
						// Parse the until timestamp
						if untilTime, err := time.Parse(time.RFC3339, rapidPollingUntil); err == nil {
							// Validate if rapid polling is still active (not expired)
							isActive := rapidPollingEnabled && time.Now().UTC().Before(untilTime)

							// Store heartbeat status in agent metadata
							agent.Metadata["rapid_polling_enabled"] = rapidPollingEnabled
							agent.Metadata["rapid_polling_until"] = rapidPollingUntil
							agent.Metadata["rapid_polling_active"] = isActive

							log.Printf("[Heartbeat] Agent %s heartbeat status: enabled=%v, until=%v, active=%v",
								agentID, rapidPollingEnabled, rapidPollingUntil, isActive)
						} else {
							log.Printf("[Heartbeat] Failed to parse rapid_polling_until timestamp for agent %s: %v", agentID, err)
						}
					}
				}
			}

			// Update agent with new metadata (preserve version tracking)
			if err := h.agentQueries.UpdateAgentMetadata(agentID, agent.Metadata, agent.Status, time.Now().UTC()); err != nil {
				log.Printf("Warning: Failed to update agent metrics: %v", err)
			}
		}
	}

	// Update last_seen
	if err := h.agentQueries.UpdateAgentLastSeen(agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update last seen"})
		return
	}

	// ARC-001: capability advertisement during check-in.
	// Idempotently sync per-scanner subsystems against what the agent reports
	// it has right now, and stash the list in agent metadata so the scheduler
	// (ARC-002) can filter out commands for scanners that aren't present.
	if len(metrics.AvailableScanners) > 0 {
		h.syncAvailableScanners(agentID, metrics.AvailableScanners)
	}

	// Process heartbeat metadata from agent check-ins
	if metrics.Metadata != nil {
		agent, err := h.agentQueries.GetAgentByID(agentID)
		if err == nil && agent.Metadata != nil {
			if rapidPollingEnabled, ok := metrics.Metadata["rapid_polling_enabled"].(bool); ok {
				if rapidPollingUntil, ok := metrics.Metadata["rapid_polling_until"].(string); ok {
					// Parse the until timestamp
					if untilTime, err := time.Parse(time.RFC3339, rapidPollingUntil); err == nil {
						// Validate if rapid polling is still active (not expired)
						isActive := rapidPollingEnabled && time.Now().UTC().Before(untilTime)

						// Store heartbeat status in agent metadata
						agent.Metadata["rapid_polling_enabled"] = rapidPollingEnabled
						agent.Metadata["rapid_polling_until"] = rapidPollingUntil
						agent.Metadata["rapid_polling_active"] = isActive

						log.Printf("[Heartbeat] Agent %s heartbeat status: enabled=%v, until=%v, active=%v",
							agentID, rapidPollingEnabled, rapidPollingUntil, isActive)

						// Update agent with new metadata
						if err := h.agentQueries.UpdateAgent(agent); err != nil {
							log.Printf("[Heartbeat] Warning: Failed to update agent heartbeat metadata: %v", err)
						}
					} else {
						log.Printf("[Heartbeat] Failed to parse rapid_polling_until timestamp for agent %s: %v", agentID, err)
					}
				}
			}
		}
	}

	// Check for version updates for agents that don't send version in metrics
	// This ensures agents like Metis that don't report version still get update checks
	if metrics.Version == "" {
		// Get current agent to check version
		agent, err := h.agentQueries.GetAgentByID(agentID)
		if err == nil && agent.CurrentVersion != "" {
			// Check if update is available based on stored version
			updateAvailable := utils.IsNewerVersion(h.latestAgentVersion, agent.CurrentVersion)

			// Update agent's update availability status if it changed
			if agent.UpdateAvailable != updateAvailable {
				if err := h.agentQueries.UpdateAgentUpdateAvailable(agentID, updateAvailable); err != nil {
					log.Printf("Warning: Failed to update agent update availability: %v", err)
				} else {
					// Log version check for agent without version reporting
					if updateAvailable {
						log.Printf("[INFO] [server] [agents]Agent %s (%s) stored version %s has update available: %s",
							agent.Hostname, agentID, agent.CurrentVersion, h.latestAgentVersion)
					} else {
						log.Printf("[INFO] [server] [agents]Agent %s (%s) stored version %s is up to date",
							agent.Hostname, agentID, agent.CurrentVersion)
					}
				}
			}
		}
	}

	// F-B2-2 fix: Atomic command delivery with SELECT FOR UPDATE SKIP LOCKED
	// Prevents concurrent requests from delivering the same commands
	cmdTx, err := h.commandQueries.DB().Beginx()
	if err != nil {
		log.Printf("[ERROR] [server] [command] transaction_begin_failed agent_id=%s error=%v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve commands"})
		return
	}
	defer cmdTx.Rollback()

	// Post-update version attestation: if the agent reports a version, record it. The
	// IsUpdating flag clearance is handled by TimeoutService once it observes
	// current_version == updating_to_version (avoids per-request side effects).
	if metrics.Version != "" {
		if err := h.agentQueries.UpdateAgentVersion(agentID, metrics.Version); err != nil {
			log.Printf("[ERROR] [server] [command] update_version_failed agent_id=%s version=%s error=%v",
				agentID, metrics.Version, err)
		}
	}

	// Migration 033 §2: transition agent-reported sent→received BEFORE looking for stuck
	// commands. Excludes those IDs from the GetStuckCommandsTx re-issuance candidate set,
	// which now only sees 'pending' and 'sent'. Receipt confirmation goes back in the
	// response so the agent can drop them from its outbox.
	var receiptConfirmedIDs []string
	var confirmed []string // commands the server has marked completed (via ReportLog)
	if len(metrics.ReceivedCommandIDs) > 0 {
		var markErr error
		confirmed, markErr = h.commandQueries.MarkCommandsReceivedTx(cmdTx, agentID, metrics.ReceivedCommandIDs)
		if markErr != nil {
			log.Printf("[ERROR] [server] [command] mark_received_failed agent_id=%s reported=%d error=%v",
				agentID, len(metrics.ReceivedCommandIDs), markErr)
		} else {
			receiptConfirmedIDs = confirmed
			if len(confirmed) > 0 {
				log.Printf("[INFO] [server] [command] commands_received agent_id=%s confirmed=%d reported=%d",
					agentID, len(confirmed), len(metrics.ReceivedCommandIDs))
			}
		}
	} else {
		// No ReceivedCommandIDs — start with empty confirmed set
		confirmed = []string{}
	}

	// Get pending commands with row-level lock
	pendingCommands, err := h.commandQueries.GetPendingCommandsTx(cmdTx, agentID)
	if err != nil {
		log.Printf("[ERROR] [server] [command] get_pending_failed agent_id=%s error=%v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve commands"})
		return
	}

	// Recover stuck commands with row-level lock (excludes 'received' — migration 033)
	stuckCommands, err := h.commandQueries.GetStuckCommandsTx(cmdTx, agentID, h.stuckCommandTimeout, h.maxCommandRetries)
	if err != nil {
		log.Printf("[WARNING] [server] [command] get_stuck_failed agent_id=%s error=%v", agentID, err)
	}

	// Convert to response format and mark as sent within the transaction
	allCommands := append(pendingCommands, stuckCommands...)
	commandItems := make([]models.CommandItem, 0, len(allCommands))

	// Mark pending commands as sent (first delivery — retry_count stays at 0)
	for _, cmd := range pendingCommands {
		createdAt := cmd.CreatedAt
		commandItems = append(commandItems, models.CommandItem{
			ID:        cmd.ID.String(),
			Type:      cmd.CommandType,
			Params:    cmd.Params,
			Signature: cmd.Signature,
			KeyID:     cmd.KeyID,
			SignedAt:  cmd.SignedAt,
			AgentID:   cmd.AgentID.String(),
			CreatedAt: &createdAt,
		})
		if err := h.commandQueries.MarkCommandSentTx(cmdTx, cmd.ID); err != nil {
			log.Printf("[ERROR] [server] [command] mark_sent_failed command_id=%s error=%v", cmd.ID, err)
		}
	}

	// Re-deliver stuck commands (increments retry_count — DEV-029 fix)
	for _, cmd := range stuckCommands {
		createdAt := cmd.CreatedAt
		commandItems = append(commandItems, models.CommandItem{
			ID:        cmd.ID.String(),
			Type:      cmd.CommandType,
			Params:    cmd.Params,
			Signature: cmd.Signature,
			KeyID:     cmd.KeyID,
			SignedAt:  cmd.SignedAt,
			AgentID:   cmd.AgentID.String(),
			CreatedAt: &createdAt,
		})
		if err := h.commandQueries.RedeliverStuckCommandTx(cmdTx, cmd.ID); err != nil {
			log.Printf("[ERROR] [server] [command] redeliver_stuck_failed command_id=%s error=%v", cmd.ID, err)
		}
	}

	// Commit the transaction — releases locks
	if err := cmdTx.Commit(); err != nil {
		log.Printf("[ERROR] [server] [command] transaction_commit_failed agent_id=%s error=%v", agentID, err)
	}

	// Log command retrieval for audit trail
	if len(allCommands) > 0 {
		log.Printf("[INFO] [server] [command] retrieved_commands agent_id=%s count=%d timestamp=%s",
			agentID, len(allCommands), time.Now().UTC().Format(time.RFC3339))
		log.Printf("[HISTORY] [server] [command] retrieved_commands agent_id=%s count=%d timestamp=%s",
			agentID, len(allCommands), time.Now().UTC().Format(time.RFC3339))
	}

	// Check if rapid polling should be enabled
	var rapidPolling *models.RapidPollingConfig

	// Enable rapid polling if there are commands to process and policy allows it.
	// The response flag tells the agent to adjust its interval immediately —
	// no separate enable_heartbeat command needed. Source tracking (system vs
	// manual) is preserved via heartbeat_source in agent metadata.
	if len(commandItems) > 0 && h.policyAutoHeartbeatEnabled() {
		rapidPolling = &models.RapidPollingConfig{
			Enabled: true,
			Until:   time.Now().UTC().Add(10 * time.Minute).Format(time.RFC3339), // 10 minutes default
		}
		// Record source so the UI can distinguish system-triggered from manual
		if agent, err := h.agentQueries.GetAgentByID(agentID); err == nil {
			if agent.Metadata == nil {
				agent.Metadata = models.JSONB{}
			}
			agent.Metadata["heartbeat_source"] = models.CommandSourceSystem
			agent.Metadata["rapid_polling_enabled"] = true
			agent.Metadata["rapid_polling_until"] = rapidPolling.Until
			if err := h.agentQueries.UpdateAgent(agent); err != nil {
				log.Printf("[ERROR] [server] [agents] rapid_polling_metadata_write_failed agent_id=%s error=%v", agentID, err)
			}
		}
	} else {
		// Check if agent has rapid polling already configured in metadata
		agent, err := h.agentQueries.GetAgentByID(agentID)
		if err == nil && agent.Metadata != nil {
			if enabled, ok := agent.Metadata["rapid_polling_enabled"].(bool); ok && enabled {
				if untilStr, ok := agent.Metadata["rapid_polling_until"].(string); ok {
					if until, err := time.Parse(time.RFC3339, untilStr); err == nil && time.Now().UTC().Before(until) {
						rapidPolling = &models.RapidPollingConfig{
							Enabled: true,
							Until:   untilStr,
						}
					}
				}
			}
		}
	}

	// Detect stale heartbeat state: Server thinks it's active, but agent didn't report it
	// This happens when agent restarts without heartbeat mode
	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err == nil && agent.Metadata != nil {
		// Check if server metadata shows heartbeat active
		if serverEnabled, ok := agent.Metadata["rapid_polling_enabled"].(bool); ok && serverEnabled {
			if untilStr, ok := agent.Metadata["rapid_polling_until"].(string); ok {
				if until, err := time.Parse(time.RFC3339, untilStr); err == nil && time.Now().UTC().Before(until) {
					// Server thinks heartbeat is active and not expired
					// Check if agent is reporting heartbeat in this check-in
					agentReportingHeartbeat := false
					if metrics.Metadata != nil {
						if agentEnabled, ok := metrics.Metadata["rapid_polling_enabled"].(bool); ok {
							agentReportingHeartbeat = agentEnabled
						}
					}

					// If agent is NOT reporting heartbeat but server expects it → stale state
					if !agentReportingHeartbeat {
						log.Printf("[Heartbeat] Stale heartbeat detected for agent %s - server expected active until %s, but agent not reporting heartbeat (likely restarted)",
							agentID, until.Format(time.RFC3339))

						// Clear stale heartbeat state
						agent.Metadata["rapid_polling_enabled"] = false
						delete(agent.Metadata, "rapid_polling_until")

						if err := h.agentQueries.UpdateAgent(agent); err != nil {
							log.Printf("[Heartbeat] Warning: Failed to clear stale heartbeat state: %v", err)
						} else {
							log.Printf("[Heartbeat] Cleared stale heartbeat state for agent %s", agentID)

							// Create audit command to show in history
							now := time.Now().UTC()
							auditCmd := &models.AgentCommand{
								ID:          uuid.Must(uuid.NewV4()),
								AgentID:     agentID,
								CommandType: models.CommandTypeDisableHeartbeat,
								Params:      models.JSONB{},
								Status:      models.CommandStatusCompleted,
								Source:      models.CommandSourceSystem,
								Result: models.JSONB{
									"message": "Heartbeat cleared - agent restarted without active heartbeat mode",
								},
								CreatedAt:   now,
								SentAt:      &now,
								CompletedAt: &now,
							}

							if err := h.signAndCreateCommand(auditCmd); err != nil {
								log.Printf("[Heartbeat] Warning: Failed to create audit command for stale heartbeat: %v", err)
							} else {
								log.Printf("[Heartbeat] Created audit trail for stale heartbeat cleanup (agent %s)", agentID)
							}
						}

						// Clear rapidPolling response since we just disabled it
						rapidPolling = nil
					}
				}
			}
		}
	}

	// Process command acknowledgments from agent
	var acknowledgedIDs []string
	if len(metrics.PendingAcknowledgments) > 0 {
		// Debug: Check what commands exist for this agent
		agentCommands, err := h.commandQueries.GetCommandsByAgentID(agentID)
		if err != nil {
			log.Printf("DEBUG: Failed to get commands for agent %s: %v", agentID, err)
		} else {
			log.Printf("DEBUG: Agent %s has %d total commands in database", agentID, len(agentCommands))
			for _, cmd := range agentCommands {
				if cmd.Status == "completed" || cmd.Status == "failed" || cmd.Status == "timed_out" {
					log.Printf("DEBUG: Completed command found - ID: %s, Status: %s, Type: %s", cmd.ID, cmd.Status, cmd.CommandType)
				}
			}
		}

		log.Printf("DEBUG: Processing %d pending acknowledgments for agent %s: %v", len(metrics.PendingAcknowledgments), agentID, metrics.PendingAcknowledgments)
		// Ack every command whose result the server has durably recorded, regardless
		// of its lifecycle status — that receipt is all the agent's pending ack waits on.
		verified, err := h.commandQueries.VerifyResultsRecorded(metrics.PendingAcknowledgments)
		if err != nil {
			log.Printf("Warning: Failed to verify command acknowledgments for agent %s: %v", agentID, err)
		} else {
			acknowledgedIDs = verified
			log.Printf("DEBUG: Verified %d completed commands out of %d pending for agent %s", len(acknowledgedIDs), len(metrics.PendingAcknowledgments), agentID)
			if len(acknowledgedIDs) > 0 {
				log.Printf("Acknowledged %d command results for agent %s", len(acknowledgedIDs), agentID)
			}
		}
	}

	response := models.CommandsResponse{
		Commands:             commandItems,
		RapidPolling:         rapidPolling,
		AcknowledgedIDs:      acknowledgedIDs,
		ReceiptConfirmedIDs:  receiptConfirmedIDs,
		ConfirmedCommandIDs:  confirmed,
	}

	c.JSON(http.StatusOK, response)
}

// ListAgents returns all agents with last scan information
func (h *AgentHandler) ListAgents(c *gin.Context) {
	status := c.Query("status")
	osType := c.Query("os_type")

	agents, err := h.agentQueries.ListAgentsWithLastScan(status, osType)
	if err != nil {
		log.Printf("ERROR: Failed to list agents: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list agents - database error"})
		return
	}

	// Debug: Log what we're returning
	for _, agent := range agents {
		log.Printf("DEBUG: Returning agent %s: last_seen=%s, last_scan=%s", agent.Hostname, agent.LastSeen, agent.LastScan)
	}

	c.JSON(http.StatusOK, gin.H{
		"agents": agents,
		"total":  len(agents),
	})
}

// GetAgent returns a single agent by ID with last scan information
func (h *AgentHandler) GetAgent(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	agent, err := h.agentQueries.GetAgentWithLastScan(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	c.JSON(http.StatusOK, agent)
}

// TriggerHeartbeat creates a heartbeat toggle command for an agent
func (h *AgentHandler) TriggerHeartbeat(c *gin.Context) {
	idStr := c.Param("id")
	agentID, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	var request struct {
		Enabled        bool `json:"enabled"`
		DurationMinutes int  `json:"duration_minutes"`
	}

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Determine command type based on enabled flag
	commandType := models.CommandTypeDisableHeartbeat
	if request.Enabled {
		commandType = models.CommandTypeEnableHeartbeat
	}

	// Create heartbeat command with duration parameter (manual = user-initiated)
	cmd := &models.AgentCommand{
		ID:          uuid.Must(uuid.NewV4()),
		AgentID:     agentID,
		CommandType: commandType,
		Params: models.JSONB{
			"duration_minutes": request.DurationMinutes,
		},
		Status: models.CommandStatusPending,
		Source: models.CommandSourceManual,
	}

	if err := h.signAndCreateCommand(cmd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create heartbeat command"})
		return
	}

	// Store heartbeat source in agent metadata immediately
	if request.Enabled {
		agent, err := h.agentQueries.GetAgentByID(agentID)
		if err == nil {
			if agent.Metadata == nil {
				agent.Metadata = models.JSONB{}
			}
			agent.Metadata["heartbeat_source"] = models.CommandSourceManual
			if err := h.agentQueries.UpdateAgent(agent); err != nil {
				log.Printf("Warning: Failed to update agent metadata with heartbeat source: %v", err)
			}
		}
	}

	action := "disabled"
	if request.Enabled {
		action = "enabled"
	}

	log.Printf("[Heartbeat] Manual heartbeat %s command created for agent %s (duration: %d minutes)",
		action, agentID, request.DurationMinutes)

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("heartbeat %s command sent", action),
		"command_id": cmd.ID,
		"enabled": request.Enabled,
	})
}

// GetHeartbeatStatus returns the current heartbeat status for an agent.
func (h *AgentHandler) GetHeartbeatStatus(c *gin.Context) {
	idStr := c.Param("id")
	agentID, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Get agent and their heartbeat metadata
	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Extract heartbeat information from metadata
	response := gin.H{
		"enabled": false,
		"until": nil,
		"active": false,
		"duration_minutes": 0,
		"source": nil,
	}

	if agent.Metadata != nil {
		// Check if heartbeat is enabled in metadata
		if enabled, ok := agent.Metadata["rapid_polling_enabled"].(bool); ok {
			response["enabled"] = enabled

			// If enabled, get the until time and check if still active
			if enabled {
				if untilStr, ok := agent.Metadata["rapid_polling_until"].(string); ok {
					response["until"] = untilStr

					// Parse the until timestamp to check if still active
					if untilTime, err := time.Parse(time.RFC3339, untilStr); err == nil {
						response["active"] = time.Now().UTC().Before(untilTime)
					}
				}

				// Get duration if available
				if duration, ok := agent.Metadata["rapid_polling_duration_minutes"].(float64); ok {
					response["duration_minutes"] = duration
				}

				// Get source if available
				if source, ok := agent.Metadata["heartbeat_source"].(string); ok {
					response["source"] = source
				}
			}
		}
	}

	// Source of truth is agent metadata (rapid_polling_enabled,
	// rapid_polling_until, heartbeat_source), written by the poll
	// response path and the manual toggle. No command-table fallback —
	// that created split-brain when an older manual command overrode
	// a newer system heartbeat in metadata.

	c.JSON(http.StatusOK, response)
}

// TriggerUpdate creates an update command for an agent
func (h *AgentHandler) TriggerUpdate(c *gin.Context) {
	idStr := c.Param("id")
	agentID, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	var req struct {
		PackageType string `json:"package_type"` // "system", "docker", or specific type
		PackageName string `json:"package_name"` // optional specific package
		Action      string `json:"action"`       // "update_all", "update_approved", or "update_package"
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request format"})
		return
	}

	// Validate action
	validActions := map[string]bool{
		"update_all":        true,
		"update_approved":   true,
		"update_package":    true,
	}
	if !validActions[req.Action] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid action. Use: update_all, update_approved, or update_package"})
		return
	}

	// Create parameters for the command
	params := models.JSONB{
		"action":       req.Action,
		"package_type": req.PackageType,
	}
	if req.PackageName != "" {
		params["package_name"] = req.PackageName
	}

	// Create update command
	cmd := &models.AgentCommand{
		ID:          uuid.Must(uuid.NewV4()),
		AgentID:     agentID,
		CommandType: models.CommandTypeInstallUpdate,
		Params:      params,
		Status:      models.CommandStatusPending,
		Source:      models.CommandSourceManual,
	}

	if err := h.signAndCreateCommand(cmd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create update command"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "update command sent to agent",
		"command_id": cmd.ID,
		"action":     req.Action,
		"package":    req.PackageName,
	})
}

// RenewToken handles token renewal using refresh token
func (h *AgentHandler) RenewToken(c *gin.Context) {
	var req models.TokenRenewalRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// F-B2-9 fix: Wrap validate + update in a transaction
	renewTx, err := h.agentQueries.DB.Beginx()
	if err != nil {
		log.Printf("[ERROR] [server] [auth] renewal_transaction_begin_failed error=%q", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token renewal failed"})
		return
	}
	defer renewTx.Rollback()

		// Look up the presented token inside the transaction with FOR UPDATE to
		// serialize concurrent renewals. Returns the full rotation state (consumed_at,
		// superseded_by, family_id) so the handler can distinguish first-use, grace
		// recovery, and reuse from a single row.
	refreshToken, err := h.refreshTokenQueries.GetRefreshTokenForRenew(renewTx, req.AgentID, req.RefreshToken)
	if err != nil {
		log.Printf("[WARNING] [server] [auth] token_renewal_failed agent_id=%s error=%v", req.AgentID, err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}

	// Bind renewal to the machine the agent registered on, exactly as the command
	// endpoints do. The refresh token is a long-lived secret sitting on the agent's
	// disk; without this check it mints access tokens from *any* machine, so a
	// stolen config.json works anywhere for 90 days. A mismatch is a copied-identity
	// signal, not a transient error — we reject and surface it before sliding the
	// window or minting a token.
	reportedMachineID := c.GetHeader("X-Machine-ID")
	if reportedMachineID == "" {
		log.Printf("[WARNING] [server] [auth] renew_missing_machine_id agent_id=%s", req.AgentID)
		c.JSON(http.StatusForbidden, gin.H{"error": "missing machine ID header"})
		return
	}

	agent, err := h.agentQueries.GetAgentByID(req.AgentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	if agent.MachineID == nil {
		log.Printf("[WARNING] [server] [auth] renew_agent_unbound agent_id=%s", req.AgentID)
		c.JSON(http.StatusForbidden, gin.H{"error": "agent not bound to machine - re-registration required"})
		return
	}
	if *agent.MachineID != reportedMachineID {
		log.Printf("[WARNING] [server] [auth] renew_machine_id_mismatch agent=%s (%s) db=%s reported=%s",
			agent.Hostname, req.AgentID, *agent.MachineID, reportedMachineID)
		if h.securityLogger != nil {
			h.securityLogger.LogMachineIDMismatch(req.AgentID, *agent.MachineID, reportedMachineID)
		}
		c.JSON(http.StatusForbidden, gin.H{
			"error":         "machine ID mismatch - refresh token presented from a different machine",
			"security_note": "renewal is bound to the registered machine; this prevents stolen-token replay",
		})
		return
	}

	// --- Refresh-token rotation + reuse detection (migration 045) ---
	// familyID groups the rotation chain; fall back to the token's own id for
	// pre-rotation tokens that predate the 045 backfill.
	familyID := refreshToken.ID
	if refreshToken.FamilyID != nil {
		familyID = *refreshToken.FamilyID
	}

	// revokeFamily kills every token in the chain, commits, and logs a security
	// event. Used on reuse/theft and on any revoked-token replay against a live
	// family. Fail-closed and loud — both the legitimate agent and a thief lose
	// access, forcing a deliberate human re-registration.
	revokeFamily := func(reason string) {
		if _, e := renewTx.Exec("UPDATE refresh_tokens SET revoked = true WHERE family_id = $1", familyID); e != nil {
			log.Printf("[ERROR] [server] [auth] family_revoke_failed family_id=%s error=%q", familyID, e)
		}
		if e := renewTx.Commit(); e != nil {
			log.Printf("[ERROR] [server] [auth] family_revoke_commit_failed family_id=%s error=%q", familyID, e)
		}
		log.Printf("[WARNING] [server] [auth] refresh_token_family_revoked agent_id=%s family_id=%s reason=%s", req.AgentID, familyID, reason)
		if h.securityLogger != nil {
			h.securityLogger.LogUnauthorizedAccessAttempt(c.ClientIP(), "/api/v1/agents/renew", "refresh_token_reuse: "+reason, req.AgentID)
		}
	}

	// Expiry check: the query returns non-revoked tokens, but the expiry
	// could lapse between fetch and the rotation below.
	if time.Now().UTC().After(refreshToken.ExpiresAt) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token expired"})
		return
	}

	// mintSuccessor inserts a fresh token into the family and returns its plaintext + id.
	mintSuccessor := func() (string, uuid.UUID, error) {
		plain, gErr := queries.GenerateRefreshToken()
		if gErr != nil {
			return "", uuid.Nil, gErr
		}
		exp := time.Now().UTC().Add(90 * 24 * time.Hour)
		var newID uuid.UUID
		if sErr := renewTx.QueryRowx(
			`INSERT INTO refresh_tokens (agent_id, token_hash, expires_at, family_id)
			 VALUES ($1, $2, $3, $4) RETURNING id`,
			req.AgentID, queries.HashRefreshToken(plain), exp, familyID,
		).Scan(&newID); sErr != nil {
			return "", uuid.Nil, sErr
		}
		return plain, newID, nil
	}

	var newRefreshToken string
	if refreshToken.ConsumedAt == nil {
		// First use of this token — normal rotation: mint a successor and mark spent.
		plain, newID, mErr := mintSuccessor()
		if mErr != nil {
			log.Printf("[ERROR] [server] [auth] mint_successor_failed error=%q", mErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "token renewal failed"})
			return
		}
		if _, uErr := renewTx.Exec(
			"UPDATE refresh_tokens SET consumed_at = NOW(), superseded_by = $1, last_used_at = NOW() WHERE id = $2",
			newID, refreshToken.ID); uErr != nil {
			log.Printf("[ERROR] [server] [auth] consume_token_failed error=%q", uErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "token renewal failed"})
			return
		}
		newRefreshToken = plain
	} else {
		// Replay of an already-consumed token. Inspect its successor to tell a
		// benign crash-before-save retry from genuine reuse.
		if refreshToken.SupersededBy == nil {
			revokeFamily("consumed_without_successor")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token reuse detected"})
			return
		}
		var successor queries.RefreshToken
		succErr := renewTx.Get(&successor,
			`SELECT id, agent_id, token_hash, expires_at, created_at, last_used_at, revoked,
			        family_id, superseded_by, consumed_at
			 FROM refresh_tokens WHERE id = $1`, *refreshToken.SupersededBy)
		if succErr != nil || successor.Revoked || successor.ConsumedAt != nil {
			// Successor is missing, revoked, or already consumed → the chain advanced
			// past this token without us. Reuse of a superseded token: revoke family.
			revokeFamily("successor_consumed_or_missing")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "refresh token reuse detected"})
			return
		}
		// Successor exists and is still unconsumed → the agent rotated but crashed
		// before persisting the new token (it provably never used the successor).
		// Accept-previous-once: orphan that unsaved leaf, mint a fresh one, re-point.
		if _, rErr := renewTx.Exec("UPDATE refresh_tokens SET revoked = true WHERE id = $1", successor.ID); rErr != nil {
			log.Printf("[ERROR] [server] [auth] grace_orphan_failed error=%q", rErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "token renewal failed"})
			return
		}
		plain, newID, mErr := mintSuccessor()
		if mErr != nil {
			log.Printf("[ERROR] [server] [auth] grace_mint_failed error=%q", mErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "token renewal failed"})
			return
		}
		if _, uErr := renewTx.Exec(
			"UPDATE refresh_tokens SET superseded_by = $1, last_used_at = NOW() WHERE id = $2",
			newID, refreshToken.ID); uErr != nil {
			log.Printf("[ERROR] [server] [auth] grace_repoint_failed error=%q", uErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "token renewal failed"})
			return
		}
		log.Printf("[WARNING] [server] [auth] refresh_token_grace_reissue agent_id=%s family_id=%s reason=crash_before_save_recovery", req.AgentID, familyID)
		newRefreshToken = plain
	}

	if err := renewTx.Commit(); err != nil {
		log.Printf("[ERROR] [server] [auth] renewal_commit_failed error=%q", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token renewal failed"})
		return
	}

	// Update agent last_seen timestamp
	if err := h.agentQueries.UpdateAgentLastSeen(req.AgentID); err != nil {
		log.Printf("[WARNING] [server] [auth] update_last_seen_failed agent_id=%s error=%v", req.AgentID, err)
	}

	// Update agent version if provided
	if req.AgentVersion != "" {
		if err := h.agentQueries.UpdateAgentVersion(req.AgentID, req.AgentVersion); err != nil {
			log.Printf("[WARNING] [server] [auth] update_version_failed agent_id=%s error=%v", req.AgentID, err)
		}
	}

	// Generate new access token (24 hours)
	token, err := middleware.GenerateAgentToken(req.AgentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	log.Printf("[INFO] [server] [agents]Token renewed successfully for agent %s (%s)", agent.Hostname, req.AgentID)

	// Return new access token + the rotated refresh token. The agent must persist
	// the refresh token; if it crashes before doing so, the accept-previous-once
	// grace above lets it recover on the next attempt.
	response := models.TokenRenewalResponse{
		Token:        token,
		RefreshToken: newRefreshToken,
	}

	c.JSON(http.StatusOK, response)
}

// RebindMachineID updates an agent's stored machine ID (F-D1-2 admin endpoint).
// Requires WebAuthMiddleware + RequireAdmin. Used when hardware changes or
// to recover from the old "unknown-" fallback machine ID bug (F-D1-1).
func (h *AgentHandler) RebindMachineID(c *gin.Context) {
	idStr := c.Param("id")
	agentID, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	var req struct {
		NewMachineID string `json:"new_machine_id" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate: must be exactly 64 hex characters (SHA256 hash)
	if len(req.NewMachineID) != 64 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "machine_id must be exactly 64 hex characters"})
		return
	}
	for _, ch := range req.NewMachineID {
		if !((ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f')) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "machine_id must contain only lowercase hex characters [0-9a-f]"})
			return
		}
	}

	// Get current agent for logging
	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	oldMachineID := ""
	if agent.MachineID != nil {
		oldMachineID = *agent.MachineID
	}

	// Update machine ID
	if err := h.agentQueries.UpdateMachineID(agentID, req.NewMachineID); err != nil {
		log.Printf("[ERROR] [server] [admin] rebind_machine_id_failed agent_id=%s error=%q", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update machine ID"})
		return
	}

	adminUserID := c.GetString("user_id")
	log.Printf("[INFO] [server] [admin] agent_machine_id_updated agent_id=%s old_id=%s new_id=%s admin_user=%s",
		agentID, oldMachineID, req.NewMachineID, adminUserID)

	c.JSON(http.StatusOK, gin.H{
		"message":        "machine ID updated",
		"agent_id":       agentID,
		"old_machine_id": oldMachineID,
		"new_machine_id": req.NewMachineID,
	})
}

// ReclassifyDeviceType sets or clears the operator override for an agent's
// device type (SERVER-002). Auto-detection is heuristic; the operator has
// final say. null/empty device_type clears the override.
func (h *AgentHandler) ReclassifyDeviceType(c *gin.Context) {
	idStr := c.Param("id")
	agentID, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	var req struct {
		DeviceType *string `json:"device_type"` // null clears the override
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var override *string
	if req.DeviceType != nil && *req.DeviceType != "" {
		if !models.ValidDeviceType(*req.DeviceType) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "device_type must be one of: server, desktop, laptop, phone, tablet, vm, container (or null to clear)"})
			return
		}
		override = req.DeviceType
	}

	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}
	oldEffective := agent.EffectiveDeviceType()

	if err := h.agentQueries.UpdateDeviceTypeManual(agentID, override); err != nil {
		log.Printf("[ERROR] [server] [admin] device_reclassify_failed agent_id=%s error=%q", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update device type"})
		return
	}

	agent.DeviceTypeManual = override
	adminUserID := c.GetString("user_id")
	log.Printf("[INFO] [server] [admin] device_reclassified agent_id=%s from=%s to=%s admin_user=%s",
		agentID, oldEffective, agent.EffectiveDeviceType(), adminUserID)

	c.JSON(http.StatusOK, gin.H{
		"id":                    agent.ID,
		"device_type":           agent.DeviceType,
		"device_type_manual":    agent.DeviceTypeManual,
		"effective_device_type": agent.EffectiveDeviceType(),
	})
}

// UnregisterAgent removes an agent from the system
func (h *AgentHandler) UnregisterAgent(c *gin.Context) {
	idStr := c.Param("id")
	agentID, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Check if agent exists
	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Delete the agent and all associated data
	if err := h.agentQueries.DeleteAgent(agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete agent"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "agent unregistered successfully",
		"agent_id": agentID,
		"hostname": agent.Hostname,
	})
}

// ReportSystemInfo handles system information updates from agents
func (h *AgentHandler) ReportSystemInfo(c *gin.Context) {
	agentID := c.MustGet("agent_id").(uuid.UUID)

	var req struct {
		Timestamp  time.Time              `json:"timestamp"`
		CPUModel    string                 `json:"cpu_model,omitempty"`
		CPUCores    int                    `json:"cpu_cores,omitempty"`
		CPUThreads  int                    `json:"cpu_threads,omitempty"`
		MemoryTotal uint64                 `json:"memory_total,omitempty"`
		DiskTotal   uint64                 `json:"disk_total,omitempty"`
		DiskUsed    uint64                 `json:"disk_used,omitempty"`
		IPAddress   string                 `json:"ip_address,omitempty"`
		Processes   int                    `json:"processes,omitempty"`
		Uptime      string                 `json:"uptime,omitempty"`
		DeviceType  string                 `json:"device_type,omitempty"`
		DeviceModel string                 `json:"device_model,omitempty"`
		OSDistro    string                 `json:"os_distro,omitempty"`
		Metadata    map[string]interface{} `json:"metadata,omitempty"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get current agent to preserve existing metadata
	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Update agent metadata with system information
	if agent.Metadata == nil {
		agent.Metadata = models.JSONB{}
	}

	// Store system specs in metadata
	if req.CPUModel != "" {
		agent.Metadata["cpu_model"] = req.CPUModel
	}
	if req.CPUCores > 0 {
		agent.Metadata["cpu_cores"] = req.CPUCores
	}
	if req.CPUThreads > 0 {
		agent.Metadata["cpu_threads"] = req.CPUThreads
	}
	if req.MemoryTotal > 0 {
		agent.Metadata["memory_total"] = req.MemoryTotal
	}
	if req.DiskTotal > 0 {
		agent.Metadata["disk_total"] = req.DiskTotal
	}
	if req.DiskUsed > 0 {
		agent.Metadata["disk_used"] = req.DiskUsed
	}
	if req.IPAddress != "" {
		agent.Metadata["ip_address"] = req.IPAddress
	}
	if req.Processes > 0 {
		agent.Metadata["processes"] = req.Processes
	}
	if req.Uptime != "" {
		agent.Metadata["uptime"] = req.Uptime
	}

	// Device classification (DEVICE-001): dedicated columns, not metadata.
	// device_type_manual is operator-owned and never touched by agent reports.
	if models.ValidDeviceType(req.DeviceType) {
		agent.DeviceType = req.DeviceType
	}
	if req.DeviceModel != "" {
		agent.DeviceModel = &req.DeviceModel
	}
	if req.OSDistro != "" {
		agent.OSDistro = &req.OSDistro
	}

	// Store the timestamp when system info was last updated
	agent.Metadata["system_info_updated_at"] = time.Now().UTC().Format(time.RFC3339)

	// Merge any additional metadata
	if req.Metadata != nil {
		for k, v := range req.Metadata {
			agent.Metadata[k] = v
		}
	}

	// Update agent with new metadata
	if err := h.agentQueries.UpdateAgent(agent); err != nil {
		log.Printf("Warning: Failed to update agent system info: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update system info"})
		return
	}

	log.Printf("[INFO] [server] [agents]System info updated for agent %s (%s): CPU=%s, Cores=%d, Memory=%dMB",
		agent.Hostname, agentID, req.CPUModel, req.CPUCores, req.MemoryTotal/1024/1024)

	c.JSON(http.StatusOK, gin.H{"message": "system info updated successfully"})
}

// EnableRapidPollingMode enables rapid polling for an agent by updating metadata
func (h *AgentHandler) EnableRapidPollingMode(agentID uuid.UUID, durationMinutes int) error {
	// Get current agent
	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		return fmt.Errorf("failed to get agent: %w", err)
	}

	// Calculate new rapid polling end time
	newRapidPollingUntil := time.Now().UTC().Add(time.Duration(durationMinutes) * time.Minute)

	// Update agent metadata with rapid polling settings
	if agent.Metadata == nil {
		agent.Metadata = models.JSONB{}
	}

	// Check if rapid polling is already active
	if enabled, ok := agent.Metadata["rapid_polling_enabled"].(bool); ok && enabled {
		if untilStr, ok := agent.Metadata["rapid_polling_until"].(string); ok {
			if currentUntil, err := time.Parse(time.RFC3339, untilStr); err == nil {
				// If current heartbeat expires later than the new duration, keep the longer duration
				if currentUntil.After(newRapidPollingUntil) {
					log.Printf("[INFO] [server] [agents] heartbeatHeartbeat already active for agent %s (%s), keeping longer duration (expires: %s)",
						agent.Hostname, agentID, currentUntil.Format(time.RFC3339))
					return nil
				}
				// Otherwise extend the heartbeat
				log.Printf("[INFO] [server] [agents] heartbeatExtending heartbeat for agent %s (%s) from %s to %s",
					agent.Hostname, agentID,
					currentUntil.Format(time.RFC3339),
					newRapidPollingUntil.Format(time.RFC3339))
			}
		}
	} else {
		log.Printf("[INFO] [server] [agents] heartbeatEnabling heartbeat mode for agent %s (%s) for %d minutes",
			agent.Hostname, agentID, durationMinutes)
	}

	// Set/update rapid polling settings
	agent.Metadata["rapid_polling_enabled"] = true
	agent.Metadata["rapid_polling_until"] = newRapidPollingUntil.Format(time.RFC3339)

	// Update agent in database
	if err := h.agentQueries.UpdateAgent(agent); err != nil {
		return fmt.Errorf("failed to update agent with rapid polling: %w", err)
	}

	return nil
}

// SetRapidPollingMode enables rapid polling mode for an agent
// Rate limiting is implemented at router level in cmd/server/main.go
func (h *AgentHandler) SetRapidPollingMode(c *gin.Context) {
	idStr := c.Param("id")
	agentID, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Check if agent exists
	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	var req struct {
		DurationMinutes int `json:"duration_minutes" binding:"required,min=1,max=60"`
		Enabled        bool `json:"enabled"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Calculate rapid polling end time
	rapidPollingUntil := time.Now().UTC().Add(time.Duration(req.DurationMinutes) * time.Minute)

	// Update agent metadata with rapid polling settings
	if agent.Metadata == nil {
		agent.Metadata = models.JSONB{}
	}
	agent.Metadata["rapid_polling_enabled"] = req.Enabled
	agent.Metadata["rapid_polling_until"] = rapidPollingUntil.Format(time.RFC3339)

	// Update agent in database
	if err := h.agentQueries.UpdateAgent(agent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update agent"})
		return
	}

	status := "disabled"
	duration := 0
	if req.Enabled {
		status = "enabled"
		duration = req.DurationMinutes
	}

	log.Printf("[INFO] [server] [agents]Rapid polling mode %s for agent %s (%s) for %d minutes",
		status, agent.Hostname, agentID, duration)

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Rapid polling mode %s", status),
		"enabled": req.Enabled,
		"duration_minutes": req.DurationMinutes,
		"rapid_polling_until": rapidPollingUntil,
	})
}

// TriggerReboot triggers a system reboot for an agent
func (h *AgentHandler) TriggerReboot(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Check if agent exists
	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Parse request body for optional parameters
	var req struct {
		DelayMinutes int    `json:"delay_minutes"`
		Message      string `json:"message"`
	}
	c.ShouldBindJSON(&req)

	// Default to 1 minute delay if not specified
	if req.DelayMinutes == 0 {
		req.DelayMinutes = 1
	}
	if req.Message == "" {
		req.Message = "Reboot requested by RedFlag"
	}

	// Create reboot command
	cmd := &models.AgentCommand{
		ID:          uuid.Must(uuid.NewV4()),
		AgentID:     agentID,
		CommandType: models.CommandTypeReboot,
		Params: models.JSONB{
			"delay_minutes": req.DelayMinutes,
			"message":       req.Message,
		},
		Status:    models.CommandStatusPending,
		Source:    models.CommandSourceManual,
		CreatedAt: time.Now().UTC(),
	}

	// Save command to database
	if err := h.signAndCreateCommand(cmd); err != nil {
		log.Printf("Failed to create reboot command: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create reboot command"})
		return
	}

	log.Printf("Reboot command created for agent %s (%s)", agent.Hostname, agentID)

	c.JSON(http.StatusOK, gin.H{
		"message":    "reboot command sent",
		"command_id": cmd.ID,
		"agent_id":   agentID,
		"hostname":   agent.Hostname,
	})
}

// TriggerCaptureScreenshot sends a capture_screenshot command to an agent.
// The agent captures its display, base64-encodes the PNG, and reports it back
// in the command result's stdout field. The UI polls GET /commands/:id to
// retrieve the image once the command completes.
func (h *AgentHandler) TriggerCaptureScreenshot(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Verify agent exists
	if _, err := h.agentQueries.GetAgentByID(agentID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	cmd := &models.AgentCommand{
		ID:          uuid.Must(uuid.NewV4()),
		AgentID:     agentID,
		CommandType: models.CommandTypeCaptureScreenshot,
		Params:      models.JSONB{},
		Status:      models.CommandStatusPending,
		Source:      models.CommandSourceManual,
		CreatedAt:   time.Now().UTC(),
	}

	if err := h.signAndCreateCommand(cmd); err != nil {
		log.Printf("[ERROR] [server] [screenshot] command_create_failed agent_id=%s error=%v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create screenshot command"})
		return
	}

	log.Printf("[INFO] [server] [screenshot] command_created agent_id=%s command_id=%s", agentID, cmd.ID)

	c.JSON(http.StatusOK, gin.H{
		"message":    "screenshot command sent",
		"command_id": cmd.ID,
	})
}

// GetAgentConfig returns current subsystem configuration for an agent
// GET /api/v1/agents/:id/config
func (h *AgentHandler) GetAgentConfig(c *gin.Context) {
	idStr := c.Param("id")
	agentID, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Verify agent exists
	_, err = h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Get subsystem configuration from database
	subsystems, err := h.subsystemQueries.GetSubsystems(agentID)
	if err != nil {
		log.Printf("Failed to get subsystems for agent %s: %v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get subsystem configuration"})
		return
	}

	// Convert to simple format for agent
	config := make(map[string]interface{})
	for _, subsystem := range subsystems {
		config[subsystem.Subsystem] = map[string]interface{}{
			"enabled":         subsystem.Enabled,
			"interval_minutes": subsystem.IntervalMinutes,
			"auto_run":        subsystem.AutoRun,
		}
	}

	// Polling resilience tuning (operational, fleet-wide). Delivered here so the
	// agent merges it into its local PollingConfig. Defaults match the agent's
	// built-in resilience defaults (agent/internal/agent/loop.go) so a missing
	// settings row, or a server without the settings service, degrades to those.
	polling := gin.H{
		"jitter_max_seconds":   30,
		"backoff_base_seconds": 10,
		"backoff_max_seconds":  300,
	}
	if h.securitySettings != nil {
		polling["jitter_max_seconds"] = h.securitySettings.GetOperationalInt("jitter_max_seconds", 30)
		polling["backoff_base_seconds"] = h.securitySettings.GetOperationalInt("backoff_base_seconds", 10)
		polling["backoff_max_seconds"] = h.securitySettings.GetOperationalInt("backoff_max_seconds", 300)
	}

	// Command-signing policy (fleet-wide). Default matches the agent's built-in
	// stale-key window so a missing settings row or a server without the settings
	// service degrades to the same behavior. The agent clamps to its doctrinal
	// ceiling regardless (SEC-028).
	commandSigning := gin.H{"stale_key_max_age_hours": 168}
	if h.securitySettings != nil {
		if v, err := h.securitySettings.GetSetting("command_signing", "stale_key_max_age_hours"); err == nil {
			if f, ok := v.(float64); ok {
				commandSigning["stale_key_max_age_hours"] = int(f)
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"subsystems":      config,
		"polling":         polling,
		"command_signing": commandSigning,
		"version":         time.Now().UTC().Unix(), // Simple version timestamp
	})
}

// getStringFromMap safely extracts a string value from a map
func getStringFromMap(m map[string]interface{}, key string) string {
	if val, exists := m[key]; exists {
		if str, ok := val.(string); ok {
			return str
		}
	}
	return ""
}

// CircuitBreakerStatus represents the health of a single circuit breaker
type CircuitBreakerStatus struct {
	Name               string     `json:"name"`
	State              string     `json:"state"`
	RecentFailures     int        `json:"recent_failures"`
	ConsecutiveSuccess int        `json:"consecutive_success"`
	NextAttempt        *time.Time `json:"next_attempt,omitempty"`
}

// CircuitBreakerReport represents circuit breaker health from an agent
// [ISSUE-004] Added for circuit breaker monitoring and alerting
type CircuitBreakerReport struct {
	Timestamp  time.Time              `json:"timestamp"`
	Subsystems []CircuitBreakerStatus `json:"subsystems"`
}

// ReportCircuitBreakerStats receives circuit breaker health from agents
// [ISSUE-004] Enables monitoring and alerting for circuit breaker states
func (h *AgentHandler) ReportCircuitBreakerStats(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	var report CircuitBreakerReport
	if err := c.ShouldBindJSON(&report); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid report format"})
		return
	}

	// Check for open circuit breakers and log warnings
	openBreakers := 0
	for _, subsystem := range report.Subsystems {
		if subsystem.State == "open" {
			openBreakers++
			log.Printf("[WARNING] [server] [circuit_breaker] agent=%s subsystem=%s state=open failures=%d",
				agentID, subsystem.Name, subsystem.RecentFailures)
		}
	}

	// Store the report in database for historical tracking
	// TODO: Add database storage for circuit breaker history

	if openBreakers > 0 {
		c.JSON(http.StatusOK, gin.H{
			"status":        "received",
			"open_breakers": openBreakers,
			"warning":       "circuit breakers are open - auto-healing in progress",
		})
	} else {
		c.JSON(http.StatusOK, gin.H{"status": "healthy"})
	}
}

// RevokeAgent invalidates an agent's refresh tokens so the agent loses
// authenticated access on its next renewal. Spec: docs/AGENT_LIFECYCLE.md
// "Revocation" — this is the explicit per-agent path, deliberately separate
// from registration-token revocation (no cascade in either direction).
//
// Surfaced from two places in the UI:
//   - Settings → Token Management → expand a token → per-bound-agent revoke
//   - Settings → Agents → row action menu → Revoke Agent
// Both call this endpoint.
//
// Behavior: revokes all refresh tokens for the agent_id and writes a
// system_event. The agent row itself is kept for history; the agent goes
// offline on its own at next renew failure. To fully remove the agent, use
// the existing Remove Agent endpoint.
func (h *AgentHandler) RevokeAgent(c *gin.Context) {
	idParam := c.Param("id")
	agentID, err := uuid.FromString(idParam)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent id"})
		return
	}

	var request struct {
		Reason string `json:"reason"`
	}
	c.ShouldBindJSON(&request) // optional

	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil || agent == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	if err := h.refreshTokenQueries.RevokeAllAgentTokens(agentID); err != nil {
		log.Printf("[ERROR] [server] [revoke_agent] revoke_failed agent_id=%s error=%v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to revoke agent"})
		return
	}

	reason := request.Reason
	if reason == "" {
		reason = "revoked via API"
	}

	event := &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		AgentID:      &agentID,
		EventType:    "agent_revoked",
		EventSubtype: "by_operator",
		Severity:     "warning",
		Component:    "agent",
		Message:      fmt.Sprintf("Agent %s revoked: %s", agent.Hostname, reason),
		Metadata: map[string]interface{}{
			"agent_id": agentID.String(),
			"hostname": agent.Hostname,
			"reason":   reason,
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := h.agentQueries.CreateSystemEvent(event); err != nil {
		log.Printf("[WARNING] [server] [revoke_agent] system_event_failed agent_id=%s error=%v", agentID, err)
	}

	log.Printf("[INFO] [server] [revoke_agent] agent_revoked agent_id=%s hostname=%s reason=%q",
		agentID, agent.Hostname, reason)

	c.JSON(http.StatusOK, gin.H{
		"status":   "revoked",
		"agent_id": agentID.String(),
		"hostname": agent.Hostname,
		"message":  "refresh tokens invalidated; agent will go offline on next renewal attempt",
	})
}
