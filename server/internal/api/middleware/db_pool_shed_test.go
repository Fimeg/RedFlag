package middleware_test

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Fimeg/RedFlag/server/internal/api/middleware"
	"github.com/gin-gonic/gin"
)

func makePoolShedRouter(stats func() sql.DBStats, cfg middleware.DBPoolShedConfig) *gin.Engine {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(middleware.DBPoolShed(stats, cfg))
	router.POST("/report", func(c *gin.Context) {
		c.String(http.StatusOK, "ok")
	})
	return router
}

func TestDBPoolShed(t *testing.T) {
	defaultCfg := middleware.DBPoolShedConfig{
		UtilizationThreshold: 1.0,
		RetryAfterSeconds:    2,
	}

	tests := []struct {
		name           string
		stats          sql.DBStats
		cfg            middleware.DBPoolShedConfig
		wantStatus     int
		wantRetryAfter bool
	}{
		{
			name: "unlimited pool never sheds",
			stats: sql.DBStats{
				MaxOpenConnections: 0, // unlimited
				InUse:              999,
			},
			cfg:            defaultCfg,
			wantStatus:     http.StatusOK,
			wantRetryAfter: false,
		},
		{
			name: "below threshold serves",
			stats: sql.DBStats{
				MaxOpenConnections: 100,
				InUse:              80,
			},
			cfg:            defaultCfg,
			wantStatus:     http.StatusOK,
			wantRetryAfter: false,
		},
		{
			name: "at threshold sheds",
			stats: sql.DBStats{
				MaxOpenConnections: 100,
				InUse:              100,
			},
			cfg:            defaultCfg,
			wantStatus:     http.StatusServiceUnavailable,
			wantRetryAfter: true,
		},
		{
			name: "above threshold sheds",
			stats: sql.DBStats{
				MaxOpenConnections: 100,
				InUse:              101,
			},
			cfg:            defaultCfg,
			wantStatus:     http.StatusServiceUnavailable,
			wantRetryAfter: true,
		},
		{
			name: "custom lower threshold sheds at partial saturation",
			stats: sql.DBStats{
				MaxOpenConnections: 100,
				InUse:              90,
			},
			cfg: middleware.DBPoolShedConfig{
				UtilizationThreshold: 0.85,
				RetryAfterSeconds:    5,
			},
			wantStatus:     http.StatusServiceUnavailable,
			wantRetryAfter: true,
		},
		{
			name: "custom lower threshold passes below threshold",
			stats: sql.DBStats{
				MaxOpenConnections: 100,
				InUse:              50,
			},
			cfg: middleware.DBPoolShedConfig{
				UtilizationThreshold: 0.85,
				RetryAfterSeconds:    5,
			},
			wantStatus:     http.StatusOK,
			wantRetryAfter: false,
		},
		{
			// Zero-value DBStats (e.g. no DB, empty struct): MaxOpenConnections == 0
			// means unlimited pool — must fail open and serve the request.
			name:           "zero-value stats fails open",
			stats:          sql.DBStats{},
			cfg:            defaultCfg,
			wantStatus:     http.StatusOK,
			wantRetryAfter: false,
		},
		{
			// Invalid threshold (<= 0) falls back to default 1.0 and does not shed
			// when the pool is not fully saturated.
			name: "invalid threshold falls back to default",
			stats: sql.DBStats{
				MaxOpenConnections: 100,
				InUse:              90,
			},
			cfg: middleware.DBPoolShedConfig{
				UtilizationThreshold: -0.5, // invalid — falls back to 1.0
				RetryAfterSeconds:    2,
			},
			wantStatus:     http.StatusOK,
			wantRetryAfter: false,
		},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			captured := tc.stats
			router := makePoolShedRouter(func() sql.DBStats { return captured }, tc.cfg)

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/report", nil)
			router.ServeHTTP(rec, req)

			if rec.Code != tc.wantStatus {
				t.Errorf("status=%d want=%d", rec.Code, tc.wantStatus)
			}
			hasRetryAfter := rec.Header().Get("Retry-After") != ""
			if hasRetryAfter != tc.wantRetryAfter {
				t.Errorf("Retry-After present=%v want=%v (header=%q)", hasRetryAfter, tc.wantRetryAfter, rec.Header().Get("Retry-After"))
			}
		})
	}
}
