package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/command"
	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/Fimeg/RedFlag/server/internal/scheduler"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/Fimeg/RedFlag/server/internal/logging"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

type SubsystemHandler struct {
	subsystemQueries *queries.SubsystemQueries
	commandQueries   *queries.CommandQueries
	agentQueries     *queries.AgentQueries
	commandFactory   *command.Factory
	signingService   *services.SigningService
	securityLogger   *logging.SecurityLogger
	scheduler        *scheduler.Scheduler
}

func NewSubsystemHandler(sq *queries.SubsystemQueries, cq *queries.CommandQueries, aq *queries.AgentQueries, cf *command.Factory, signingService *services.SigningService, securityLogger *logging.SecurityLogger) *SubsystemHandler {
	return &SubsystemHandler{
		subsystemQueries: sq,
		commandQueries:   cq,
		agentQueries:     aq,
		commandFactory:   cf,
		signingService:   signingService,
		securityLogger:   securityLogger,
	}
}

// SetScheduler injects the scheduler reference so DisableSubsystem can evict
// jobs from the in-memory queue. Nil-safe — a nil scheduler is a no-op.
func (h *SubsystemHandler) SetScheduler(s *scheduler.Scheduler) {
	h.scheduler = s
}

// signAndCreateCommand signs a command before storing.
// STRICT MODE: Commands without signatures are rejected (ETHOS #2 Security is Non-Negotiable)
func (h *SubsystemHandler) signAndCreateCommand(cmd *models.AgentCommand) error {
	// Generate ID if not set (prevents zero UUID issues)
	if cmd.ID == uuid.Nil {
		cmd.ID = uuid.Must(uuid.NewV4())
	}

	// Set timestamps if not set
	if cmd.CreatedAt.IsZero() {
		cmd.CreatedAt = time.Now().UTC()
	}
	if cmd.UpdatedAt.IsZero() {
		cmd.UpdatedAt = time.Now().UTC()
	}

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

	return nil
}

// GetSubsystems retrieves all subsystems for an agent
// GET /api/v1/agents/:id/subsystems
func (h *SubsystemHandler) GetSubsystems(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	subsystems, err := h.subsystemQueries.GetSubsystems(agentID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve subsystems"})
		return
	}

	c.JSON(http.StatusOK, subsystems)
}

// GetSubsystem retrieves a specific subsystem for an agent
// GET /api/v1/agents/:id/subsystems/:subsystem
func (h *SubsystemHandler) GetSubsystem(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	subsystem := c.Param("subsystem")
	if subsystem == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subsystem name required"})
		return
	}

	sub, err := h.subsystemQueries.GetSubsystem(agentID, subsystem)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve subsystem"})
		return
	}

	if sub == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subsystem not found"})
		return
	}

	c.JSON(http.StatusOK, sub)
}

// UpdateSubsystem updates subsystem configuration
// PATCH /api/v1/agents/:id/subsystems/:subsystem
func (h *SubsystemHandler) UpdateSubsystem(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	subsystem := c.Param("subsystem")
	if subsystem == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subsystem name required"})
		return
	}

	var config models.SubsystemConfig
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate interval if provided
	if config.IntervalMinutes != nil && (*config.IntervalMinutes < 5 || *config.IntervalMinutes > 1440) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Interval must be between 5 and 1440 minutes"})
		return
	}

	err = h.subsystemQueries.UpdateSubsystem(agentID, subsystem, config)
	if err != nil {
		if err.Error() == "subsystem not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Subsystem not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update subsystem"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Subsystem updated successfully"})
}

// EnableSubsystem enables a subsystem
// POST /api/v1/agents/:id/subsystems/:subsystem/enable
func (h *SubsystemHandler) EnableSubsystem(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	subsystem := c.Param("subsystem")
	if subsystem == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subsystem name required"})
		return
	}

	err = h.subsystemQueries.EnableSubsystem(agentID, subsystem)
	if err != nil {
		if err.Error() == "subsystem not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Subsystem not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to enable subsystem"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Subsystem enabled successfully"})
}

// DisableSubsystem disables a subsystem
// POST /api/v1/agents/:id/subsystems/:subsystem/disable
func (h *SubsystemHandler) DisableSubsystem(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	subsystem := c.Param("subsystem")
	if subsystem == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subsystem name required"})
		return
	}

	err = h.subsystemQueries.DisableSubsystem(agentID, subsystem)
	if err != nil {
		if err.Error() == "subsystem not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Subsystem not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to disable subsystem"})
		return
	}

	// Evict the scheduled job from the in-memory scheduler so it stops generating
	// commands immediately — the DB flip alone isn't enough since the scheduler's
	// priority queue only re-reads state on restart.
	if h.scheduler != nil {
		if removed := h.scheduler.RemoveSubsystemJob(agentID, subsystem); removed {
			log.Printf("[INFO] [server] [subsystems] scheduler_job_removed agent_id=%s subsystem=%s",
				agentID, subsystem)
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "Subsystem disabled successfully"})
}

// getScanCommandType determines the correct scan command type based on subsystem and agent platform.
// BUG-015 FIX: Maps 'updates' subsystem to platform-specific scan commands (apt, dnf, windows, winget)
func (h *SubsystemHandler) getScanCommandType(subsystem string, agentID uuid.UUID) (string, error) {
	// Non-update subsystems use direct mapping
	if subsystem != "updates" {
		return "scan_" + subsystem, nil
	}

	// For 'updates' subsystem, we need to query the agent's OS type
	if h.agentQueries == nil {
		return "", fmt.Errorf("agent queries not available for platform detection")
	}

	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		return "", fmt.Errorf("failed to retrieve agent for platform detection: %w", err)
	}
	if agent == nil {
		return "", fmt.Errorf("agent not found for platform detection")
	}

	osType := agent.OSType
	log.Printf("[INFO] [server] [scan_updates] platform_detection agent_id=%s os_type=%s", agentID, osType)

	// Map OS type to appropriate scan command
	osLower := ""
	if osType != "" {
		osLower = strings.ToLower(osType)
	}

	switch {
	case strings.Contains(osLower, "debian"), strings.Contains(osLower, "ubuntu"):
		return "scan_apt", nil
	case strings.Contains(osLower, "fedora"), strings.Contains(osLower, "rhel"), strings.Contains(osLower, "centos"):
		return "scan_dnf", nil
	case strings.Contains(osLower, "windows"):
		return "scan_windows", nil
	default:
		// Fallback: try to guess from other indicators
		if strings.Contains(osLower, "arch") || strings.Contains(osLower, "manjaro") {
			return "scan_apt", nil // Arch can use pacman but apt is closest in structure
		}
		log.Printf("[WARNING] [server] [scan_updates] unknown_platform agent_id=%s os_type=%s defaulting_to_apt", agentID, osType)
		return "scan_apt", nil // Conservative fallback
	}
}

// TriggerSubsystem manually triggers a subsystem scan
// POST /api/v1/agents/:id/subsystems/:subsystem/trigger
// BUG-015 FIX: Supports platform-specific scanners (apt, dnf, windows, winget)
// Frontend maps 'updates' → platform scanner, this handler creates scan_<platform> commands
func (h *SubsystemHandler) TriggerSubsystem(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	subsystem := c.Param("subsystem")
	if subsystem == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subsystem name required"})
		return
	}

	// Verify subsystem exists and is enabled
	sub, err := h.subsystemQueries.GetSubsystem(agentID, subsystem)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve subsystem"})
		return
	}

	if sub == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subsystem not found"})
		return
	}

	if !sub.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subsystem is disabled"})
		return
	}

	// BUG-015 FIX: Get platform-specific scan command type
	commandType, err := h.getScanCommandType(subsystem, agentID)
	if err != nil {
		log.Printf("[ERROR] [server] [scan] platform_detection_failed agent_id=%s subsystem=%s error=%v", agentID, subsystem, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to determine scan command type: %v", err)})
		return
	}

	idempotencyKey := fmt.Sprintf("%s_%s_%d", agentID.String(), subsystem, time.Now().UTC().Unix())

	// Log command creation attempt
	log.Printf("[INFO] [server] [command] creating_scan_command agent_id=%s subsystem=%s command_type=%s timestamp=%s",
		agentID, subsystem, commandType, time.Now().UTC().Format(time.RFC3339))
	log.Printf("[HISTORY] [server] [scan_%s] command_creation_started agent_id=%s timestamp=%s",
		subsystem, agentID, time.Now().UTC().Format(time.RFC3339))

	command, err := h.commandFactory.CreateWithIdempotency(
		agentID,
		commandType,
		map[string]interface{}{"subsystem": subsystem},
		idempotencyKey,
	)
	if err != nil {
		log.Printf("[ERROR] [server] [scan_%s] command_creation_failed agent_id=%s error=%v", subsystem, agentID, err)
		log.Printf("[HISTORY] [server] [scan_%s] command_creation_failed error=\"%v\" timestamp=%s",
			subsystem, err, time.Now().UTC().Format(time.RFC3339))

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to create %s scan command: %v", subsystem, err),
		})
		return
	}

	err = h.signAndCreateCommand(command)
	if err != nil {
		log.Printf("[ERROR] [server] [scan_%s] command_creation_failed agent_id=%s error=%v", subsystem, agentID, err)
		log.Printf("[HISTORY] [server] [scan_%s] command_creation_failed error=\"%v\" timestamp=%s",
			subsystem, err, time.Now().UTC().Format(time.RFC3339))

		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("Failed to create %s scan command: %v", subsystem, err),
		})
		return
	}

	log.Printf("[SUCCESS] [server] [scan_%s] command_created agent_id=%s command_id=%s timestamp=%s",
		subsystem, agentID, command.ID, time.Now().UTC().Format(time.RFC3339))
	log.Printf("[HISTORY] [server] [scan_%s] command_created agent_id=%s command_id=%s timestamp=%s",
		subsystem, agentID, command.ID, time.Now().UTC().Format(time.RFC3339))

	c.JSON(http.StatusOK, gin.H{
		"message":    "Subsystem scan triggered successfully",
		"command_id": command.ID,
	})
}

// GetSubsystemStats retrieves statistics for a subsystem
// GET /api/v1/agents/:id/subsystems/:subsystem/stats
func (h *SubsystemHandler) GetSubsystemStats(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	subsystem := c.Param("subsystem")
	if subsystem == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subsystem name required"})
		return
	}

	stats, err := h.subsystemQueries.GetSubsystemStats(agentID, subsystem)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve subsystem stats"})
		return
	}

	if stats == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Subsystem not found"})
		return
	}

	c.JSON(http.StatusOK, stats)
}

// SetAutoRun enables or disables auto-run for a subsystem
// POST /api/v1/agents/:id/subsystems/:subsystem/auto-run
func (h *SubsystemHandler) SetAutoRun(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	subsystem := c.Param("subsystem")
	if subsystem == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subsystem name required"})
		return
	}

	var req struct {
		AutoRun bool `json:"auto_run"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	err = h.subsystemQueries.SetAutoRun(agentID, subsystem, req.AutoRun)
	if err != nil {
		if err.Error() == "subsystem not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Subsystem not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update auto-run"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Auto-run updated successfully"})
}

// SetInterval sets the interval for a subsystem
// POST /api/v1/agents/:id/subsystems/:subsystem/interval
func (h *SubsystemHandler) SetInterval(c *gin.Context) {
	agentID, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid agent ID"})
		return
	}

	subsystem := c.Param("subsystem")
	if subsystem == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Subsystem name required"})
		return
	}

	var req struct {
		IntervalMinutes int `json:"interval_minutes"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate interval
	if req.IntervalMinutes < 5 || req.IntervalMinutes > 1440 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Interval must be between 5 and 1440 minutes"})
		return
	}

	err = h.subsystemQueries.SetInterval(agentID, subsystem, req.IntervalMinutes)
	if err != nil {
		if err.Error() == "subsystem not found" {
			c.JSON(http.StatusNotFound, gin.H{"error": "Subsystem not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update interval"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Interval updated successfully"})
}
