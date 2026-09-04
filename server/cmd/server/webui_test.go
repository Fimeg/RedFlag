package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Fimeg/RedFlag/server/internal/webui"
	"github.com/gin-gonic/gin"
)

// These tests only run when a UI build is embedded (dist populated before
// compile). A clean checkout builds API-only and skips.
func newWebUIRouter(t *testing.T) *gin.Engine {
	t.Helper()
	if !webui.Present() {
		t.Skip("no embedded UI build in this binary")
	}
	gin.SetMode(gin.TestMode)
	router := gin.New()
	registerWebUI(router)
	return router
}

func TestWebUIServesIndexAtRoot(t *testing.T) {
	router := newWebUIRouter(t)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET / = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("GET / content-type = %q, want html", ct)
	}
}

func TestWebUISPAFallbackForClientRoutes(t *testing.T) {
	router := newWebUIRouter(t)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/agents/123", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET /agents/123 = %d, want 200 (SPA fallback)", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "text/html") {
		t.Fatalf("SPA fallback content-type = %q, want html", ct)
	}
	if cc := w.Header().Get("Cache-Control"); cc != "no-cache" {
		t.Fatalf("SPA fallback cache-control = %q, want no-cache", cc)
	}
}

func TestWebUIUnmatchedAPIPathStaysJSON404(t *testing.T) {
	router := newWebUIRouter(t)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/v1/does-not-exist", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("GET /api/v1/does-not-exist = %d, want 404", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Fatalf("API 404 content-type = %q, want json (must not leak SPA html)", ct)
	}
}

func TestWebUINonGetIs404(t *testing.T) {
	router := newWebUIRouter(t)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/agents/123", nil))
	if w.Code != http.StatusNotFound {
		t.Fatalf("POST /agents/123 = %d, want 404", w.Code)
	}
}

func TestWebUIHashedAssetsCacheHard(t *testing.T) {
	router := newWebUIRouter(t)
	uiFS, err := webui.FS()
	if err != nil {
		t.Fatalf("webui.FS: %v", err)
	}
	entries, err := fs.ReadDir(uiFS, "assets")
	if err != nil || len(entries) == 0 {
		t.Skip("no assets directory in embedded build")
	}
	asset := "/assets/" + entries[0].Name()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, asset, nil))
	if w.Code != http.StatusOK {
		t.Fatalf("GET %s = %d, want 200", asset, w.Code)
	}
	if cc := w.Header().Get("Cache-Control"); !strings.Contains(cc, "immutable") {
		t.Fatalf("asset cache-control = %q, want immutable", cc)
	}
}
