package handlers

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/aggregator-project/aggregator-server/internal/api/middleware"
	"github.com/aggregator-project/aggregator-server/internal/database/queries"
	"github.com/aggregator-project/aggregator-server/internal/models"
	"github.com/aggregator-project/aggregator-server/internal/utils"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AgentHandler struct {
	agentQueries          *queries.AgentQueries
	commandQueries        *queries.CommandQueries
	refreshTokenQueries   *queries.RefreshTokenQueries
	checkInInterval       int
	latestAgentVersion    string
}

func NewAgentHandler(aq *queries.AgentQueries, cq *queries.CommandQueries, rtq *queries.RefreshTokenQueries, checkInInterval int, latestAgentVersion string) *AgentHandler {
	return &AgentHandler{
		agentQueries:          aq,
		commandQueries:        cq,
		refreshTokenQueries:   rtq,
		checkInInterval:       checkInInterval,
		latestAgentVersion:    latestAgentVersion,
	}
}

// RegisterAgent handles agent registration
func (h *AgentHandler) RegisterAgent(c *gin.Context) {
	var req models.AgentRegistrationRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create new agent
	agent := &models.Agent{
		ID:             uuid.New(),
		Hostname:       req.Hostname,
		OSType:         req.OSType,
		OSVersion:      req.OSVersion,
		OSArchitecture: req.OSArchitecture,
		AgentVersion:   req.AgentVersion,
		LastSeen:       time.Now(),
		Status:         "online",
		Metadata:       models.JSONB{},
	}

	// Add metadata if provided
	if req.Metadata != nil {
		for k, v := range req.Metadata {
			agent.Metadata[k] = v
		}
	}

	// Save to database
	if err := h.agentQueries.CreateAgent(agent); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to register agent"})
		return
	}

	// Generate JWT access token (short-lived: 24 hours)
	token, err := middleware.GenerateAgentToken(agent.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// Generate refresh token (long-lived: 90 days)
	refreshToken, err := queries.GenerateRefreshToken()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate refresh token"})
		return
	}

	// Store refresh token in database with 90-day expiration
	refreshTokenExpiry := time.Now().Add(90 * 24 * time.Hour)
	if err := h.refreshTokenQueries.CreateRefreshToken(agent.ID, refreshToken, refreshTokenExpiry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store refresh token"})
		return
	}

	// Return response with both tokens
	response := models.AgentRegistrationResponse{
		AgentID:      agent.ID,
		Token:        token,
		RefreshToken: refreshToken,
		Config: map[string]interface{}{
			"check_in_interval": h.checkInInterval,
			"server_url":        c.Request.Host,
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
		CPUPercent    float64                   `json:"cpu_percent,omitempty"`
		MemoryPercent float64                   `json:"memory_percent,omitempty"`
		MemoryUsedGB  float64                   `json:"memory_used_gb,omitempty"`
		MemoryTotalGB float64                   `json:"memory_total_gb,omitempty"`
		DiskUsedGB    float64                   `json:"disk_used_gb,omitempty"`
		DiskTotalGB   float64                   `json:"disk_total_gb,omitempty"`
		DiskPercent   float64                   `json:"disk_percent,omitempty"`
		Uptime        string                    `json:"uptime,omitempty"`
		Version       string                    `json:"version,omitempty"`
		Metadata      map[string]interface{}     `json:"metadata,omitempty"`
	}

	// Parse metrics if provided (optional, won't fail if empty)
	err := c.ShouldBindJSON(&metrics)
	if err != nil {
		log.Printf("DEBUG: Failed to parse metrics JSON: %v", err)
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
					log.Printf("🔄 Agent %s (%s) version %s has update available: %s",
						agent.Hostname, agentID, metrics.Version, h.latestAgentVersion)
				} else {
					log.Printf("✅ Agent %s (%s) version %s is up to date",
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
				agent.Metadata["version_checked_at"] = time.Now().Format(time.RFC3339)

				// Update agent metadata
				if err := h.agentQueries.UpdateAgent(agent); err != nil {
					log.Printf("Warning: Failed to update agent metadata: %v", err)
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
			agent.Metadata["metrics_updated_at"] = time.Now().Format(time.RFC3339)

			// Process heartbeat metadata from agent check-ins
			if metrics.Metadata != nil {
				if rapidPollingEnabled, exists := metrics.Metadata["rapid_polling_enabled"]; exists {
					if rapidPollingUntil, exists := metrics.Metadata["rapid_polling_until"]; exists {
						// Parse the until timestamp
						if untilTime, err := time.Parse(time.RFC3339, rapidPollingUntil.(string)); err == nil {
							// Validate if rapid polling is still active (not expired)
							isActive := rapidPollingEnabled.(bool) && time.Now().Before(untilTime)

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

			// Update agent with new metadata
			if err := h.agentQueries.UpdateAgent(agent); err != nil {
				log.Printf("Warning: Failed to update agent metrics: %v", err)
			}
		}
	}

	// Update last_seen
	if err := h.agentQueries.UpdateAgentLastSeen(agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update last seen"})
		return
	}

	// Process heartbeat metadata from agent check-ins
	if metrics.Metadata != nil {
		agent, err := h.agentQueries.GetAgentByID(agentID)
		if err == nil && agent.Metadata != nil {
			if rapidPollingEnabled, exists := metrics.Metadata["rapid_polling_enabled"]; exists {
				if rapidPollingUntil, exists := metrics.Metadata["rapid_polling_until"]; exists {
					// Parse the until timestamp
					if untilTime, err := time.Parse(time.RFC3339, rapidPollingUntil.(string)); err == nil {
						// Validate if rapid polling is still active (not expired)
						isActive := rapidPollingEnabled.(bool) && time.Now().Before(untilTime)

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
						log.Printf("🔄 Agent %s (%s) stored version %s has update available: %s",
							agent.Hostname, agentID, agent.CurrentVersion, h.latestAgentVersion)
					} else {
						log.Printf("✅ Agent %s (%s) stored version %s is up to date",
							agent.Hostname, agentID, agent.CurrentVersion)
					}
				}
			}
		}
	}

	// Get pending commands
	commands, err := h.commandQueries.GetPendingCommands(agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve commands"})
		return
	}

	// Convert to response format
	commandItems := make([]models.CommandItem, 0, len(commands))
	for _, cmd := range commands {
		commandItems = append(commandItems, models.CommandItem{
			ID:     cmd.ID.String(),
			Type:   cmd.CommandType,
			Params: cmd.Params,
		})

		// Mark as sent
		h.commandQueries.MarkCommandSent(cmd.ID)
	}

	// Check if rapid polling should be enabled
	var rapidPolling *models.RapidPollingConfig

	// Enable rapid polling if there are commands to process
	if len(commandItems) > 0 {
		rapidPolling = &models.RapidPollingConfig{
			Enabled: true,
			Until:   time.Now().Add(10 * time.Minute).Format(time.RFC3339), // 10 minutes default
		}
	} else {
		// Check if agent has rapid polling already configured in metadata
		agent, err := h.agentQueries.GetAgentByID(agentID)
		if err == nil && agent.Metadata != nil {
			if enabled, ok := agent.Metadata["rapid_polling_enabled"].(bool); ok && enabled {
				if untilStr, ok := agent.Metadata["rapid_polling_until"].(string); ok {
					if until, err := time.Parse(time.RFC3339, untilStr); err == nil && time.Now().Before(until) {
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
				if until, err := time.Parse(time.RFC3339, untilStr); err == nil && time.Now().Before(until) {
					// Server thinks heartbeat is active and not expired
					// Check if agent is reporting heartbeat in this check-in
					agentReportingHeartbeat := false
					if metrics.Metadata != nil {
						if agentEnabled, exists := metrics.Metadata["rapid_polling_enabled"]; exists {
							agentReportingHeartbeat = agentEnabled.(bool)
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
							now := time.Now()
							auditCmd := &models.AgentCommand{
								ID:          uuid.New(),
								AgentID:     agentID,
								CommandType: models.CommandTypeDisableHeartbeat,
								Params:      models.JSONB{},
								Status:      models.CommandStatusCompleted,
								Result: models.JSONB{
									"message": "Heartbeat cleared - agent restarted without active heartbeat mode",
								},
								CreatedAt:   now,
								SentAt:      &now,
								CompletedAt: &now,
							}

							if err := h.commandQueries.CreateCommand(auditCmd); err != nil {
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

	response := models.CommandsResponse{
		Commands:     commandItems,
		RapidPolling: rapidPolling,
	}

	c.JSON(http.StatusOK, response)
}

// ListAgents returns all agents with last scan information
func (h *AgentHandler) ListAgents(c *gin.Context) {
	status := c.Query("status")
	osType := c.Query("os_type")

	agents, err := h.agentQueries.ListAgentsWithLastScan(status, osType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list agents"})
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
	id, err := uuid.Parse(idStr)
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

// TriggerScan creates a scan command for an agent
func (h *AgentHandler) TriggerScan(c *gin.Context) {
	idStr := c.Param("id")
	agentID, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	// Create scan command
	cmd := &models.AgentCommand{
		ID:          uuid.New(),
		AgentID:     agentID,
		CommandType: models.CommandTypeScanUpdates,
		Params:      models.JSONB{},
		Status:      models.CommandStatusPending,
	}

	if err := h.commandQueries.CreateCommand(cmd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create command"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "scan triggered", "command_id": cmd.ID})
}

// TriggerHeartbeat creates a heartbeat toggle command for an agent
func (h *AgentHandler) TriggerHeartbeat(c *gin.Context) {
	idStr := c.Param("id")
	agentID, err := uuid.Parse(idStr)
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

	// Create heartbeat command with duration parameter
	cmd := &models.AgentCommand{
		ID:          uuid.New(),
		AgentID:     agentID,
		CommandType: commandType,
		Params: models.JSONB{
			"duration_minutes": request.DurationMinutes,
		},
		Status: models.CommandStatusPending,
	}

	if err := h.commandQueries.CreateCommand(cmd); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create heartbeat command"})
		return
	}

	// TODO: Clean up previous heartbeat commands for this agent (only for enable commands)
	// if request.Enabled {
	// 	// Mark previous heartbeat commands as 'replaced' to clean up Live Operations view
	// 	if err := h.commandQueries.MarkPreviousHeartbeatCommandsReplaced(agentID, cmd.ID); err != nil {
	// 		log.Printf("Warning: Failed to mark previous heartbeat commands as replaced: %v", err)
	// 		// Don't fail the request, just log the warning
	// 	} else {
	// 		log.Printf("[Heartbeat] Cleaned up previous heartbeat commands for agent %s", agentID)
	// 	}
	// }

	action := "disabled"
	if request.Enabled {
		action = "enabled"
	}

	log.Printf("💓 Heartbeat %s command created for agent %s (duration: %d minutes)",
		action, agentID, request.DurationMinutes)

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("heartbeat %s command sent", action),
		"command_id": cmd.ID,
		"enabled": request.Enabled,
	})
}

// GetHeartbeatStatus returns the current heartbeat status for an agent
func (h *AgentHandler) GetHeartbeatStatus(c *gin.Context) {
	idStr := c.Param("id")
	agentID, err := uuid.Parse(idStr)
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
	}

	if agent.Metadata != nil {
		// Check if heartbeat is enabled in metadata
		if enabled, exists := agent.Metadata["rapid_polling_enabled"]; exists {
			response["enabled"] = enabled.(bool)

			// If enabled, get the until time and check if still active
			if enabled.(bool) {
				if untilStr, exists := agent.Metadata["rapid_polling_until"]; exists {
					response["until"] = untilStr.(string)

					// Parse the until timestamp to check if still active
					if untilTime, err := time.Parse(time.RFC3339, untilStr.(string)); err == nil {
						response["active"] = time.Now().Before(untilTime)
					}
				}

				// Get duration if available
				if duration, exists := agent.Metadata["rapid_polling_duration_minutes"]; exists {
					response["duration_minutes"] = duration.(float64)
				}
			}
		}
	}

	c.JSON(http.StatusOK, response)
}

// TriggerUpdate creates an update command for an agent
func (h *AgentHandler) TriggerUpdate(c *gin.Context) {
	idStr := c.Param("id")
	agentID, err := uuid.Parse(idStr)
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
		ID:          uuid.New(),
		AgentID:     agentID,
		CommandType: models.CommandTypeInstallUpdate,
		Params:      params,
		Status:      models.CommandStatusPending,
	}

	if err := h.commandQueries.CreateCommand(cmd); err != nil {
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

	// Validate refresh token
	refreshToken, err := h.refreshTokenQueries.ValidateRefreshToken(req.AgentID, req.RefreshToken)
	if err != nil {
		log.Printf("Token renewal failed for agent %s: %v", req.AgentID, err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired refresh token"})
		return
	}

	// Check if agent still exists
	agent, err := h.agentQueries.GetAgentByID(req.AgentID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "agent not found"})
		return
	}

	// Update agent last_seen timestamp
	if err := h.agentQueries.UpdateAgentLastSeen(req.AgentID); err != nil {
		log.Printf("Warning: Failed to update last_seen for agent %s: %v", req.AgentID, err)
	}

	// Update refresh token expiration (sliding window - reset to 90 days from now)
	// This ensures active agents never need to re-register
	newExpiry := time.Now().Add(90 * 24 * time.Hour)
	if err := h.refreshTokenQueries.UpdateExpiration(refreshToken.ID, newExpiry); err != nil {
		log.Printf("Warning: Failed to update refresh token expiration: %v", err)
	}

	// Generate new access token (24 hours)
	token, err := middleware.GenerateAgentToken(req.AgentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	log.Printf("✅ Token renewed successfully for agent %s (%s)", agent.Hostname, req.AgentID)

	// Return new access token
	response := models.TokenRenewalResponse{
		Token: token,
	}

	c.JSON(http.StatusOK, response)
}

// UnregisterAgent removes an agent from the system
func (h *AgentHandler) UnregisterAgent(c *gin.Context) {
	idStr := c.Param("id")
	agentID, err := uuid.Parse(idStr)
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

	// Store the timestamp when system info was last updated
	agent.Metadata["system_info_updated_at"] = time.Now().Format(time.RFC3339)

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

	log.Printf("✅ System info updated for agent %s (%s): CPU=%s, Cores=%d, Memory=%dMB",
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
	newRapidPollingUntil := time.Now().Add(time.Duration(durationMinutes) * time.Minute)

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
					log.Printf("💓 Heartbeat already active for agent %s (%s), keeping longer duration (expires: %s)",
						agent.Hostname, agentID, currentUntil.Format(time.RFC3339))
					return nil
				}
				// Otherwise extend the heartbeat
				log.Printf("💓 Extending heartbeat for agent %s (%s) from %s to %s",
					agent.Hostname, agentID,
					currentUntil.Format(time.RFC3339),
					newRapidPollingUntil.Format(time.RFC3339))
			}
		}
	} else {
		log.Printf("💓 Enabling heartbeat mode for agent %s (%s) for %d minutes",
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
// TODO: Rate limiting should be implemented for rapid polling endpoints to prevent abuse (technical debt)
func (h *AgentHandler) SetRapidPollingMode(c *gin.Context) {
	idStr := c.Param("id")
	agentID, err := uuid.Parse(idStr)
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
	rapidPollingUntil := time.Now().Add(time.Duration(req.DurationMinutes) * time.Minute)

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

	log.Printf("🚀 Rapid polling mode %s for agent %s (%s) for %d minutes",
		status, agent.Hostname, agentID, duration)

	c.JSON(http.StatusOK, gin.H{
		"message": fmt.Sprintf("Rapid polling mode %s", status),
		"enabled": req.Enabled,
		"duration_minutes": req.DurationMinutes,
		"rapid_polling_until": rapidPollingUntil,
	})
}
