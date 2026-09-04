package handlers

import (
	"net/http"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// ReportEventsRequest represents a batch of events from an agent
type ReportEventsRequest struct {
	Events []models.SystemEvent `json:"events" binding:"required,dive"`
}

// ReportEventsResponse represents the response to a batch event report
type ReportEventsResponse struct {
	Accepted int      `json:"accepted"`
	Rejected int      `json:"rejected"`
	Errors   []string `json:"errors,omitempty"`
}

// EventsHandler handles agent event reporting
type EventsHandler struct {
	agentQueries *queries.AgentQueries
}

// NewEventsHandler creates a new events handler
func NewEventsHandler(aq *queries.AgentQueries) *EventsHandler {
	return &EventsHandler{agentQueries: aq}
}

// ReportEvents handles batch event reporting from agents
// POST /api/v1/agents/:id/events
// Accepts up to 100 events per request (TD-003)
func (h *EventsHandler) ReportEvents(c *gin.Context) {
	agentIDStr := c.Param("id")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	var req ReportEventsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Enforce 100 event limit per request (TD-003)
	const maxEventsPerRequest = 100
	if len(req.Events) > maxEventsPerRequest {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "too many events",
			"max":    maxEventsPerRequest,
			"actual": len(req.Events),
		})
		return
	}

	response := ReportEventsResponse{
		Accepted: 0,
		Rejected: 0,
	}

	// Process each event
	for i, event := range req.Events {
		// Ensure event has ID
		if event.ID == uuid.Nil {
			event.ID = uuid.Must(uuid.NewV4())
		}

		// Set agent ID from URL parameter
		event.AgentID = &agentID

		// Store event
		if err := h.agentQueries.CreateSystemEvent(&event); err != nil {
			response.Rejected++
			response.Errors = append(response.Errors, "event "+string(rune('0'+i))+": "+err.Error())
		} else {
			response.Accepted++
		}
	}

	status := http.StatusOK
	if response.Rejected > 0 && response.Accepted == 0 {
		status = http.StatusInternalServerError
	} else if response.Rejected > 0 {
		status = http.StatusPartialContent // 206
	}

	c.JSON(status, response)
}
