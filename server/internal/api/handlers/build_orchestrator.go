package handlers

import (
	"fmt"
	"net/http"

	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/gin-gonic/gin"
)

// NewAgentBuild handles new agent installation requests
// Deprecated: Use AgentHandler.Upgrade instead
func NewAgentBuild(c *gin.Context) {
	var req services.NewBuildRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate registration token
	if req.RegistrationToken == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "registration token is required for new installations"})
		return
	}

	// Convert to setup request format
	setupReq := services.AgentSetupRequest{
		ServerURL:     req.ServerURL,
		Environment:   req.Environment,
		AgentType:     req.AgentType,
		Organization:  req.Organization,
		CustomSettings: req.CustomSettings,
		DeploymentID:  req.DeploymentID,
	}

	// Create config builder
	configBuilder := services.NewConfigBuilder(req.ServerURL, nil)

	// Build agent configuration
	config, err := configBuilder.BuildAgentConfig(setupReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Override generated agent ID if provided (for upgrades)
	if req.AgentID != "" {
		config.AgentID = req.AgentID
		// Update public config with existing agent ID
		if config.PublicConfig == nil {
			config.PublicConfig = make(map[string]interface{})
		}
		config.PublicConfig["agent_id"] = req.AgentID
	}

	// Create agent builder
	agentBuilder := services.NewAgentBuilder()

	// Generate build artifacts
	buildResult, err := agentBuilder.BuildAgentWithConfig(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Construct download URL
	binaryURL := fmt.Sprintf("%s/api/v1/downloads/%s", req.ServerURL, config.Platform)

	// Create response with native binary instructions
	response := gin.H{
		"agent_id":        config.AgentID,
		"binary_url":      binaryURL,
		"platform":        config.Platform,
		"config_version":  config.ConfigVersion,
		"agent_version":   config.AgentVersion,
		"build_time":      buildResult.BuildTime,
		"install_type":    "new",
		"consumes_seat":   true,
		"next_steps": []string{
			"1. Download native binary: curl -sL " + binaryURL + " -o /usr/local/bin/redflag-agent",
			"2. Set permissions: chmod 755 /usr/local/bin/redflag-agent",
			"3. Create config directory: mkdir -p /etc/redflag",
			"4. Save configuration (provided in this response) to /etc/redflag/config.json",
			"5. Set config permissions: chmod 600 /etc/redflag/config.json",
			"6. Start service: systemctl enable --now redflag-agent",
		},
		"configuration": config.PublicConfig,
	}

	c.JSON(http.StatusOK, response)
}

// UpgradeAgentBuild handles agent upgrade requests
// Deprecated: superseded by AgentHandler.Rebuild (ConfigService was removed unwired)
func UpgradeAgentBuild(c *gin.Context) {
	agentID := c.Param("agentID")
	if agentID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "agent ID is required"})
		return
	}

	var req services.UpgradeBuildRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate required fields
	if req.ServerURL == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "server URL is required"})
		return
	}

	// Convert to setup request format
	setupReq := services.AgentSetupRequest{
		ServerURL:     req.ServerURL,
		Environment:   req.Environment,
		AgentType:     req.AgentType,
		Organization:  req.Organization,
		CustomSettings: req.CustomSettings,
		DeploymentID:  req.DeploymentID,
	}

	// Create config builder
	configBuilder := services.NewConfigBuilder(req.ServerURL, nil)

	// Build agent configuration
	config, err := configBuilder.BuildAgentConfig(setupReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Override with existing agent ID (this is the key for upgrades)
	config.AgentID = agentID
	if config.PublicConfig == nil {
		config.PublicConfig = make(map[string]interface{})
	}
	config.PublicConfig["agent_id"] = agentID

	// For upgrades, we might want to preserve certain existing settings
	if req.PreserveExisting {
		// TODO: Load existing agent config and merge/override as needed
		// This would involve reading the existing agent's configuration
		// and selectively preserving certain fields
	}

	// Create agent builder
	agentBuilder := services.NewAgentBuilder()

	// Generate build artifacts
	buildResult, err := agentBuilder.BuildAgentWithConfig(config)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// Construct download URL
	binaryURL := fmt.Sprintf("%s/api/v1/downloads/%s?version=%s", req.ServerURL, config.Platform, config.AgentVersion)

	// Create response with native binary upgrade instructions
	response := gin.H{
		"agent_id":           config.AgentID,
		"binary_url":         binaryURL,
		"platform":           config.Platform,
		"config_version":     config.ConfigVersion,
		"agent_version":      config.AgentVersion,
		"build_time":         buildResult.BuildTime,
		"install_type":       "upgrade",
		"consumes_seat":      false,
		"preserves_agent_id": true,
		"next_steps": []string{
			"1. Stop agent service: systemctl stop redflag-agent",
			"2. Download updated binary: curl -sL " + binaryURL + " -o /usr/local/bin/redflag-agent",
			"3. Set permissions: chmod 755 /usr/local/bin/redflag-agent",
			"4. Update config (provided in this response) to /etc/redflag/config.json if needed",
			"5. Start service: systemctl start redflag-agent",
			"6. Verify: systemctl status redflag-agent",
		},
		"configuration": config.PublicConfig,
		"upgrade_notes": []string{
			"This upgrade preserves the existing agent ID: " + agentID,
			"No additional seat will be consumed",
			"Config version: " + config.ConfigVersion,
			"Agent binary version: " + config.AgentVersion,
			"Agent will receive latest security enhancements and bug fixes",
		},
	}

	c.JSON(http.StatusOK, response)
}

// DetectAgentInstallation detects existing agent installations
func DetectAgentInstallation(c *gin.Context) {
	// This endpoint helps the installer determine what type of installation to perform
	var req struct {
		AgentID string `json:"agent_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Create detector service
	detector := services.NewInstallationDetector()

	// Detect existing installation
	detection, err := detector.DetectExistingInstallation(req.AgentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := gin.H{
		"detection_result": detection,
		"recommended_action": func() string {
			if detection.HasExistingAgent {
				return "upgrade"
			}
			return "new_installation"
		}(),
		"installation_type": func() string {
			if detection.HasExistingAgent {
				return "upgrade"
			}
			return "new"
		}(),
	}

	c.JSON(http.StatusOK, response)
}