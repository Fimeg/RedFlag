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
	updateQueries *queries.UpdateQueries
	agentQueries  *queries.AgentQueries
}

func NewUpdateHandler(uq *queries.UpdateQueries, aq *queries.AgentQueries) *UpdateHandler {
	return &UpdateHandler{
		updateQueries: uq,
		agentQueries:  aq,
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

	if err := h.updateQueries.CreateUpdateLog(log); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save log"})
		return
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

// InstallUpdate marks an update as ready for installation
func (h *UpdateHandler) InstallUpdate(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.Parse(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid update ID"})
		return
	}

	if err := h.updateQueries.InstallUpdate(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to start update installation"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "update installation started"})
}
