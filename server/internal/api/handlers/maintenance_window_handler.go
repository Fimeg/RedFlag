package handlers

import (
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

type MaintenanceWindowHandler struct {
	windowQueries *queries.MaintenanceWindowQueries
}

func NewMaintenanceWindowHandler(windowQueries *queries.MaintenanceWindowQueries) *MaintenanceWindowHandler {
	return &MaintenanceWindowHandler{windowQueries: windowQueries}
}

// ListWindows returns all maintenance windows.
func (h *MaintenanceWindowHandler) ListWindows(c *gin.Context) {
	windows, err := h.windowQueries.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list maintenance windows"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"windows": windows})
}

// GetWindow returns a single maintenance window by ID.
func (h *MaintenanceWindowHandler) GetWindow(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid window ID"})
		return
	}

	window, err := h.windowQueries.GetByID(id)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "maintenance window not found"})
		return
	}
	c.JSON(http.StatusOK, window)
}

// CreateWindow creates a new maintenance window.
func (h *MaintenanceWindowHandler) CreateWindow(c *gin.Context) {
	var input models.MaintenanceWindowInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	if input.DayOfWeek < 0 || input.DayOfWeek > 6 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "day_of_week must be between 0 (Sunday) and 6 (Saturday)"})
		return
	}
	if input.StartTime == "" || input.EndTime == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start_time and end_time are required (HH:MM)"})
		return
	}

	window, err := h.windowQueries.Create(input)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create maintenance window: " + err.Error()})
		return
	}
	c.JSON(http.StatusCreated, window)
}

// UpdateWindow updates an existing maintenance window.
func (h *MaintenanceWindowHandler) UpdateWindow(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid window ID"})
		return
	}

	var input models.MaintenanceWindowInput
	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	if err := h.windowQueries.Update(id, input); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to update maintenance window: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "maintenance window updated"})
}

// DeleteWindow removes a maintenance window.
func (h *MaintenanceWindowHandler) DeleteWindow(c *gin.Context) {
	idStr := c.Param("id")
	id, err := uuid.FromString(idStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid window ID"})
		return
	}

	if err := h.windowQueries.Delete(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete maintenance window: " + err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "maintenance window deleted"})
}

// CheckWindow returns whether the current server time falls inside any window.
func (h *MaintenanceWindowHandler) CheckWindow(c *gin.Context) {
	inside, err := h.windowQueries.IsWithinMaintenanceWindow(time.Now().UTC())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to check maintenance windows"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"inside_window": inside})
}
