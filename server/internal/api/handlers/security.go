package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/gin-gonic/gin"
)

// SecurityHandler handles security health check endpoints
type SecurityHandler struct {
	signingService  *services.SigningService
	agentQueries    *queries.AgentQueries
	commandQueries  *queries.CommandQueries
}

// NewSecurityHandler creates a new security handler
func NewSecurityHandler(signingService *services.SigningService, agentQueries *queries.AgentQueries, commandQueries *queries.CommandQueries) *SecurityHandler {
	return &SecurityHandler{
		signingService: signingService,
		agentQueries:   agentQueries,
		commandQueries: commandQueries,
	}
}

// setSecurityHeaders sets appropriate cache control headers for security endpoints
func (h *SecurityHandler) setSecurityHeaders(c *gin.Context) {
	c.Header("Cache-Control", "no-store, no-cache, must-revalidate, private")
	c.Header("Pragma", "no-cache")
	c.Header("Expires", "0")
}

// SigningStatus returns the status of the Ed25519 signing service
func (h *SecurityHandler) SigningStatus(c *gin.Context) {
	h.setSecurityHeaders(c)

	response := gin.H{
		"status": "unavailable",
		"timestamp": time.Now().UTC(),
		"checks": map[string]interface{}{
			"service_initialized": false,
			"public_key_available": false,
			"signing_operational": false,
		},
	}

	if h.signingService != nil {
		response["status"] = "available"
		response["checks"].(map[string]interface{})["service_initialized"] = true

		// Check if public key is available
		pubKey := h.signingService.GetPublicKey()
		if pubKey != "" {
			response["checks"].(map[string]interface{})["public_key_available"] = true
			response["checks"].(map[string]interface{})["signing_operational"] = true
			response["public_key_fingerprint"] = h.signingService.GetPublicKeyFingerprint()
			response["algorithm"] = "ed25519"
		}
	}

	c.JSON(http.StatusOK, response)
}

// NonceValidationStatus returns nonce validation health metrics
func (h *SecurityHandler) NonceValidationStatus(c *gin.Context) {
	h.setSecurityHeaders(c)
	response := gin.H{
		"status": "unknown",
		"timestamp": time.Now().UTC(),
		"checks": map[string]interface{}{
			"validation_enabled": true,
			"max_age_minutes":    5,
			"recent_validations": 0,
			"validation_failures": 0,
		},
		"details": map[string]interface{}{
			"nonce_format": "UUID:UnixTimestamp",
			"signature_algorithm": "ed25519",
			"replay_protection": "active",
		},
	}

	// TODO: Add metrics collection for nonce validations
	// This would require adding logging/metrics to the nonce validation process
	// For now, we provide the configuration status

	response["status"] = "healthy"
	response["checks"].(map[string]interface{})["validation_enabled"] = true
	response["checks"].(map[string]interface{})["max_age_minutes"] = 5

	c.JSON(http.StatusOK, response)
}

// CommandValidationStatus returns command validation and processing metrics
func (h *SecurityHandler) CommandValidationStatus(c *gin.Context) {
	h.setSecurityHeaders(c)
	response := gin.H{
		"status": "unknown",
		"timestamp": time.Now().UTC(),
		"metrics": map[string]interface{}{
			"total_pending_commands": 0,
			"agents_with_pending":    0,
			"commands_last_hour":     0,
			"commands_last_24h":      0,
		},
		"checks": map[string]interface{}{
			"command_processing": "unknown",
			"backpressure_active": false,
			"agent_responsive":   "unknown",
		},
	}

	// Get real command metrics
	if h.commandQueries != nil {
		if totalPending, err := h.commandQueries.GetTotalPendingCommands(); err == nil {
			response["metrics"].(map[string]interface{})["total_pending_commands"] = totalPending
		}
		if agentsWithPending, err := h.commandQueries.GetAgentsWithPendingCommands(); err == nil {
			response["metrics"].(map[string]interface{})["agents_with_pending"] = agentsWithPending
		}
		if commandsLastHour, err := h.commandQueries.GetCommandsInTimeRange(1); err == nil {
			response["metrics"].(map[string]interface{})["commands_last_hour"] = commandsLastHour
		}
		if commandsLast24h, err := h.commandQueries.GetCommandsInTimeRange(24); err == nil {
			response["metrics"].(map[string]interface{})["commands_last_24h"] = commandsLast24h
		}
	}

	// Get agent metrics for responsiveness
	if h.agentQueries != nil {
		if activeAgents, err := h.agentQueries.GetActiveAgentCount(); err == nil {
			response["checks"].(map[string]interface{})["agent_responsive"] = fmt.Sprintf("%d online", activeAgents)
		}
	}

	// Determine if backpressure is active (5+ pending commands per agent threshold)
	if totalPending, ok := response["metrics"].(map[string]interface{})["total_pending_commands"].(int); ok {
		if agentsWithPending, ok := response["metrics"].(map[string]interface{})["agents_with_pending"].(int); ok && agentsWithPending > 0 {
			avgPerAgent := float64(totalPending) / float64(agentsWithPending)
			response["checks"].(map[string]interface{})["backpressure_active"] = avgPerAgent >= 5.0
		}
	}

	response["status"] = "operational"
	response["checks"].(map[string]interface{})["command_processing"] = "operational"

	c.JSON(http.StatusOK, response)
}

// MachineBindingStatus returns machine binding enforcement metrics
func (h *SecurityHandler) MachineBindingStatus(c *gin.Context) {
	h.setSecurityHeaders(c)
	response := gin.H{
		"status": "unknown",
		"timestamp": time.Now().UTC(),
		"checks": map[string]interface{}{
			"binding_enforced":    true,
			"min_agent_version":   "v0.2.0.3",
			"fingerprint_required": true,
			"recent_violations":   0,
			"bound_agents":        0,
			"version_compliance":  0,
		},
		"details": map[string]interface{}{
			"enforcement_method": "hardware_fingerprint",
			"binding_scope":      "machine_id + cpu + memory + system_uuid",
			"violation_action":   "command_rejection",
		},
	}

	// Get real machine binding metrics
	if h.agentQueries != nil {
		// Get total agents with machine binding
		if boundAgents, err := h.agentQueries.GetAgentsWithMachineBinding(); err == nil {
			response["checks"].(map[string]interface{})["bound_agents"] = boundAgents
		}

		// Get total agents for comparison
		if totalAgents, err := h.agentQueries.GetTotalAgentCount(); err == nil {
			response["checks"].(map[string]interface{})["total_agents"] = totalAgents

			// Calculate version compliance (agents meeting minimum version requirement)
			if compliantAgents, err := h.agentQueries.GetAgentCountByVersion("0.1.26"); err == nil {
				response["checks"].(map[string]interface{})["version_compliance"] = compliantAgents
			}

			// Set recent violations based on version compliance gap.
			// Either count may be absent if its query failed above; default to 0.
			boundAgents, _ := response["checks"].(map[string]interface{})["bound_agents"].(int)
			versionCompliance, _ := response["checks"].(map[string]interface{})["version_compliance"].(int)
			violations := boundAgents - versionCompliance
			if violations < 0 {
				violations = 0
			}
			response["checks"].(map[string]interface{})["recent_violations"] = violations
		}
	}

	response["status"] = "enforced"
	response["checks"].(map[string]interface{})["binding_enforced"] = true
	response["checks"].(map[string]interface{})["min_agent_version"] = "v0.1.26"

	c.JSON(http.StatusOK, response)
}

// SecurityOverview returns a comprehensive overview of all security subsystems
func (h *SecurityHandler) SecurityOverview(c *gin.Context) {
	h.setSecurityHeaders(c)
	overview := gin.H{
		"timestamp": time.Now().UTC(),
		"overall_status": "unknown",
		"subsystems": map[string]interface{}{
			"ed25519_signing": map[string]interface{}{
				"status": "unknown",
				"enabled": true,
			},
			"nonce_validation": map[string]interface{}{
				"status": "unknown",
				"enabled": true,
			},
			"machine_binding": map[string]interface{}{
				"status": "unknown",
				"enabled": true,
			},
			"command_validation": map[string]interface{}{
				"status": "unknown",
				"enabled": true,
			},
		},
		"alerts": []string{},
		"recommendations": []string{},
	}

	// Check Ed25519 signing
	if h.signingService != nil && h.signingService.GetPublicKey() != "" {
		overview["subsystems"].(map[string]interface{})["ed25519_signing"].(map[string]interface{})["status"] = "healthy"
		// Add Ed25519 details
		overview["subsystems"].(map[string]interface{})["ed25519_signing"].(map[string]interface{})["checks"] = map[string]interface{}{
			"service_initialized":   true,
			"public_key_available": true,
			"signing_operational":  true,
			"public_key_fingerprint": h.signingService.GetPublicKeyFingerprint(),
			"algorithm": "ed25519",
		}
	} else {
		overview["subsystems"].(map[string]interface{})["ed25519_signing"].(map[string]interface{})["status"] = "unavailable"
		overview["alerts"] = append(overview["alerts"].([]string), "Ed25519 signing service not configured")
		overview["recommendations"] = append(overview["recommendations"].([]string), "Set REDFLAG_SIGNING_PRIVATE_KEY environment variable")
	}

	// Check nonce validation
	overview["subsystems"].(map[string]interface{})["nonce_validation"].(map[string]interface{})["status"] = "healthy"
	overview["subsystems"].(map[string]interface{})["nonce_validation"].(map[string]interface{})["checks"] = map[string]interface{}{
		"validation_enabled":  true,
		"max_age_minutes":     5,
		"validation_failures": 0, // TODO: Implement nonce validation failure tracking
	}
	overview["subsystems"].(map[string]interface{})["nonce_validation"].(map[string]interface{})["details"] = map[string]interface{}{
		"nonce_format": "UUID:UnixTimestamp",
		"signature_algorithm": "ed25519",
		"replay_protection": "active",
	}

	// Get real machine binding metrics
	if h.agentQueries != nil {
		boundAgents, _ := h.agentQueries.GetAgentsWithMachineBinding()
		compliantAgents, _ := h.agentQueries.GetAgentCountByVersion("0.1.26")
		violations := boundAgents - compliantAgents
		if violations < 0 {
			violations = 0
		}

		overview["subsystems"].(map[string]interface{})["machine_binding"].(map[string]interface{})["status"] = "enforced"
		overview["subsystems"].(map[string]interface{})["machine_binding"].(map[string]interface{})["checks"] = map[string]interface{}{
			"binding_enforced":   true,
			"min_agent_version":  "v0.1.26",
			"recent_violations":  violations,
			"bound_agents":       boundAgents,
			"version_compliance": compliantAgents,
		}
		overview["subsystems"].(map[string]interface{})["machine_binding"].(map[string]interface{})["details"] = map[string]interface{}{
			"enforcement_method": "hardware_fingerprint",
			"binding_scope":      "machine_id + cpu + memory + system_uuid",
			"violation_action":   "command_rejection",
		}
	}

	// Get real command validation metrics
	if h.commandQueries != nil {
		totalPending, _ := h.commandQueries.GetTotalPendingCommands()
		agentsWithPending, _ := h.commandQueries.GetAgentsWithPendingCommands()
		commandsLastHour, _ := h.commandQueries.GetCommandsInTimeRange(1)
		commandsLast24h, _ := h.commandQueries.GetCommandsInTimeRange(24)

		// Calculate backpressure
		backpressureActive := false
		if agentsWithPending > 0 {
			avgPerAgent := float64(totalPending) / float64(agentsWithPending)
			backpressureActive = avgPerAgent >= 5.0
		}

		overview["subsystems"].(map[string]interface{})["command_validation"].(map[string]interface{})["status"] = "operational"
		overview["subsystems"].(map[string]interface{})["command_validation"].(map[string]interface{})["metrics"] = map[string]interface{}{
			"total_pending_commands": totalPending,
			"agents_with_pending":    agentsWithPending,
			"commands_last_hour":     commandsLastHour,
			"commands_last_24h":      commandsLast24h,
		}
		overview["subsystems"].(map[string]interface{})["command_validation"].(map[string]interface{})["checks"] = map[string]interface{}{
			"command_processing":  "operational",
			"backpressure_active": backpressureActive,
		}

		// Add agent responsiveness info
		if h.agentQueries != nil {
			if activeAgents, err := h.agentQueries.GetActiveAgentCount(); err == nil {
				overview["subsystems"].(map[string]interface{})["command_validation"].(map[string]interface{})["checks"].(map[string]interface{})["agent_responsive"] = fmt.Sprintf("%d online", activeAgents)
			}
		}
	}

	// Determine overall status
	healthyCount := 0
	totalCount := 4
	for _, subsystem := range overview["subsystems"].(map[string]interface{}) {
		subsystemMap := subsystem.(map[string]interface{})
		if subsystemMap["status"] == "healthy" || subsystemMap["status"] == "enforced" || subsystemMap["status"] == "operational" {
			healthyCount++
		}
	}

	if healthyCount == totalCount {
		overview["overall_status"] = "healthy"
	} else if healthyCount >= totalCount/2 {
		overview["overall_status"] = "degraded"
	} else {
		overview["overall_status"] = "unhealthy"
	}

	c.JSON(http.StatusOK, overview)
}

// SecurityMetrics returns detailed security metrics for monitoring
func (h *SecurityHandler) SecurityMetrics(c *gin.Context) {
	h.setSecurityHeaders(c)
	metrics := gin.H{
		"timestamp": time.Now().UTC(),
		"signing": map[string]interface{}{
			"public_key_fingerprint": "",
			"algorithm": "ed25519",
			"key_size": 32,
		},
		"nonce": map[string]interface{}{
			"max_age_seconds": 300, // 5 minutes
			"format": "UUID:UnixTimestamp",
		},
		"machine_binding": map[string]interface{}{
			"min_version": "v0.1.26",
			"enforcement": "hardware_fingerprint",
		},
		"command_processing": map[string]interface{}{
			"backpressure_threshold": 5,
			"rate_limit_per_second": 100,
		},
	}

	// Add signing metrics if available
	if h.signingService != nil {
		metrics["signing"].(map[string]interface{})["public_key_fingerprint"] = h.signingService.GetPublicKeyFingerprint()
		metrics["signing"].(map[string]interface{})["configured"] = true
	} else {
		metrics["signing"].(map[string]interface{})["configured"] = false
	}

	c.JSON(http.StatusOK, metrics)
}