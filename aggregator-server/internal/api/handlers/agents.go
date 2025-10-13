package handlers

import (
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

// ListAgents returns all agents
func (h *AgentHandler) ListAgents(c *gin.Context) {
	status := c.Query("status")
	osType := c.Query("os_type")

	agents, err := h.agentQueries.ListAgents(status, osType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list agents"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"agents": agents,
		"total":  len(agents),
	})
}

// GetAgent returns a single agent by ID
func (h *AgentHandler) GetAgent(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	agent, err := h.agentQueries.GetAgentByID(id)
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
