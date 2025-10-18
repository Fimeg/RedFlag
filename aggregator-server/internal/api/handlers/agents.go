package handlers

import (
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
		CPUPercent    float64 `json:"cpu_percent,omitempty"`
		MemoryPercent float64 `json:"memory_percent,omitempty"`
		MemoryUsedGB  float64 `json:"memory_used_gb,omitempty"`
		MemoryTotalGB float64 `json:"memory_total_gb,omitempty"`
		DiskUsedGB    float64 `json:"disk_used_gb,omitempty"`
		DiskTotalGB   float64 `json:"disk_total_gb,omitempty"`
		DiskPercent   float64 `json:"disk_percent,omitempty"`
		Uptime        string  `json:"uptime,omitempty"`
		Version       string  `json:"version,omitempty"`
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
		// Get current agent to preserve existing metadata
		agent, err := h.agentQueries.GetAgentByID(agentID)
		if err == nil && agent.Metadata != nil {
			// Update agent's current version
			if err := h.agentQueries.UpdateAgentVersion(agentID, metrics.Version); err != nil {
				log.Printf("Warning: Failed to update agent version: %v", err)
			} else {
				// Check if update is available
				updateAvailable := utils.IsNewerVersion(h.latestAgentVersion, metrics.Version)

				// Update agent's update availability status
				if err := h.agentQueries.UpdateAgentUpdateAvailable(agentID, updateAvailable); err != nil {
					log.Printf("Warning: Failed to update agent update availability: %v", err)
				}

				// Log version check
				if updateAvailable {
					log.Printf("🔄 Agent %s (%s) version %s has update available: %s",
						agent.Hostname, agentID, metrics.Version, h.latestAgentVersion)
				} else {
					log.Printf("✅ Agent %s (%s) version %s is up to date",
						agent.Hostname, agentID, metrics.Version)
				}

				// Store version in metadata as well
				agent.Metadata["reported_version"] = metrics.Version
				agent.Metadata["latest_version"] = h.latestAgentVersion
				agent.Metadata["update_available"] = updateAvailable
				agent.Metadata["version_checked_at"] = time.Now().Format(time.RFC3339)
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

	response := models.CommandsResponse{
		Commands: commandItems,
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
