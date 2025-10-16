package handlers

import (
	"log"
	"net/http"
	"time"

	"github.com/aggregator-project/aggregator-server/internal/api/middleware"
	"github.com/aggregator-project/aggregator-server/internal/database/queries"
	"github.com/aggregator-project/aggregator-server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AgentHandler struct {
	agentQueries   *queries.AgentQueries
	commandQueries *queries.CommandQueries
	checkInInterval int
}

func NewAgentHandler(aq *queries.AgentQueries, cq *queries.CommandQueries, checkInInterval int) *AgentHandler {
	return &AgentHandler{
		agentQueries:   aq,
		commandQueries: cq,
		checkInInterval: checkInInterval,
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

	// Generate JWT token
	token, err := middleware.GenerateAgentToken(agent.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to generate token"})
		return
	}

	// Return response
	response := models.AgentRegistrationResponse{
		AgentID: agent.ID,
		Token:   token,
		Config: map[string]interface{}{
			"check_in_interval": h.checkInInterval,
			"server_url":        c.Request.Host,
		},
	}

	c.JSON(http.StatusOK, response)
}

// GetCommands returns pending commands for an agent
func (h *AgentHandler) GetCommands(c *gin.Context) {
	agentID := c.MustGet("agent_id").(uuid.UUID)

	// Update last_seen
	if err := h.agentQueries.UpdateAgentLastSeen(agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update last seen"})
		return
	}
	log.Printf("Updated last_seen for agent %s", agentID)

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
