package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
	"github.com/doug-martin/goqu/v9"
)

// ClientErrorHandler handles frontend error logging per ETHOS #1
type ClientErrorHandler struct {
	db      *sqlx.DB
	eventDB *sqlx.DB // for system_events writes
}

// NewClientErrorHandler creates a new error handler
func NewClientErrorHandler(db *sqlx.DB) *ClientErrorHandler {
	return &ClientErrorHandler{db: db, eventDB: db}
}

// GetErrorsResponse represents paginated error list
type GetErrorsResponse struct {
	Errors     []ClientErrorResponse `json:"errors"`
	Total      int64                 `json:"total"`
	Page       int                   `json:"page"`
	PageSize   int                   `json:"page_size"`
	TotalPages int                   `json:"total_pages"`
}

// ClientErrorResponse represents a single error in response
type ClientErrorResponse struct {
	ID         string                 `json:"id"`
	AgentID    string                 `json:"agent_id,omitempty"`
	Subsystem  string                 `json:"subsystem"`
	ErrorType  string                 `json:"error_type"`
	Message    string                 `json:"message"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
	URL        string                 `json:"url"`
	CreatedAt  time.Time              `json:"created_at"`
}

// GetErrors returns paginated error logs (admin only)
// GetErrors returns paginated error logs (admin only)
func (h *ClientErrorHandler) GetErrors(c *gin.Context) {
	// Parse pagination params
	page := 1
	pageSize := 50
	if p, ok := c.GetQuery("page"); ok {
		fmt.Sscanf(p, "%d", &page)
	}
	if ps, ok := c.GetQuery("page_size"); ok {
		fmt.Sscanf(ps, "%d", &pageSize)
	}
	if pageSize > 100 {
		pageSize = 100 // Max page size
	}

	// Parse filters
	subsystem := c.Query("subsystem")
	errorType := c.Query("error_type")
	agentIDStr := c.Query("agent_id")

	// Build filter with goqu
	sd := goqu.Dialect("postgres").From("client_errors")

	if subsystem != "" {
		sd = sd.Where(goqu.Ex{"subsystem": subsystem})
	}
	if errorType != "" {
		sd = sd.Where(goqu.Ex{"error_type": errorType})
	}
	if agentIDStr != "" {
		if id, err := uuid.FromString(agentIDStr); err == nil {
			sd = sd.Where(goqu.Ex{"agent_id": id})
		}
	}

	// Count
	countSQL, countArgs, err := sd.Select(goqu.COUNT("*")).ToSQL()
	if err != nil {
		log.Printf(`[ERROR] [server] [client_error] count_build_failed error="%v"`, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}
	var total int64
	if err := h.db.Get(&total, countSQL, countArgs...); err != nil {
		log.Printf(`[ERROR] [server] [client_error] count_failed error="%v"`, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "count failed"})
		return
	}

	// Data with pagination
	sql, args, err := sd.Select(
		"id", "agent_id", "subsystem", "error_type", "message", "metadata", "url", "created_at",
	).Order(goqu.C("created_at").Desc()).Limit(uint(pageSize)).Offset(uint((page - 1) * pageSize)).ToSQL()
	if err != nil {
		log.Printf(`[ERROR] [server] [client_error] query_build_failed error="%v"`, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}

	var errors []ClientErrorResponse
	if err := h.db.Select(&errors, sql, args...); err != nil {
		log.Printf(`[ERROR] [server] [client_error] query_failed error="%v"`, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query failed"})
		return
	}

	totalPages := int((total + int64(pageSize) - 1) / int64(pageSize))

	c.JSON(http.StatusOK, GetErrorsResponse{
		Errors:     errors,
		Total:      total,
		Page:       page,
		PageSize:   pageSize,
		TotalPages: totalPages,
	})
}

// LogErrorRequest represents a client error log entry
type LogErrorRequest struct {
	Subsystem   string                 `json:"subsystem" binding:"required"`
	ErrorType   string                 `json:"error_type" binding:"required,oneof=javascript_error api_error ui_error validation_error client_debug client_trace"`
	Message     string                 `json:"message" binding:"required,max=10000"`
	StackTrace  string                 `json:"stack_trace,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	URL         string                 `json:"url" binding:"required"`
}

// LogError processes and stores frontend errors
func (h *ClientErrorHandler) LogError(c *gin.Context) {
	var req LogErrorRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[ERROR] [server] [client_error] validation_failed error=\"%v\"", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request data"})
		return
	}

	// Extract agent ID from auth middleware if available
	var agentID interface{}
	if agentIDValue, exists := c.Get("agentID"); exists {
		if id, ok := agentIDValue.(uuid.UUID); ok {
			agentID = id
		}
	}

	// Log to console with HISTORY prefix
	log.Printf("[ERROR] [server] [client] [%s] agent_id=%v subsystem=%s message=\"%s\"",
		req.ErrorType, agentID, req.Subsystem, truncate(req.Message, 200))
	log.Printf("[HISTORY] [server] [client_error] agent_id=%v subsystem=%s type=%s url=\"%s\" message=\"%s\" timestamp=%s",
		agentID, req.Subsystem, req.ErrorType, req.URL, req.Message, time.Now().UTC().Format(time.RFC3339))

	// Store in database with retry logic
	if err := h.storeError(agentID, req, c); err != nil {
		log.Printf("[ERROR] [server] [client_error] store_failed error=\"%v\"", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store error"})
		return
	}

	// Bridge to system_events so client errors appear in the unified timeline.
	go h.bridgeToSystemEvents(agentID, req)

	c.JSON(http.StatusOK, gin.H{"logged": true})
}

// storeError persists error to database with retry
func (h *ClientErrorHandler) storeError(agentID interface{}, req LogErrorRequest, c *gin.Context) error {
	const maxRetries = 3
	var lastErr error

	for attempt := 1; attempt <= maxRetries; attempt++ {
		query := `INSERT INTO client_errors (agent_id, subsystem, error_type, message, stack_trace, metadata, url, user_agent)
		          VALUES (:agent_id, :subsystem, :error_type, :message, :stack_trace, :metadata, :url, :user_agent)`

		// Convert metadata map to JSON for PostgreSQL JSONB column
		var metadataJSON json.RawMessage
		if req.Metadata != nil && len(req.Metadata) > 0 {
			jsonBytes, err := json.Marshal(req.Metadata)
			if err != nil {
				log.Printf("[ERROR] [server] [client_error] metadata_marshal_failed error=\"%v\"", err)
				metadataJSON = nil
			} else {
				metadataJSON = json.RawMessage(jsonBytes)
			}
		}

		_, err := h.db.NamedExec(query, map[string]interface{}{
			"agent_id":    agentID,
			"subsystem":   req.Subsystem,
			"error_type":  req.ErrorType,
			"message":     req.Message,
			"stack_trace": req.StackTrace,
			"metadata":    metadataJSON,
			"url":         req.URL,
			"user_agent":  c.GetHeader("User-Agent"),
		})

		if err == nil {
			return nil
		}

		lastErr = err
		if attempt < maxRetries {
			time.Sleep(time.Duration(attempt) * time.Second)
			continue
		}
	}

	return fmt.Errorf("failed after %d attempts: %w", maxRetries, lastErr)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// bridgeToSystemEvents writes a system_events row so client errors appear in
// the unified timeline alongside agent/server events.
func (h *ClientErrorHandler) bridgeToSystemEvents(agentID interface{}, req LogErrorRequest) {
	id, err := uuid.NewV4()
	if err != nil {
		log.Printf("[ERROR] [server] [client_error] bridge_uuid_failed error=%v", err)
		return
	}

	severity := "info"
	switch req.ErrorType {
	case "javascript_error", "api_error":
		severity = "error"
	case "ui_error", "validation_error":
		severity = "warning"
	case "client_debug", "client_trace":
		severity = "info"
	}

	var agentPtr *uuid.UUID
	if id, ok := agentID.(uuid.UUID); ok {
		agentPtr = &id
	}

	meta := map[string]interface{}{
		"subsystem":  req.Subsystem,
		"error_type": req.ErrorType,
		"url":        req.URL,
	}
	if req.StackTrace != "" {
		meta["stack_trace"] = truncate(req.StackTrace, 2000)
	}

	query := `
		INSERT INTO system_events (id, agent_id, event_type, event_subtype, severity, component, message, metadata, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW())
	`
	_, err = h.eventDB.Exec(query, id, agentPtr, "client_error", req.ErrorType, severity, "client",
		truncate(req.Message, 500), models.JSONB(meta))
	if err != nil {
		log.Printf("[ERROR] [server] [client_error] bridge_failed error=%v", err)
	}
}
