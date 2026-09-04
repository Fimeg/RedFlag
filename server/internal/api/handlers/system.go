package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/Fimeg/RedFlag/server/internal/version"
	"github.com/gin-gonic/gin"
)

// SystemHandler handles system-level operations
type SystemHandler struct {
	signingService    *services.SigningService
	signingKeyQueries *queries.SigningKeyQueries
}

// NewSystemHandler creates a new system handler
func NewSystemHandler(ss *services.SigningService, skq *queries.SigningKeyQueries) *SystemHandler {
	return &SystemHandler{
		signingService:    ss,
		signingKeyQueries: skq,
	}
}

// GetPublicKey returns the server's Ed25519 public key for signature verification.
// This allows agents to fetch the public key at runtime instead of embedding it at build time.
func (h *SystemHandler) GetPublicKey(c *gin.Context) {
	if h.signingService == nil || !h.signingService.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "signing service not configured",
			"hint":  "Set REDFLAG_SIGNING_PRIVATE_KEY environment variable",
		})
		return
	}

	pubKeyHex := h.signingService.GetPublicKey()
	fingerprint := h.signingService.GetPublicKeyFingerprint()
	keyID := h.signingService.GetCurrentKeyID()

	// Try to get version from DB; fall back to 1 if unavailable
	version := 1
	if h.signingKeyQueries != nil {
		ctx := context.Background()
		if primaryKey, err := h.signingKeyQueries.GetPrimarySigningKey(ctx); err == nil {
			version = primaryKey.Version
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"public_key":  pubKeyHex,
		"fingerprint": fingerprint,
		"algorithm":   "ed25519",
		"key_size":    32,
		"key_id":      keyID,
		"version":     version,
	})
}

// GetActivePublicKeys returns all currently active public keys for key-rotation-aware agents.
// This is a rate-limited public endpoint — no authentication required.
func (h *SystemHandler) GetActivePublicKeys(c *gin.Context) {
	if h.signingService == nil || !h.signingService.IsEnabled() {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "signing service not configured",
		})
		return
	}

	ctx := c.Request.Context()
	activeKeys, err := h.signingService.GetAllActivePublicKeys(ctx)

	// Build response — always return at least the current key
	type keyEntry struct {
		KeyID     string `json:"key_id"`
		PublicKey string `json:"public_key"`
		IsPrimary bool   `json:"is_primary"`
		Version   int    `json:"version"`
		Algorithm string `json:"algorithm"`
	}

	if err != nil || len(activeKeys) == 0 {
		// Fall back to single-entry response with current key
		c.JSON(http.StatusOK, []keyEntry{
			{
				KeyID:     h.signingService.GetCurrentKeyID(),
				PublicKey: h.signingService.GetPublicKeyHex(),
				IsPrimary: true,
				Version:   1,
				Algorithm: "ed25519",
			},
		})
		return
	}

	entries := make([]keyEntry, 0, len(activeKeys))
	for _, k := range activeKeys {
		entries = append(entries, keyEntry{
			KeyID:     k.KeyID,
			PublicKey: k.PublicKey,
			IsPrimary: k.IsPrimary,
			Version:   k.Version,
			Algorithm: k.Algorithm,
		})
	}

	c.JSON(http.StatusOK, entries)
}

// ListSigningKeys returns all signing keys (active and deprecated) for the admin UI.
// Admin-authenticated route. Used by the dashboard's Signing Keys page to render the
// key roster and surface which key is primary, which are accepted, and which are retired.
func (h *SystemHandler) ListSigningKeys(c *gin.Context) {
	if h.signingKeyQueries == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "signing key registry not configured"})
		return
	}

	type keyRow struct {
		KeyID        string  `json:"key_id"`
		PublicKey    string  `json:"public_key"`
		Algorithm    string  `json:"algorithm"`
		IsActive     bool    `json:"is_active"`
		IsPrimary    bool    `json:"is_primary"`
		Version      int     `json:"version"`
		CreatedAt    string  `json:"created_at"`
		DeprecatedAt *string `json:"deprecated_at,omitempty"`
	}

	ctx := c.Request.Context()
	keys, err := h.signingKeyQueries.GetAllSigningKeys(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list signing keys"})
		return
	}

	rows := make([]keyRow, 0, len(keys))
	for _, k := range keys {
		row := keyRow{
			KeyID:     k.KeyID,
			PublicKey: k.PublicKey,
			Algorithm: k.Algorithm,
			IsActive:  k.IsActive,
			IsPrimary: k.IsPrimary,
			Version:   k.Version,
			CreatedAt: k.CreatedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		}
		if k.DeprecatedAt != nil {
			s := k.DeprecatedAt.UTC().Format("2006-01-02T15:04:05Z07:00")
			row.DeprecatedAt = &s
		}
		rows = append(rows, row)
	}

	c.JSON(http.StatusOK, gin.H{"keys": rows, "count": len(rows)})
}

// DeprecateSigningKey marks a non-primary signing key as deprecated (inactive).
// Refuses to deprecate the current primary — the queries layer enforces this and
// returns a clear error which we surface to the operator.
func (h *SystemHandler) DeprecateSigningKey(c *gin.Context) {
	if h.signingKeyQueries == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "signing key registry not configured"})
		return
	}

	keyID := c.Param("key_id")
	if keyID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "key_id is required"})
		return
	}

	if err := h.signingKeyQueries.DeprecateKey(c.Request.Context(), keyID); err != nil {
		msg := err.Error()
		switch {
		case strings.Contains(msg, "cannot deprecate primary"):
			c.JSON(http.StatusConflict, gin.H{"error": msg})
		case strings.Contains(msg, "not found"):
			c.JSON(http.StatusNotFound, gin.H{"error": msg})
		default:
			c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "signing key deprecated", "key_id": keyID})
}

// GetSystemInfo returns general system information
func (h *SystemHandler) GetSystemInfo(c *gin.Context) {
	versions := version.GetCurrentVersions()
	c.JSON(http.StatusOK, gin.H{
		"version":              versions.AgentVersion,
		"latest_agent_version": versions.AgentVersion,
		"min_agent_version":    versions.MinAgentVersion,
		"name":                 "RedFlag Aggregator",
		"description":          "Self-hosted update management platform",
		"features": []string{
			"agent_management",
			"update_tracking",
			"command_execution",
			"ed25519_signing",
			"key_rotation",
		},
	})
}
