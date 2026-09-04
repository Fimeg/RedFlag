package handlers

import (
	"context"
	"database/sql"
	"errors"
	"net/http"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/Fimeg/RedFlag/server/internal/services/upstream"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// UpstreamHandler exposes the tracked_software CRUD + sync-now action.
// The dashboard's Stack Drift widget reads from ListDrifted.
type UpstreamHandler struct {
	queries  *queries.UpstreamQueries
	syncer   *upstream.Syncer
	registry *upstream.Registry
	runner   BackgroundRunner // optional; bounded fire-and-forget pool (SCALE-001 S2)
}

func NewUpstreamHandler(q *queries.UpstreamQueries, s *upstream.Syncer, r *upstream.Registry) *UpstreamHandler {
	return &UpstreamHandler{queries: q, syncer: s, registry: r}
}

// SetTaskRunner wires the bounded background pool. Nil falls back to a plain
// goroutine (legacy unbounded behavior).
func (h *UpstreamHandler) SetTaskRunner(r BackgroundRunner) {
	h.runner = r
}

// bg runs fn on the bounded pool when wired, else as a plain goroutine.
func (h *UpstreamHandler) bg(name string, fn func()) {
	if h.runner != nil {
		h.runner.Go(name, fn)
		return
	}
	go fn()
}

// List returns every tracked_software row. The dashboard panel filters
// client-side; admin pages use the full list.
func (h *UpstreamHandler) List(c *gin.Context) {
	rows, err := h.queries.List()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list tracked software"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"software": rows,
		"sources":  h.registry.Names(),
	})
}

// ListDrifted is the dashboard feed — only rows where current != latest.
func (h *UpstreamHandler) ListDrifted(c *gin.Context) {
	rows, err := h.queries.ListDrifted()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list drifted software"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"software": rows})
}

func (h *UpstreamHandler) Create(c *gin.Context) {
	var in models.TrackedSoftwareInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	if _, ok := h.registry.Get(in.Source); !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "unknown source",
			"available_sources": h.registry.Names(),
		})
		return
	}
	row, err := h.queries.Create(in)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Kick off an immediate sync so the new row gets populated without waiting a
	// full tick. Fire-and-forget; the result writes to DB. Uses a background
	// context, not the request's — the handler returns immediately, and the
	// request context would be cancelled out from under the sync (the bug that
	// silently killed the immediate sync's HTTP fetch).
	id := row.ID
	h.bg("upstream_sync_one", func() {
		_ = h.syncer.SyncOne(context.Background(), id)
	})
	c.JSON(http.StatusCreated, row)
}

func (h *UpstreamHandler) UpdateSettings(c *gin.Context) {
	id, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	var in models.TrackedSoftwareSettingsInput
	if err := c.ShouldBindJSON(&in); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	row, err := h.queries.UpdateSettings(id, in)
	if err != nil {
		// sql.ErrNoRows (wrapped) means the UPDATE matched no row — a real
		// not-found. Anything else is a connection/constraint failure and must
		// not be misreported as 404; surface it as a server error so the cause
		// isn't hidden behind a misleading "not found."
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "tracked software not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Kick an immediate sync when the operator changed something that affects
	// latest_version: re-enabling a row, or flipping track_prereleases on an
	// enabled one. Matches Create's prompt-sync behavior; fire-and-forget and
	// idempotent, so a redundant kick on a no-op enable is harmless.
	if row.Enabled && (in.Enabled != nil || in.TrackPrereleases != nil) {
		h.bg("upstream_sync_one", func() {
			_ = h.syncer.SyncOne(context.Background(), id)
		})
	}
	c.JSON(http.StatusOK, row)
}

func (h *UpstreamHandler) Delete(c *gin.Context) {
	id, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.queries.Delete(id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Status(http.StatusNoContent)
}

// SyncNow forces a sync for a single tracked_software row. Blocks on the
// fetch so the operator sees the result inline.
func (h *UpstreamHandler) SyncNow(c *gin.Context) {
	id, err := uuid.FromString(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	if err := h.syncer.SyncOne(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	row, err := h.queries.GetByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, row)
}

// RecentDrift returns the most recent drift events for the dashboard.
func (h *UpstreamHandler) RecentDrift(c *gin.Context) {
	events, err := h.queries.RecentDriftEvents(25)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"events": events})
}
