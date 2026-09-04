package middleware

import (
	"bytes"
	"io"
	"log"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
	"github.com/jmoiron/sqlx"
)

// AuditMiddleware logs admin actions to system_events for the unified timeline.
// Wraps admin routes so every mutation is journaled with the acting user, method,
// path, and request body (truncated).
type AuditMiddleware struct {
	db *sqlx.DB
}

// NewAuditMiddleware creates an AuditMiddleware.
func NewAuditMiddleware(db *sqlx.DB) *AuditMiddleware {
	return &AuditMiddleware{db: db}
}

// Audit returns a gin.HandlerFunc that logs admin actions to system_events.
func (a *AuditMiddleware) Audit() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Only audit mutations (POST, PUT, PATCH, DELETE).
		method := c.Request.Method
		if method == "GET" || method == "OPTIONS" || method == "HEAD" {
			c.Next()
			return
		}

		// Capture request body (truncated).
		var bodySnippet string
		if c.Request.Body != nil {
			bodyBytes, err := io.ReadAll(c.Request.Body)
			if err == nil {
				c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
				bodySnippet = truncateBody(bodyBytes, 500)
			}
		}

		// Extract admin identity from context (set by WebAuthMiddleware).
		adminID, _ := c.Get("userID")
		adminUser, _ := c.Get("username")

		start := time.Now()
		c.Next()
		duration := time.Since(start)

		// Skip logging for health checks and read-only admin endpoints.
		path := c.Request.URL.Path
		if path == "/api/v1/admin/health" {
			return
		}

		status := c.Writer.Status()
		severity := "info"
		if status >= 400 {
			severity = "warning"
		}
		if status >= 500 {
			severity = "error"
		}

		// Build metadata.
		meta := map[string]interface{}{
			"method":   method,
			"path":     path,
			"status":   status,
			"duration": duration.Milliseconds(),
		}
		if bodySnippet != "" {
			meta["request_body"] = bodySnippet
		}
		if adminUser != nil {
			meta["admin_user"] = adminUser
		}

		// Write to system_events (best-effort, async).
		go a.logEvent(adminID, method, path, severity, meta)
	}
}

func (a *AuditMiddleware) logEvent(adminID interface{}, method, path, severity string, meta map[string]interface{}) {
	id, err := uuid.NewV4()
	if err != nil {
		log.Printf("[ERROR] [server] [audit] uuid_failed error=%v", err)
		return
	}

	// Determine event type from method.
	eventType := "admin_action"
	switch method {
	case "POST":
		eventType = "admin_create"
	case "PUT", "PATCH":
		eventType = "admin_update"
	case "DELETE":
		eventType = "admin_delete"
	}

	message := method + " " + path

	query := `
		INSERT INTO system_events (id, agent_id, event_type, event_subtype, severity, component, message, metadata, created_at)
		VALUES ($1, NULL, $2, $3, $4, 'admin', $5, $6, NOW())
	`
	_, err = a.db.Exec(query, id, eventType, "audit", severity, message, models.JSONB(meta))
	if err != nil {
		log.Printf("[ERROR] [server] [audit] insert_failed path=%s error=%v", path, err)
	}
}

func truncateBody(body []byte, maxLen int) string {
	s := string(body)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
