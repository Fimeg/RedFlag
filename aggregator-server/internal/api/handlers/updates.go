package handlers

import (
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/aggregator-project/aggregator-server/internal/database/queries"
	"github.com/aggregator-project/aggregator-server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type UpdateHandler struct {
	updateQueries  *queries.UpdateQueries
	agentQueries   *queries.AgentQueries
	commandQueries *queries.CommandQueries
}

func NewUpdateHandler(uq *queries.UpdateQueries, aq *queries.AgentQueries, cq *queries.CommandQueries) *UpdateHandler {
	return &UpdateHandler{
		updateQueries:  uq,
		agentQueries:   aq,
		commandQueries: cq,
	}
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

	log := &models.UpdateLog{
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
	if err := h.updateQueries.CreateUpdateLog(log); err != nil {
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
			if req.Result == "success" {
				if err := h.commandQueries.MarkCommandCompleted(commandID, result); err != nil {
					fmt.Printf("Warning: Failed to mark command %s as completed: %v\n", commandID, err)
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

	if err := h.updateQueries.UpdatePackageStatus(agentID, req.PackageType, req.PackageName, req.Status, req.Metadata); err != nil {
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

	// Store the command in database
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

	// Update the package status to pending_dependencies
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

	logs, total, err := h.updateQueries.GetAllLogs(filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to retrieve logs"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"logs":      logs,
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
