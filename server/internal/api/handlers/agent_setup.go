package handlers

import (
	"net/http"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/gin-gonic/gin"
)

// AgentSetupHandler handles agent setup operations
type AgentSetupHandler struct {
	agentQueries *queries.AgentQueries
}

// NewAgentSetupHandler creates a new agent setup handler
func NewAgentSetupHandler(agentQueries *queries.AgentQueries) *AgentSetupHandler {
	return &AgentSetupHandler{
		agentQueries: agentQueries,
	}
}

// SetupAgent handles the agent setup endpoint
func (h *AgentSetupHandler) SetupAgent(c *gin.Context) {
	var req services.AgentSetupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create config builder with database access
	configBuilder := services.NewConfigBuilder(req.ServerURL, h.agentQueries.DB)

	// Build agent configuration
	config, err := configBuilder.BuildAgentConfig(req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Create response
	response := gin.H{
		"agent_id":            config.AgentID,
		"registration_token":  config.Secrets["registration_token"],
		"server_public_key":    config.Secrets["server_public_key"],
		"configuration":      config.PublicConfig,
		"secrets":            config.Secrets,
		"template":           config.Template,
		"setup_time":          config.BuildTime,
		"secrets_created":     config.SecretsCreated,
		"secrets_path":        config.SecretsPath,
	}

	c.JSON(http.StatusOK, response)
}

// GetTemplates returns available agent templates
func (h *AgentSetupHandler) GetTemplates(c *gin.Context) {
	configBuilder := services.NewConfigBuilder("", h.agentQueries.DB)
	templates := configBuilder.GetTemplates()
	c.JSON(http.StatusOK, gin.H{"templates": templates})
}

// ValidateConfiguration validates a configuration before deployment
func (h *AgentSetupHandler) ValidateConfiguration(c *gin.Context) {
	var config map[string]interface{}
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	agentType, exists := config["agent_type"].(string)
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent_type is required"})
		return
	}

	configBuilder := services.NewConfigBuilder("", h.agentQueries.DB)
	template, exists := configBuilder.GetTemplate(agentType)
	if !exists {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown agent type"})
		return
	}

	// Simple validation response
	c.JSON(http.StatusOK, gin.H{
		"valid":   true,
		"message": "Configuration appears valid",
		"agent_type": agentType,
		"template": template.Name,
	})
}
