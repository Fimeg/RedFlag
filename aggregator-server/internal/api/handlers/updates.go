package handlers

import (
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/Fimeg/RedFlag/aggregator-server/internal/database/queries"
	"github.com/Fimeg/RedFlag/aggregator-server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UpdateHandler struct {
	updateQueries  *queries.UpdateQueries
	agentQueries   *queries.AgentQueries
	commandQueries *queries.CommandQueries
	agentHandler   *AgentHandler
}

func NewUpdateHandler(uq *queries.UpdateQueries, aq *queries.AgentQueries, cq *queries.CommandQueries, ah *AgentHandler) *UpdateHandler {
	return &UpdateHandler{
		updateQueries:  uq,
		agentQueries:   aq,
		commandQueries: cq,
		agentHandler:   ah,
	}
}

// shouldEnableHeartbeat checks if heartbeat is already active for an agent
// Returns true if heartbeat should be enabled (i.e., not already active or expired)
func (h *UpdateHandler) shouldEnableHeartbeat(agentID uuid.UUID, durationMinutes int) (bool, error) {
	agent, err := h.agentQueries.GetAgentByID(agentID)
	if err != nil {
		log.Printf("Warning: Failed to get agent %s for heartbeat check: %v", agentID, err)
		return true, nil // Enable heartbeat by default if we can't check
	}

	// Check if rapid polling is already enabled and not expired
	if enabled, ok := agent.Metadata["rapid_polling_enabled"].(bool); ok && enabled {
		if untilStr, ok := agent.Metadata["rapid_polling_until"].(string); ok {
			until, err := time.Parse(time.RFC3339, untilStr)
			if err == nil && until.After(time.Now().Add(5*time.Minute)) {
				// Heartbeat is already active for sufficient time
				log.Printf("[Heartbeat] Agent %s already has active heartbeat until %s (skipping)", agentID, untilStr)
				return false, nil
			}
		}
	}

	return true, nil
}

// ReportUpdates handles update reports from agents using event sourcing
func (h *UpdateHandler) ReportUpdates(c *gin.Context) {
	agentID := c.MustGet("agent_id").(uuid.UUID)

	// Update last_seen timestamp
	if err := h.agentQueries.UpdateAgentLastSeen(agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update last seen"})
		return
	}

	var req models.UpdateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert update report items to events
	events := make([]models.UpdateEvent, 0, len(req.Updates))
	for _, item := range req.Updates {
		event := models.UpdateEvent{
			ID:               uuid.New(),
			AgentID:          agentID,
			PackageType:      item.PackageType,
			PackageName:      item.PackageName,
			VersionFrom:      item.CurrentVersion,
			VersionTo:        item.AvailableVersion,
			Severity:         item.Severity,
			RepositorySource: item.RepositorySource,
			Metadata:         item.Metadata,
			EventType:        "discovered",
			CreatedAt:        req.Timestamp,
		}
		events = append(events, event)
	}

	// Store events in batch with error isolation
	if err := h.updateQueries.CreateUpdateEventsBatch(events); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record update events"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "update events recorded",
		"count":   len(events),
		"command_id": req.CommandID,
	})
}

// ListUpdates retrieves updates with filtering using the new state table
func (h *UpdateHandler) ListUpdates(c *gin.Context) {
	filters := &models.UpdateFilters{
		Status:      c.Query("status"),
		Severity:    c.Query("severity"),
		PackageType: c.Query("package_type"),
	}

	// Parse agent_id if provided
	if agentIDStr := c.Query("agent_id"); agentIDStr != "" {
		agentID, err := uuid.Parse(agentIDStr)
		if err == nil {
			filters.AgentID = agentID
		}
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	filters.Page = page
	filters.PageSize = pageSize

	updates, total, err := h.updateQueries.ListUpdatesFromState(filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list updates"})
		return
	}

	// Get overall statistics for the summary cards
	stats, err := h.updateQueries.GetAllUpdateStats()
	if err != nil {
		// Don't fail the request if stats fail, just log and continue
		// In production, we'd use proper logging
		stats = &models.UpdateStats{}
	}

	c.JSON(http.StatusOK, gin.H{
		"updates":   updates,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
		"stats":     stats,
	})
}

// GetUpdate retrieves a single update by ID
func (h *UpdateHandler) GetUpdate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	update, err := h.updateQueries.GetUpdateByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "update not found"})
		return
	}

	c.JSON(http.StatusOK, update)
}

// ApproveUpdate marks an update as approved
func (h *UpdateHandler) ApproveUpdate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	// For now, use "admin" as approver. Will integrate with proper auth later
	if err := h.updateQueries.ApproveUpdate(id, "admin"); err != nil {
		fmt.Printf("DEBUG: ApproveUpdate failed for ID %s: %v\n", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to approve update: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "update approved"})
}

// ReportLog handles update execution logs from agents
func (h *UpdateHandler) ReportLog(c *gin.Context) {
	agentID := c.MustGet("agent_id").(uuid.UUID)

	// Update last_seen timestamp
	if err := h.agentQueries.UpdateAgentLastSeen(agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update last seen"})
		return
	}

	var req models.UpdateLogRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	logEntry := &models.UpdateLog{
		ID:              uuid.New(),
		AgentID:         agentID,
		Action:          req.Action,
		Result:          req.Result,
		Stdout:          req.Stdout,
		Stderr:          req.Stderr,
		ExitCode:        req.ExitCode,
		DurationSeconds: req.DurationSeconds,
		ExecutedAt:      time.Now(),
	}

	// Store the log entry
	if err := h.updateQueries.CreateUpdateLog(logEntry); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save log"})
		return
	}

	// NEW: Update command status if command_id is provided
	if req.CommandID != "" {
		commandID, err := uuid.Parse(req.CommandID)
		if err != nil {
			// Log warning but don't fail the request
			fmt.Printf("Warning: Invalid command ID format in log request: %s\n", req.CommandID)
		} else {
			// Prepare result data for command update
			result := models.JSONB{
				"stdout":           req.Stdout,
				"stderr":           req.Stderr,
				"exit_code":        req.ExitCode,
				"duration_seconds": req.DurationSeconds,
				"logged_at":        time.Now(),
			}

			// Update command status based on log result
			if req.Result == "success" || req.Result == "completed" {
				if err := h.commandQueries.MarkCommandCompleted(commandID, result); err != nil {
					fmt.Printf("Warning: Failed to mark command %s as completed: %v\n", commandID, err)
				}

				// NEW: If this was a successful confirm_dependencies command, mark the package as updated
				command, err := h.commandQueries.GetCommandByID(commandID)
				if err == nil && command.CommandType == models.CommandTypeConfirmDependencies {
					// Extract package info from command params
					if packageName, ok := command.Params["package_name"].(string); ok {
						if packageType, ok := command.Params["package_type"].(string); ok {
							// Extract actual completion timestamp from command result for accurate audit trail
							var completionTime *time.Time
							if loggedAtStr, ok := command.Result["logged_at"].(string); ok {
								if parsed, err := time.Parse(time.RFC3339Nano, loggedAtStr); err == nil {
									completionTime = &parsed
								}
							}

							// Update package status to 'updated' with actual completion timestamp
							if err := h.updateQueries.UpdatePackageStatus(agentID, packageType, packageName, "updated", nil, completionTime); err != nil {
								log.Printf("Warning: Failed to update package status for %s/%s: %v", packageType, packageName, err)
							} else {
								log.Printf("✅ Package %s (%s) marked as updated after successful installation", packageName, packageType)
							}
						}
					}
				}
			} else if req.Result == "failed" || req.Result == "dry_run_failed" {
				if err := h.commandQueries.MarkCommandFailed(commandID, result); err != nil {
					fmt.Printf("Warning: Failed to mark command %s as failed: %v\n", commandID, err)
				}
			} else {
				// For other results, just update the result field
				if err := h.commandQueries.UpdateCommandResult(commandID, result); err != nil {
					fmt.Printf("Warning: Failed to update command %s result: %v\n", commandID, err)
				}
			}
		}
	}

	c.JSON(http.StatusOK, gin.H{"message": "log recorded"})
}

// GetPackageHistory returns version history for a specific package
func (h *UpdateHandler) GetPackageHistory(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	packageType := c.Query("package_type")
	packageName := c.Query("package_name")

	if packageType == "" || packageName == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "package_type and package_name are required"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	history, err := h.updateQueries.GetPackageHistory(agentID, packageType, packageName, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get package history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"history": history,
		"package_type": packageType,
		"package_name": packageName,
		"count": len(history),
	})
}

// GetBatchStatus returns recent batch processing status for an agent
func (h *UpdateHandler) GetBatchStatus(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "10"))

	batches, err := h.updateQueries.GetBatchStatus(agentID, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get batch status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"batches": batches,
		"count": len(batches),
	})
}

// UpdatePackageStatus updates the status of a package (for when updates are installed)
func (h *UpdateHandler) UpdatePackageStatus(c *gin.Context) {
	agentIDStr := c.Param("agent_id")
	agentID, err := uuid.Parse(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	var req struct {
		PackageType string                 `json:"package_type" binding:"required"`
		PackageName string                 `json:"package_name" binding:"required"`
		Status      string                 `json:"status" binding:"required"`
		Metadata    map[string]interface{} `json:"metadata"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := h.updateQueries.UpdatePackageStatus(agentID, req.PackageType, req.PackageName, req.Status, req.Metadata, nil); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update package status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "package status updated"})
}

// ApproveUpdates handles bulk approval of updates
func (h *UpdateHandler) ApproveUpdates(c *gin.Context) {
	var req struct {
		UpdateIDs   []string `json:"update_ids" binding:"required"`
		ScheduledAt *string  `json:"scheduled_at"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert string IDs to UUIDs
	updateIDs := make([]uuid.UUID, 0, len(req.UpdateIDs))
	for _, idStr := range req.UpdateIDs {
		id, err := uuid.Parse(idStr)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID: " + idStr})
			return
		}
		updateIDs = append(updateIDs, id)
	}

	// For now, use "admin" as approver. Will integrate with proper auth later
	if err := h.updateQueries.BulkApproveUpdates(updateIDs, "admin"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to approve updates"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "updates approved",
		"count":   len(updateIDs),
	})
}

// RejectUpdate rejects a single update
func (h *UpdateHandler) RejectUpdate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	// For now, use "admin" as rejecter. Will integrate with proper auth later
	if err := h.updateQueries.RejectUpdate(id, "admin"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to reject update"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "update rejected"})
}

// InstallUpdate marks an update as ready for installation and creates a dry run command for the agent
func (h *UpdateHandler) InstallUpdate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	// Get the full update details to extract agent_id, package_name, and package_type
	update, err := h.updateQueries.GetUpdateByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get update details"})
		return
	}

	// Create a command for the agent to perform dry run first
	command := &models.AgentCommand{
		ID:          uuid.New(),
		AgentID:     update.AgentID,
		CommandType: models.CommandTypeDryRunUpdate,
		Params: map[string]interface{}{
			"update_id":      id.String(),
			"package_name":  update.PackageName,
			"package_type":  update.PackageType,
		},
		Status:       models.CommandStatusPending,
		CreatedAt:     time.Now(),
	}

	// Check if heartbeat should be enabled (avoid duplicates)
	if shouldEnable, err := h.shouldEnableHeartbeat(update.AgentID, 10); err == nil && shouldEnable {
		heartbeatCmd := &models.AgentCommand{
			ID:          uuid.New(),
			AgentID:     update.AgentID,
			CommandType: models.CommandTypeEnableHeartbeat,
			Params: models.JSONB{
				"duration_minutes": 10,
			},
			Status:    models.CommandStatusPending,
			CreatedAt: time.Now(),
		}

		if err := h.commandQueries.CreateCommand(heartbeatCmd); err != nil {
			log.Printf("[Heartbeat] Warning: Failed to create heartbeat command for agent %s: %v", update.AgentID, err)
		} else {
			log.Printf("[Heartbeat] Command created for agent %s before dry run", update.AgentID)
		}
	} else {
		log.Printf("[Heartbeat] Skipping heartbeat command for agent %s (already active)", update.AgentID)
	}

	// Store the dry run command in database
	if err := h.commandQueries.CreateCommand(command); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create dry run command"})
		return
	}

	// Update the package status to 'checking_dependencies' to show dry run is starting
	if err := h.updateQueries.SetCheckingDependencies(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update package status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "dry run command created for agent",
		"command_id": command.ID.String(),
	})
}

// GetUpdateLogs retrieves installation logs for a specific update
func (h *UpdateHandler) GetUpdateLogs(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	// Parse limit from query params
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	logs, err := h.updateQueries.GetUpdateLogs(id, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve update logs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":  logs,
		"count": len(logs),
	})
}

// ReportDependencies handles dependency reporting from agents after dry run
func (h *UpdateHandler) ReportDependencies(c *gin.Context) {
	agentID := c.MustGet("agent_id").(uuid.UUID)

	// Update last_seen timestamp
	if err := h.agentQueries.UpdateAgentLastSeen(agentID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update last seen"})
		return
	}

	var req models.DependencyReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// If there are NO dependencies, auto-approve and proceed directly to installation
	// This prevents updates with zero dependencies from getting stuck in "pending_dependencies"
	if len(req.Dependencies) == 0 {
		// Get the update by package to retrieve its ID
		update, err := h.updateQueries.GetUpdateByPackage(agentID, req.PackageType, req.PackageName)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get update details"})
			return
		}

		// Automatically create installation command since no dependencies need approval
		command := &models.AgentCommand{
			ID:          uuid.New(),
			AgentID:     agentID,
			CommandType: models.CommandTypeConfirmDependencies,
			Params: map[string]interface{}{
				"update_id":     update.ID.String(),
				"package_name":  req.PackageName,
				"package_type":  req.PackageType,
				"dependencies":  []string{}, // Empty dependencies array
			},
			Status:    models.CommandStatusPending,
			CreatedAt: time.Now(),
		}

		// Check if heartbeat should be enabled (avoid duplicates)
		if shouldEnable, err := h.shouldEnableHeartbeat(agentID, 10); err == nil && shouldEnable {
			heartbeatCmd := &models.AgentCommand{
				ID:          uuid.New(),
				AgentID:     agentID,
				CommandType: models.CommandTypeEnableHeartbeat,
				Params: models.JSONB{
					"duration_minutes": 10,
				},
				Status:    models.CommandStatusPending,
				CreatedAt: time.Now(),
			}

			if err := h.commandQueries.CreateCommand(heartbeatCmd); err != nil {
				log.Printf("[Heartbeat] Warning: Failed to create heartbeat command for agent %s: %v", agentID, err)
			} else {
				log.Printf("[Heartbeat] Command created for agent %s before installation", agentID)
			}
		} else {
			log.Printf("[Heartbeat] Skipping heartbeat command for agent %s (already active)", agentID)
		}

		if err := h.commandQueries.CreateCommand(command); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create installation command"})
			return
		}

		// Record that dependencies were checked (empty array) and transition directly to installing
		if err := h.updateQueries.SetInstallingWithNoDependencies(update.ID, req.Dependencies); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update package status to installing"})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"message":    "no dependencies found - installation command created automatically",
			"command_id": command.ID.String(),
		})
		return
	}

	// If dependencies EXIST, require manual approval by setting status to pending_dependencies
	if err := h.updateQueries.SetPendingDependencies(agentID, req.PackageType, req.PackageName, req.Dependencies); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update package status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "dependencies reported and status updated"})
}

// ConfirmDependencies handles user confirmation to proceed with dependency installation
func (h *UpdateHandler) ConfirmDependencies(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	// Get the update details
	update, err := h.updateQueries.GetUpdateByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "update not found"})
		return
	}

	// Create a command for the agent to install with dependencies
	command := &models.AgentCommand{
		ID:          uuid.New(),
		AgentID:     update.AgentID,
		CommandType: models.CommandTypeConfirmDependencies,
		Params: map[string]interface{}{
			"update_id":      id.String(),
			"package_name":  update.PackageName,
			"package_type":  update.PackageType,
			"dependencies":  update.Metadata["dependencies"], // Dependencies stored in metadata
		},
		Status:       models.CommandStatusPending,
		CreatedAt:     time.Now(),
	}

	// Check if heartbeat should be enabled (avoid duplicates)
	if shouldEnable, err := h.shouldEnableHeartbeat(update.AgentID, 10); err == nil && shouldEnable {
		heartbeatCmd := &models.AgentCommand{
			ID:          uuid.New(),
			AgentID:     update.AgentID,
			CommandType: models.CommandTypeEnableHeartbeat,
			Params: models.JSONB{
				"duration_minutes": 10,
			},
			Status:    models.CommandStatusPending,
			CreatedAt: time.Now(),
		}

		if err := h.commandQueries.CreateCommand(heartbeatCmd); err != nil {
			log.Printf("[Heartbeat] Warning: Failed to create heartbeat command for agent %s: %v", update.AgentID, err)
		} else {
			log.Printf("[Heartbeat] Command created for agent %s before confirm dependencies", update.AgentID)
		}
	} else {
		log.Printf("[Heartbeat] Skipping heartbeat command for agent %s (already active)", update.AgentID)
	}

	// Store the command in database
	if err := h.commandQueries.CreateCommand(command); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create confirmation command"})
		return
	}

	// Update the package status to 'installing'
	if err := h.updateQueries.InstallUpdate(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update package status"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "dependency installation confirmed and command created",
		"command_id": command.ID.String(),
	})
}

// GetAllLogs retrieves logs across all agents with filtering for universal log view
// Now returns unified history of both commands and logs
func (h *UpdateHandler) GetAllLogs(c *gin.Context) {
	filters := &models.LogFilters{
		Action:  c.Query("action"),
		Result:  c.Query("result"),
	}

	// Parse agent_id if provided
	if agentIDStr := c.Query("agent_id"); agentIDStr != "" {
		agentID, err := uuid.Parse(agentIDStr)
		if err == nil {
			filters.AgentID = agentID
		}
	}

	// Parse since timestamp if provided
	if sinceStr := c.Query("since"); sinceStr != "" {
		sinceTime, err := time.Parse(time.RFC3339, sinceStr)
		if err == nil {
			filters.Since = &sinceTime
		}
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "100"))
	filters.Page = page
	filters.PageSize = pageSize

	// Get unified history (both commands and logs)
	items, total, err := h.updateQueries.GetAllUnifiedHistory(filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve history"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":      items, // Changed from "logs" to unified items for backwards compatibility
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// GetActiveOperations retrieves currently running operations for live status view
func (h *UpdateHandler) GetActiveOperations(c *gin.Context) {
	operations, err := h.updateQueries.GetActiveOperations()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve active operations"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"operations": operations,
		"count":      len(operations),
	})
}

// RetryCommand retries a failed, timed_out, or cancelled command
func (h *UpdateHandler) RetryCommand(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command ID"})
		return
	}

	// Create a new command based on the original
	newCommand, err := h.commandQueries.RetryCommand(id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to retry command: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":    "command retry created",
		"command_id": newCommand.ID.String(),
		"new_id":     newCommand.ID.String(),
	})
}

// CancelCommand cancels a pending or sent command
func (h *UpdateHandler) CancelCommand(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid command ID"})
		return
	}

	// Cancel the command
	if err := h.commandQueries.CancelCommand(id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("failed to cancel command: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "command cancelled"})
}

// GetActiveCommands retrieves currently active commands for live operations view
func (h *UpdateHandler) GetActiveCommands(c *gin.Context) {
	commands, err := h.commandQueries.GetActiveCommands()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve active commands"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"commands": commands,
		"count":    len(commands),
	})
}

// GetRecentCommands retrieves recent commands for retry functionality
func (h *UpdateHandler) GetRecentCommands(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))

	commands, err := h.commandQueries.GetRecentCommands(limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve recent commands"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"commands": commands,
		"count":    len(commands),
		"limit":    limit,
	})
}

// ClearFailedCommands manually removes failed/timed_out commands with cheeky warning
func (h *UpdateHandler) ClearFailedCommands(c *gin.Context) {
	// Get query parameters for filtering
	olderThanDaysStr := c.Query("older_than_days")
	onlyRetriedStr := c.Query("only_retried")
	allFailedStr := c.Query("all_failed")

	var count int64
	var err error

	// Parse parameters
	olderThanDays := 7 // default
	if olderThanDaysStr != "" {
		if days, err := strconv.Atoi(olderThanDaysStr); err == nil && days > 0 {
			olderThanDays = days
		}
	}

	onlyRetried := onlyRetriedStr == "true"
	allFailed := allFailedStr == "true"

	// Build the appropriate cleanup query based on parameters
	if allFailed {
		// Clear ALL failed commands (most aggressive)
		count, err = h.commandQueries.ClearAllFailedCommands(olderThanDays)
	} else if onlyRetried {
		// Clear only failed commands that have been retried
		count, err = h.commandQueries.ClearRetriedFailedCommands(olderThanDays)
	} else {
		// Clear failed commands older than specified days (default behavior)
		count, err = h.commandQueries.ClearOldFailedCommands(olderThanDays)
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to clear failed commands",
			"details": err.Error(),
		})
		return
	}

	// Return success with cheeky message
	message := fmt.Sprintf("Archived %d failed commands", count)
	if count > 0 {
		message += ". WARNING: This shouldn't be necessary if the retry logic is working properly - you might want to check what's causing commands to fail in the first place!"
		message += " (History preserved - commands moved to archived status)"
	} else {
		message += ". No failed commands found matching your criteria. SUCCESS!"
	}

	c.JSON(http.StatusOK, gin.H{
		"message": message,
		"count":   count,
		"cheeky_warning": "Consider this a developer experience enhancement - the system should clean up after itself automatically!",
	})
}
