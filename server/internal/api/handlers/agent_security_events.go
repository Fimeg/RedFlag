package handlers

import (
	"fmt"
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// AgentSecurityEventsHandler handles security event reporting from agents.
type AgentSecurityEventsHandler struct {
	agentQueries *queries.AgentQueries
}

// NewAgentSecurityEventsHandler creates a new agent security events handler.
func NewAgentSecurityEventsHandler(aq *queries.AgentQueries) *AgentSecurityEventsHandler {
	return &AgentSecurityEventsHandler{agentQueries: aq}
}

// ReportSecurityEventsRequest is the JSON body for agent security event reports.
type ReportSecurityEventsRequest struct {
	Events []AgentSecurityEventPayload `json:"events" binding:"required,dive"`
}

// AgentSecurityEventPayload is the wire format sent by the agent.
type AgentSecurityEventPayload struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	EventType string                 `json:"event_type"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// ReportSecurityEventsResponse mirrors the ReportEventsResponse pattern.
type ReportSecurityEventsResponse struct {
	Accepted int      `json:"accepted"`
	Rejected int      `json:"rejected"`
	Errors   []string `json:"errors,omitempty"`
}

// ReportSecurityEvents handles POST /api/v1/agents/:id/security-events
// Accepts up to 100 security events per request. AgentID is taken from the
// URL parameter, matching the pattern used by ReportEvents.
func (h *AgentSecurityEventsHandler) ReportSecurityEvents(c *gin.Context) {
	agentIDStr := c.Param("id")
	agentID, err := uuid.FromString(agentIDStr)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid agent ID"})
		return
	}

	var req ReportSecurityEventsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	const maxEventsPerRequest = 100
	if len(req.Events) > maxEventsPerRequest {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":  "too many events",
			"max":    maxEventsPerRequest,
			"actual": len(req.Events),
		})
		return
	}

	response := ReportSecurityEventsResponse{}

	for i, payload := range req.Events {
		event := &models.SecurityEvent{
			Timestamp: payload.Timestamp,
			Level:     payload.Level,
			EventType: payload.EventType,
			AgentID:   agentID,
			Message:   payload.Message,
			Details:   payload.Details,
			Metadata:  map[string]interface{}{"source": "agent"},
		}

		if err := h.agentQueries.CreateSecurityEvent(event); err != nil {
			response.Rejected++
			response.Errors = append(response.Errors, fmt.Sprintf("event %d: %s", i, err.Error()))
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
