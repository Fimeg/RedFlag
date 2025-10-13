package handlers

import (
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
}

func NewUpdateHandler(uq *queries.UpdateQueries) *UpdateHandler {
	return &UpdateHandler{updateQueries: uq}
}

// ReportUpdates handles update reports from agents
func (h *UpdateHandler) ReportUpdates(c *gin.Context) {
	agentID := c.MustGet("agent_id").(uuid.UUID)

	var req models.UpdateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Process each update
	for _, item := range req.Updates {
		update := &models.UpdatePackage{
			ID:                 uuid.New(),
			AgentID:            agentID,
			PackageType:        item.PackageType,
			PackageName:        item.PackageName,
			PackageDescription: item.PackageDescription,
			CurrentVersion:     item.CurrentVersion,
			AvailableVersion:   item.AvailableVersion,
			Severity:           item.Severity,
			CVEList:            models.StringArray(item.CVEList),
			KBID:               item.KBID,
			RepositorySource:   item.RepositorySource,
			SizeBytes:          item.SizeBytes,
			Status:             "pending",
			Metadata:           item.Metadata,
		}

		if err := h.updateQueries.UpsertUpdate(update); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save update"})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "updates recorded",
		"count":   len(req.Updates),
	})
}

// ListUpdates retrieves updates with filtering
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
			filters.AgentID = &agentID
		}
	}

	// Parse pagination
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	filters.Page = page
	filters.PageSize = pageSize

	updates, total, err := h.updateQueries.ListUpdates(filters)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list updates"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"updates":   updates,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
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
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to approve update"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "update approved"})
}

// ReportLog handles update execution logs from agents
func (h *UpdateHandler) ReportLog(c *gin.Context) {
	agentID := c.MustGet("agent_id").(uuid.UUID)

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
