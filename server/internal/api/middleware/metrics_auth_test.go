package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Fimeg/RedFlag/server/internal/api/middleware"
	"github.com/gin-gonic/gin"
)

func TestMetricsBearerAuthRequiresDedicatedToken(t *testing.T) {
	gin.SetMode(gin.TestMode)

	token := "metrics-token-for-test"
	router := gin.New()
	router.Use(middleware.MetricsBearerAuth(func() middleware.MetricsAuthConfig {
		return middleware.MetricsAuthConfig{
			Enabled:        true,
			TokenSHA256Hex: middleware.HashMetricsToken(token),
		}
	}))
	router.GET("/metrics", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	missing := httptest.NewRecorder()
	router.ServeHTTP(missing, httptest.NewRequest("GET", "/metrics", nil))
	if missing.Code != http.StatusUnauthorized {
		t.Fatalf("missing token status=%d, want %d", missing.Code, http.StatusUnauthorized)
	}

	wrong := httptest.NewRecorder()
	wrongReq := httptest.NewRequest("GET", "/metrics", nil)
	wrongReq.Header.Set("Authorization", "Bearer wrong")
	router.ServeHTTP(wrong, wrongReq)
	if wrong.Code != http.StatusUnauthorized {
		t.Fatalf("wrong token status=%d, want %d", wrong.Code, http.StatusUnauthorized)
	}

	ok := httptest.NewRecorder()
	okReq := httptest.NewRequest("GET", "/metrics", nil)
	okReq.Header.Set("Authorization", "Bearer "+token)
	router.ServeHTTP(ok, okReq)
	if ok.Code != http.StatusOK {
		t.Fatalf("valid token status=%d, want %d", ok.Code, http.StatusOK)
	}
}

func TestMetricsBearerAuthDisabled(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(middleware.MetricsBearerAuth(func() middleware.MetricsAuthConfig {
		return middleware.MetricsAuthConfig{Enabled: false}
	}))
	router.GET("/metrics", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer anything")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled status=%d, want %d", rec.Code, http.StatusNotFound)
	}
}

func TestMetricsBearerAuthRequiresConfiguredHash(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	router.Use(middleware.MetricsBearerAuth(func() middleware.MetricsAuthConfig {
		return middleware.MetricsAuthConfig{Enabled: true}
	}))
	router.GET("/metrics", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/metrics", nil)
	req.Header.Set("Authorization", "Bearer anything")
	router.ServeHTTP(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("unconfigured status=%d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}
