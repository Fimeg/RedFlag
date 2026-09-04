package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/capability"
	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/Fimeg/RedFlag/server/internal/version"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)
// AgentUpdateHandler handles agent binary update operations
// DEPRECATED: This handler is being consolidated - will be replaced by unified update handling
type AgentUpdateHandler struct {
	agentQueries       *queries.AgentQueries
	agentUpdateQueries *queries.AgentUpdateQueries
	commandQueries      *queries.CommandQueries
	signingService     *services.SigningService
	nonceService       *services.UpdateNonceService
	agentHandler       *AgentHandler
	securitySettings   *services.SecuritySettingsService // optional; reads policy.require_nonce
	capabilityTokens   *queries.CapabilityTokenQueries   // optional; enables desktop-self minting (UPDATE-002)
}

// NewAgentUpdateHandler creates a new agent update handler
func NewAgentUpdateHandler(aq *queries.AgentQueries, auq *queries.AgentUpdateQueries, cq *queries.CommandQueries, ss *services.SigningService, ns *services.UpdateNonceService, ah *AgentHandler) *AgentUpdateHandler {
	return &AgentUpdateHandler{
		agentQueries:       aq,
		agentUpdateQueries: auq,
		commandQueries:      cq,
		signingService:     ss,
		nonceService:       ns,
		agentHandler:       ah,
	}
}

// SetSecuritySettings injects the settings service so this handler can read
// policy.require_nonce at request time. Nil leaves the strict default (nonce
// required) in place.
func (h *AgentUpdateHandler) SetSecuritySettings(s *services.SecuritySettingsService) {
	h.securitySettings = s
}

// SetCapabilityTokenQueries enables desktop-self token minting (UPDATE-002).
// Nil leaves agent updates working without tray delivery.
func (h *AgentUpdateHandler) SetCapabilityTokenQueries(q *queries.CapabilityTokenQueries) {
	h.capabilityTokens = q
}

// mintDesktopSelfToken creates and stores a desktop-self capability token for
// the given agent+version so the agent's polling loop picks it up and upgrades
// the systray app. Nil capabilityTokens (not wired) or a missing desktop
// package for this version are not errors — the agent update proceeds without
// desktop delivery.
func (h *AgentUpdateHandler) mintDesktopSelfToken(agentID uuid.UUID, version string) error {
	if h.capabilityTokens == nil {
		return nil
	}

	desktopPkg, err := h.agentUpdateQueries.GetUpdatePackageByVersion(version, "desktop-linux", "amd64")
	if err != nil || desktopPkg == nil {
		return nil // no desktop package for this version — normal
	}

	closure := []capability.ClosureEntry{{
		Name:         "redflag-desktop",
		Version:      version,
		SHA256:       desktopPkg.Checksum,
		Source:       "desktop-self",
		ArtifactPath: fmt.Sprintf("/api/v1/downloads/updates/%s", desktopPkg.ID),
	}}

	now := time.Now().UTC()
	token := &capability.Token{
		Version:     capability.Version,
		TokenID:     uuid.Must(uuid.NewV4()).String(),
		AgentID:     agentID.String(),
		PackageType: "desktop-self",
		Operation:   "upgrade",
		Closure:     closure,
		IssuedAt:    now.Unix(),
		NotBefore:   now.Unix(),
		ExpiresAt:   now.Add(24 * time.Hour).Unix(), // desktop restart may lag
	}
	if err := h.signingService.SignCapabilityToken(token); err != nil {
		return fmt.Errorf("sign desktop-self token: %w", err)
	}
	if err := h.capabilityTokens.Insert(token, uuid.Nil); err != nil {
		return fmt.Errorf("store desktop-self token: %w", err)
	}

	log.Printf("[INFO] [server] [agent_update] desktop_self_token_minted agent_id=%s version=%s token_id=%s", agentID, version, token.TokenID)
	return nil
}

// mintAgentSelfToken signs an agent-self capability token authorizing the
// privileged helper to swap the agent binary to the given version/checksum. The
// helper verifies this signature and the new binary's hash before installing.
// Single signing chokepoint for both the manual and bulk update paths; reuses the
// existing signing key (no new key). Returns the JSON to embed in the update_agent
// command params.
func (h *AgentUpdateHandler) mintAgentSelfToken(agentID uuid.UUID, version, agentChecksum string) (string, error) {
	// Build closure with both agent and helper binaries. The helper is a
	// first-class artifact: when the agent updates, the helper updates too.
	closure := []capability.ClosureEntry{{
		Name:    "redflag-agent",
		Version: version,
		SHA256:  agentChecksum,
		Source:  "agent-self",
	}}

	// Look up the helper package for the same version. If it exists, include
	// it in the closure so the helper can self-update. Missing helper is not
	// fatal — the agent still updates, and the helper stays as-is.
	if helperPkg, err := h.agentUpdateQueries.GetUpdatePackageByVersion(version, "helper-linux", "amd64"); err == nil && helperPkg != nil {
		closure = append(closure, capability.ClosureEntry{
			Name:    "redflag-helper",
			Version: version,
			SHA256:  helperPkg.Checksum,
			Source:  "agent-self",
		})
	}

	now := time.Now().UTC()
	token := &capability.Token{
		Version:     capability.Version,
		TokenID:     uuid.Must(uuid.NewV4()).String(),
		AgentID:     agentID.String(),
		PackageType: "agent-self",
		Operation:   "upgrade",
		Closure:     closure,
		IssuedAt:    now.Unix(),
		NotBefore:   now.Unix(),
		ExpiresAt:   now.Add(time.Hour).Unix(),
	}
	if err := h.signingService.SignCapabilityToken(token); err != nil {
		return "", fmt.Errorf("sign agent-self token: %w", err)
	}
	tokenJSON, err := json.Marshal(token)
	if err != nil {
		return "", fmt.Errorf("marshal agent-self token: %w", err)
	}
	return string(tokenJSON), nil
}

// UpdateAgent handles POST /api/v1/agents/:id/update (manual agent update)
func (h *AgentUpdateHandler) UpdateAgent(c *gin.Context) {
	// Extract agent ID from URL path
	agentID := c.Param("id")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent ID is required"})
		return
	}

	// Debug logging for development (controlled via REDFLAG_DEBUG env var or query param)
	debugMode := os.Getenv("REDFLAG_DEBUG") == "true" || c.Query("debug") == "true"
	if debugMode {
		log.Printf("[DEBUG] [UpdateAgent] Starting update request for agent %s from %s", agentID, c.ClientIP())
		log.Printf("[DEBUG] [UpdateAgent] Content-Type: %s, Content-Length: %d", c.ContentType(), c.Request.ContentLength)
	}

	var req models.AgentUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		if debugMode {
			log.Printf("[DEBUG] [UpdateAgent] JSON binding error for agent %s: %v", agentID, err)
		}
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
			"_error_context": "json_binding_failed", // Helps identify binding vs validation errors
		})
		return
	}

	// Always log critical update operations for audit trail
	log.Printf("[UPDATE] Agent %s received update request - Version: %s, Platform: %s", agentID, req.Version, req.Platform)

	// Debug: Log the parsed request
	if debugMode {
		log.Printf("[DEBUG] [UpdateAgent] Parsed update request - Version: %s, Platform: %s, Nonce: %s", req.Version, req.Platform, req.Nonce)
	}

	agentIDUUID, err := uuid.FromString(agentID)
	if err != nil {
		log.Printf("[UPDATE] Agent ID format error for %s: %v", agentID, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID format"})
		return
	}

	// Verify the agent exists
	agent, err := h.agentQueries.GetAgentByID(agentIDUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Check if agent is already updating
	if agent.IsUpdating {
		c.JSON(http.StatusConflict, gin.H{
			"error": "agent is already updating",
			"current_update": agent.UpdatingToVersion,
			"initiated_at": agent.UpdateInitiatedAt,
		})
		return
	}

	// Validate platform compatibility
	if !h.isPlatformCompatible(agent, req.Platform) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("platform %s is not compatible with agent %s/%s",
				req.Platform, agent.OSType, agent.OSArchitecture),
		})
		return
	}

	// No-downgrades. ETHOS §2 (Update Security) treats forward-only progression
	// as part of the replay-resistance contract: accepting a lower signed
	// version is functionally a replay attack against a previously-trusted
	// artifact. UI hides downgrade targets; this is the server-side floor.
	if agent.CurrentVersion != "" && version.CompareVersions(req.Version, agent.CurrentVersion) <= 0 {
		log.Printf("[UPDATE] [server] [agent_updates] downgrade_rejected agent=%s current=%s requested=%s",
			agentID, agent.CurrentVersion, req.Version)
		c.JSON(http.StatusBadRequest, gin.H{
			"error":           fmt.Sprintf("downgrades are not allowed: target %s is not newer than current %s", req.Version, agent.CurrentVersion),
			"_error_context":  "downgrade_rejected",
			"_current_version": agent.CurrentVersion,
			"_target_version":  req.Version,
		})
		return
	}

	// Get the update package
	pkg, err := h.agentUpdateQueries.GetUpdatePackageByVersion(req.Version, req.Platform, agent.OSArchitecture)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("update package not found: %v", err)})
		return
	}

	// Update agent status to "updating"
	if err := h.agentUpdateQueries.UpdateAgentUpdatingStatus(agentIDUUID, true, &req.Version); err != nil {
		log.Printf("Failed to update agent %s status to updating: %v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to initiate update"})
		return
	}

	// Validate the provided nonce. Strict default: nonce required. Operators
	// may disable for development by setting policy.require_nonce=false in
	// security_settings — logged at the [INFO] level so the gate is visible
	// in history when it fires.
	requireNonce := true
	if h.securitySettings != nil {
		requireNonce = h.securitySettings.GetPolicyBool("require_nonce", true)
	}
	if !requireNonce {
		log.Printf("[INFO] [server] [agent_updates] nonce_skipped agent_id=%s reason=policy.require_nonce=false",
			agentID)
	}
	if h.nonceService != nil && requireNonce {
		if debugMode {
			log.Printf("[DEBUG] [UpdateAgent] Validating nonce for agent %s: %s", agentID, req.Nonce)
		}
		verifiedNonce, err := h.nonceService.Validate(req.Nonce)
		if err != nil {
			h.agentUpdateQueries.UpdateAgentUpdatingStatus(agentIDUUID, false, nil) // Rollback
			log.Printf("[UPDATE] Nonce validation failed for agent %s: %v", agentID, err)
			// Include specific error context for debugging
			errorType := "signature_verification_failed"
			if err.Error() == "nonce expired" {
				errorType = "nonce_expired"
			} else if err.Error() == "invalid base64" {
				errorType = "invalid_nonce_format"
			}
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid update nonce: " + err.Error(),
				"_error_context": errorType,
				"_error_detail": err.Error(),
			})
			return
		}

		if debugMode {
			log.Printf("[DEBUG] [UpdateAgent] Nonce verified - AgentID: %s, TargetVersion: %s", verifiedNonce.AgentID, verifiedNonce.TargetVersion)
		}

		// Verify the nonce matches the requested agent and version
		if verifiedNonce.AgentID != agentID {
			if debugMode {
				log.Printf("[DEBUG] [UpdateAgent] Agent ID mismatch - nonce: %s, URL: %s", verifiedNonce.AgentID, agentID)
			}
			log.Printf("[UPDATE] Agent ID mismatch in nonce: expected %s, got %s", agentID, verifiedNonce.AgentID)
			h.agentUpdateQueries.UpdateAgentUpdatingStatus(agentIDUUID, false, nil) // Rollback
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "nonce agent ID mismatch",
				"_agent_id": agentID,
				"_nonce_agent_id": verifiedNonce.AgentID,
			})
			return
		}
		if verifiedNonce.TargetVersion != req.Version {
			if debugMode {
				log.Printf("[DEBUG] [UpdateAgent] Version mismatch - nonce: %s, request: %s", verifiedNonce.TargetVersion, req.Version)
			}
			log.Printf("[UPDATE] Version mismatch in nonce: expected %s, got %s", req.Version, verifiedNonce.TargetVersion)
			h.agentUpdateQueries.UpdateAgentUpdatingStatus(agentIDUUID, false, nil) // Rollback
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "nonce version mismatch",
				"_requested_version": req.Version,
				"_nonce_version": verifiedNonce.TargetVersion,
			})
			return
		}
		log.Printf("[UPDATE] Nonce successfully validated for agent %s to version %s", agentID, req.Version)
	}

	// Generate nonce for replay protection
	nonceUUID := uuid.Must(uuid.NewV4())
	nonceTimestamp := time.Now().UTC()
	var nonceSignature string
	if h.signingService != nil {
		var err error
		nonceSignature, err = h.signingService.SignNonce(nonceUUID, nonceTimestamp)
		if err != nil {
			log.Printf("Failed to sign nonce: %v", err)
			h.agentUpdateQueries.UpdateAgentUpdatingStatus(agentIDUUID, false, nil) // Rollback
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to sign nonce"})
			return
		}
	}

	// Mint an agent-self capability token so the privileged helper — not the
	// agent's own sudo — performs the binary swap. Fail-closed: a signed package is
	// already a precondition for getting here, so a signing failure refuses the
	// update rather than falling back to an unprivileged path the agent no longer has.
	selfTokenJSON, err := h.mintAgentSelfToken(agentIDUUID, req.Version, pkg.Checksum)
	if err != nil {
		h.agentUpdateQueries.UpdateAgentUpdatingStatus(agentIDUUID, false, nil)
		log.Printf("[ERROR] [server] [agent_update] self_token_failed agent_id=%s error=%v", agentIDUUID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to authorize agent upgrade"})
		return
	}

	// Mint a desktop-self token so the agent's polling loop also upgrades the
	// systray app. Non-fatal: the agent update proceeds without desktop delivery.
	if err := h.mintDesktopSelfToken(agentIDUUID, req.Version); err != nil {
		log.Printf("[WARNING] [server] [agent_update] desktop_self_token_failed agent_id=%s error=%v", agentIDUUID, err)
	}

	// Create update command for agent
	commandType := "update_agent"
	commandParams := map[string]interface{}{
		"version":          req.Version,
		"platform":         req.Platform,
		"download_url":     fmt.Sprintf("/api/v1/downloads/updates/%s", pkg.ID),
		"signature":        pkg.Signature,
		"checksum":         pkg.Checksum,
		"file_size":        pkg.FileSize,
		"nonce_uuid":       nonceUUID.String(),
		"nonce_timestamp":  nonceTimestamp.Format(time.RFC3339),
		"nonce_signature":  nonceSignature,
		"capability_token": selfTokenJSON,
	}

	// Include helper download URL if a helper package exists for this version.
	// The agent downloads and stages it alongside the agent binary.
	if helperPkg, err := h.agentUpdateQueries.GetUpdatePackageByVersion(req.Version, "helper-linux", "amd64"); err == nil && helperPkg != nil {
		commandParams["helper_download_url"] = fmt.Sprintf("/api/v1/downloads/updates/%s", helperPkg.ID)
		commandParams["helper_checksum"] = helperPkg.Checksum
	}

	// Schedule the update if requested
	if req.Scheduled != nil {
		scheduledTime, err := time.Parse(time.RFC3339, *req.Scheduled)
		if err != nil {
			h.agentUpdateQueries.UpdateAgentUpdatingStatus(agentIDUUID, false, nil) // Rollback
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid scheduled time format"})
			return
		}
		commandParams["scheduled_at"] = scheduledTime
	}

	// Create the command in database
	command := &models.AgentCommand{
		ID:          uuid.Must(uuid.NewV4()),
		AgentID:     agentIDUUID,
		CommandType: commandType,
		Params:      commandParams,
		Status:      models.CommandStatusPending,
		Source:      "manual",
		CreatedAt:   time.Now().UTC(),
	}

	if err := h.agentHandler.signAndCreateCommand(command); err != nil {
		// Rollback the updating status
		h.agentUpdateQueries.UpdateAgentUpdatingStatus(agentIDUUID, false, nil)
		log.Printf("Failed to create update command for agent %s: %v", agentIDUUID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create command"})
		return
	}

	// Log agent update initiation to system_events table
	event := &models.SystemEvent{
		ID:           uuid.Must(uuid.NewV4()),
		AgentID:      &agentIDUUID,
		EventType:    "agent_update",
		EventSubtype: "initiated",
		Severity:     "info",
		Component:    "agent",
		Message:      fmt.Sprintf("Agent update initiated: %s -> %s (%s)", agent.CurrentVersion, req.Version, req.Platform),
		Metadata: map[string]interface{}{
			"old_version": agent.CurrentVersion,
			"new_version": req.Version,
			"platform":    req.Platform,
			"source":      "web_ui",
		},
		CreatedAt: time.Now().UTC(),
	}
	if err := h.agentQueries.CreateSystemEvent(event); err != nil {
		log.Printf("Warning: Failed to log agent update to system_events: %v", err)
	}

	log.Printf("[UPDATE] Agent update initiated for %s: %s -> %s (%s)", agent.Hostname, agent.CurrentVersion, req.Version, req.Platform)

	response := models.AgentUpdateResponse{
		Message:       "Update initiated successfully",
		UpdateID:      command.ID.String(),
		DownloadURL:   fmt.Sprintf("/api/v1/downloads/updates/%s", pkg.ID),
		Signature:     pkg.Signature,
		Checksum:      pkg.Checksum,
		FileSize:      pkg.FileSize,
		EstimatedTime: h.estimateUpdateTime(pkg.FileSize),
	}

	c.JSON(http.StatusOK, response)
}

// BulkUpdateAgents handles POST /api/v1/agents/bulk-update (bulk agent update)
func (h *AgentUpdateHandler) BulkUpdateAgents(c *gin.Context) {
	var req models.BulkAgentUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if len(req.AgentIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no agent IDs provided"})
		return
	}

	if len(req.AgentIDs) > 50 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "too many agents in bulk update (max 50)"})
		return
	}

	// Get the update package first to validate it exists
	pkg, err := h.agentUpdateQueries.GetUpdatePackageByVersion(req.Version, req.Platform, "")
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": fmt.Sprintf("update package not found: %v", err)})
		return
	}

	// Validate all agents exist and are compatible
	var results []map[string]interface{}
	var errors []string

	for _, agentID := range req.AgentIDs {
		agent, err := h.agentQueries.GetAgentByID(agentID)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Agent %s: not found", agentID))
			continue
		}

		if agent.IsUpdating {
			errors = append(errors, fmt.Sprintf("Agent %s: already updating", agentID))
			continue
		}

		if !h.isPlatformCompatible(agent, req.Platform) {
			errors = append(errors, fmt.Sprintf("Agent %s: platform incompatible", agentID))
			continue
			}

		// No-downgrades. Same floor as the single-agent path — refuse any
		// target that isn't strictly newer than the agent's reported version.
		if agent.CurrentVersion != "" && version.CompareVersions(req.Version, agent.CurrentVersion) <= 0 {
			log.Printf("[UPDATE] [server] [agent_updates] downgrade_rejected_bulk agent=%s current=%s requested=%s",
				agentID, agent.CurrentVersion, req.Version)
			errors = append(errors, fmt.Sprintf("Agent %s: downgrade not allowed (current %s, requested %s)",
				agentID, agent.CurrentVersion, req.Version))
			continue
		}

		// Update agent status
		if err := h.agentUpdateQueries.UpdateAgentUpdatingStatus(agentID, true, &req.Version); err != nil {
			errors = append(errors, fmt.Sprintf("Agent %s: failed to update status", agentID))
			continue
		}

		// Generate nonce for replay protection
		nonceUUID := uuid.Must(uuid.NewV4())
		nonceTimestamp := time.Now().UTC()
		var nonceSignature string
		if h.signingService != nil {
			var err error
			nonceSignature, err = h.signingService.SignNonce(nonceUUID, nonceTimestamp)
			if err != nil {
				errors = append(errors, fmt.Sprintf("Agent %s: failed to sign nonce", agentID))
				h.agentUpdateQueries.UpdateAgentUpdatingStatus(agentID, false, nil)
				continue
			}
		}

		// Mint the agent-self token so the helper performs the swap (no agent sudo).
		selfTokenJSON, err := h.mintAgentSelfToken(agentID, req.Version, pkg.Checksum)
		if err != nil {
			h.agentUpdateQueries.UpdateAgentUpdatingStatus(agentID, false, nil)
			errors = append(errors, fmt.Sprintf("Agent %s: failed to authorize agent upgrade", agentID))
			continue
		}

		// Desktop-self token: non-fatal, delivered via the capability-token poll.
		if err := h.mintDesktopSelfToken(agentID, req.Version); err != nil {
			log.Printf("[WARNING] [server] [agent_update] desktop_self_token_failed agent_id=%s error=%v", agentID, err)
		}

		// Create update command
		command := &models.AgentCommand{
			ID:          uuid.Must(uuid.NewV4()),
			AgentID:     agentID,
			CommandType: "update_agent",
			Params: map[string]interface{}{
				"version":          req.Version,
				"platform":         req.Platform,
				"download_url":     fmt.Sprintf("/api/v1/downloads/updates/%s", pkg.ID),
				"signature":        pkg.Signature,
				"checksum":         pkg.Checksum,
				"file_size":        pkg.FileSize,
				"nonce_uuid":       nonceUUID.String(),
				"nonce_timestamp":  nonceTimestamp.Format(time.RFC3339),
				"nonce_signature":  nonceSignature,
				"capability_token": selfTokenJSON,
			},
			Status:    models.CommandStatusPending,
			Source:    "manual",
			CreatedAt: time.Now().UTC(),
		}

		if req.Scheduled != nil {
			command.Params["scheduled_at"] = *req.Scheduled
		}

		if err := h.agentHandler.signAndCreateCommand(command); err != nil {
			// Rollback status
			h.agentUpdateQueries.UpdateAgentUpdatingStatus(agentID, false, nil)
			errors = append(errors, fmt.Sprintf("Agent %s: failed to create command", agentID))
			continue
		}

		results = append(results, map[string]interface{}{
			"agent_id":   agentID,
			"hostname":   agent.Hostname,
			"update_id":  command.ID.String(),
			"status":     "initiated",
		})

		// Log each bulk update initiation to system_events table
		event := &models.SystemEvent{
			ID:           uuid.Must(uuid.NewV4()),
			AgentID:      &agentID,
			EventType:    "agent_update",
			EventSubtype: "initiated",
			Severity:     "info",
			Component:    "agent",
			Message:      fmt.Sprintf("Agent update initiated (bulk): %s -> %s (%s)", agent.CurrentVersion, req.Version, req.Platform),
			Metadata: map[string]interface{}{
				"old_version": agent.CurrentVersion,
				"new_version": req.Version,
				"platform":    req.Platform,
				"source":      "web_ui_bulk",
			},
			CreatedAt: time.Now().UTC(),
		}
		if err := h.agentQueries.CreateSystemEvent(event); err != nil {
			log.Printf("Warning: Failed to log bulk agent update to system_events: %v", err)
		}

		log.Printf("[INFO] [server] [updates]Bulk update initiated for %s: %s (%s)", agent.Hostname, req.Version, req.Platform)
	}

	response := gin.H{
		"message":      fmt.Sprintf("Bulk update completed with %d successes and %d failures", len(results), len(errors)),
		"updated":      results,
		"failed":       errors,
		"total_agents": len(req.AgentIDs),
		"package_info": gin.H{
			"version":   pkg.Version,
			"platform":  pkg.Platform,
			"file_size": pkg.FileSize,
			"checksum":  pkg.Checksum,
		},
	}

	c.JSON(http.StatusOK, response)
}

// ListUpdatePackages handles GET /api/v1/updates/packages (list available update packages)
func (h *AgentUpdateHandler) ListUpdatePackages(c *gin.Context) {
	version := c.Query("version")
	platform := c.Query("platform")
	limitStr := c.Query("limit")
	offsetStr := c.Query("offset")

	limit := 0
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			limit = l
		}
	}

	offset := 0
	if offsetStr != "" {
		if o, err := strconv.Atoi(offsetStr); err == nil && o >= 0 {
			offset = o
		}
	}

	packages, err := h.agentUpdateQueries.ListUpdatePackages(version, platform, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list update packages"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"packages": packages,
		"total":    len(packages),
		"limit":    limit,
		"offset":   offset,
	})
}

// SignUpdatePackage handles POST /api/v1/updates/packages/sign (sign a new update package)
func (h *AgentUpdateHandler) SignUpdatePackage(c *gin.Context) {
	var req struct {
		Version      string `json:"version" binding:"required"`
		Platform     string `json:"platform" binding:"required"`
	Architecture string `json:"architecture" binding:"required"`
		BinaryPath   string `json:"binary_path" binding:"required"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if h.signingService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "signing service not available"})
		return
	}

	// Sign the binary
	pkg, err := h.signingService.SignFile(req.BinaryPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to sign binary: %v", err)})
		return
	}

	// Set additional fields
	pkg.Version = req.Version
	pkg.Platform = req.Platform
	pkg.Architecture = req.Architecture

	// Save to database
	if err := h.agentUpdateQueries.CreateUpdatePackage(pkg); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to save update package: %v", err)})
		return
	}

	log.Printf("[INFO] [server] [updates]Update package signed and saved: %s %s/%s (ID: %s)",
		pkg.Version, pkg.Platform, pkg.Architecture, pkg.ID)

	c.JSON(http.StatusOK, gin.H{
		"message": "Update package signed successfully",
		"package": pkg,
	})
}

// isPlatformCompatible checks if the update package is compatible with the agent
func (h *AgentUpdateHandler) isPlatformCompatible(agent *models.Agent, updatePlatform string) bool {
	// Normalize platform strings
	agentPlatform := strings.ToLower(agent.OSType)
	updatePlatform = strings.ToLower(updatePlatform)

	// Check for basic OS compatibility
	if !strings.Contains(updatePlatform, agentPlatform) {
		return false
	}

	// Check architecture compatibility if specified
	if strings.Contains(updatePlatform, "amd64") && !strings.Contains(strings.ToLower(agent.OSArchitecture), "amd64") {
		return false
	}
	if strings.Contains(updatePlatform, "arm64") && !strings.Contains(strings.ToLower(agent.OSArchitecture), "arm64") {
		return false
	}
	if strings.Contains(updatePlatform, "386") && !strings.Contains(strings.ToLower(agent.OSArchitecture), "386") {
		return false
	}

	return true
}

// estimateUpdateTime estimates how long an update will take based on file size
func (h *AgentUpdateHandler) estimateUpdateTime(fileSize int64) int {
	// Rough estimate: 1 second per MB + 30 seconds base time
	seconds := int(fileSize/1024/1024) + 30

	// Cap at 5 minutes
	if seconds > 300 {
		seconds = 300
	}

	return seconds
}

// GenerateUpdateNonce handles POST /api/v1/agents/:id/update-nonce
func (h *AgentUpdateHandler) GenerateUpdateNonce(c *gin.Context) {
	agentID := c.Param("id")
	targetVersion := c.Query("target_version")

	if targetVersion == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "target_version query parameter required"})
		return
	}

	if h.nonceService == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "nonce service not available"})
		return
	}

	// Parse agent ID as UUID
	agentIDUUID, err := uuid.FromString(agentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID format"})
		return
	}

	// Verify agent exists
	agent, err := h.agentQueries.GetAgentByID(agentIDUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Generate nonce
	nonce, err := h.nonceService.Generate(agentID, targetVersion)
	if err != nil {
		log.Printf("[ERROR] Failed to generate update nonce for agent %s: %v", agentID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate nonce"})
		return
	}

	log.Printf("[system] Generated update nonce for agent %s (%s) -> %s", agentID, agent.Hostname, targetVersion)

	c.JSON(http.StatusOK, gin.H{
		"agent_id":           agentID,
		"hostname":           agent.Hostname,
		"current_version":    agent.CurrentVersion,
		"target_version":     targetVersion,
		"update_nonce":       nonce,
		"expires_at":         time.Now().Add(10 * time.Minute).Unix(),
		"expires_in_seconds": 600,
	})
}

// CheckForUpdateAvailable handles GET /api/v1/agents/:id/updates/available
func (h *AgentUpdateHandler) CheckForUpdateAvailable(c *gin.Context) {
	agentID := c.Param("id")

	// Parse agent ID
	agentIDUUID, err := uuid.FromString(agentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID format"})
		return
	}

	// Query database for agent's current version
	agent, err := h.agentQueries.GetAgentByID(agentIDUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Platform format: separate os_type and os_architecture from agent data
	osType := strings.ToLower(agent.OSType)
	osArch := agent.OSArchitecture

	// Check if newer version available from agent_update_packages table
	latestVersion, err := h.agentUpdateQueries.GetLatestVersionByTypeAndArch(osType, osArch)
	if err != nil {
		log.Printf("[DEBUG] GetLatestVersionByTypeAndArch error for %s/%s: %v", osType, osArch, err)
		c.JSON(http.StatusOK, gin.H{
			"hasUpdate":      false,
			"reason":         "no packages available",
			"currentVersion": agent.CurrentVersion,
		})
		return
	}

	// Check if this is actually newer than current version using version package
	currentVer := version.Version(agent.CurrentVersion)
	latestVer := version.Version(latestVersion)
	hasUpdate := currentVer.IsUpgrade(latestVer)

	log.Printf("[DEBUG] Version comparison - latest: %s, current: %s, hasUpdate: %v for platform: %s/%s", latestVersion, agent.CurrentVersion, hasUpdate, osType, osArch)

	// Special handling for sub-versions (0.1.23.5 vs 0.1.23)
	if !hasUpdate && strings.HasPrefix(latestVersion, agent.CurrentVersion + ".") {
		hasUpdate = true
		log.Printf("[DEBUG] Detected sub-version upgrade: %s -> %s", agent.CurrentVersion, latestVersion)
	}

	platform := version.Platform(osType + "-" + osArch)
	c.JSON(http.StatusOK, gin.H{
		"hasUpdate":       hasUpdate,
		"currentVersion":  agent.CurrentVersion,
		"latestVersion":   latestVersion,
		"platform":        platform.String(),
	})
}

// GetUpdateStatus handles GET /api/v1/agents/:id/updates/status
func (h *AgentUpdateHandler) GetUpdateStatus(c *gin.Context) {
	agentID := c.Param("id")

	// Parse agent ID
	agentIDUUID, err := uuid.FromString(agentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID format"})
		return
	}

	// Fetch agent with update state
	agent, err := h.agentQueries.GetAgentByID(agentIDUUID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Determine status from agent state + recent commands
	var status string
	var progress *int
	var errorMsg *string

	if agent.IsUpdating {
		// Check if agent has pending update command
		cmd, err := h.agentUpdateQueries.GetPendingUpdateCommand(agentID)
		if err == nil && cmd != nil {
			status = "downloading"
			// Progress could be based on last acknowledgment time
			if time.Since(cmd.CreatedAt) > 2*time.Minute {
				status = "installing"
			}
		} else {
			status = "pending"
		}
	} else {
		status = "idle"
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    status,
		"progress":  progress,
		"error":     errorMsg,
	})
}