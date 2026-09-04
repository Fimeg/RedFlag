package middleware

import (
	"database/sql"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
)

// DBPoolShedConfig holds the static config resolved once at construction.
type DBPoolShedConfig struct {
	// UtilizationThreshold is the fraction of MaxOpenConnections that must be
	// in use before shedding begins. 1.0 means shed only when fully saturated.
	// Valid range: (0, 1]. Values outside this range fall back to the default.
	UtilizationThreshold float64
	// RetryAfterSeconds is the value sent in the Retry-After response header.
	RetryAfterSeconds int
}

const (
	defaultUtilizationThreshold = 1.0
	defaultRetryAfterSeconds    = 2
)

// DBPoolShed returns a gin middleware that sheds requests with 503 when the
// database connection pool is saturated. It is a safety valve and fails OPEN:
//   - When MaxOpenConnections <= 0 (unlimited pool), shedding is disabled.
//   - When stats cannot meaningfully indicate saturation, the request is served.
//
// The stats provider and config are injected so the middleware is testable
// without a real database. Resolve config once in the caller (e.g. main.go)
// using env vars; do not put env parsing here.
func DBPoolShed(stats func() sql.DBStats, cfg DBPoolShedConfig) gin.HandlerFunc {
	threshold := cfg.UtilizationThreshold
	if threshold <= 0 || threshold > 1 {
		threshold = defaultUtilizationThreshold
	}
	retryAfter := cfg.RetryAfterSeconds
	if retryAfter <= 0 {
		retryAfter = defaultRetryAfterSeconds
	}

	return func(c *gin.Context) {
		s := stats()

		// Unlimited pool (MaxOpenConnections == 0): never shed.
		if s.MaxOpenConnections <= 0 {
			c.Next()
			return
		}

		utilization := float64(s.InUse) / float64(s.MaxOpenConnections)
		if utilization >= threshold {
			log.Printf("[WARNING] [server] [db_shed] pool_saturated in_use=%d max=%d utilization=%.2f threshold=%.2f",
				s.InUse, s.MaxOpenConnections, utilization, threshold)
			c.Header("Retry-After", fmt.Sprintf("%d", retryAfter))
			c.JSON(http.StatusServiceUnavailable, gin.H{"error": "database pool saturated, retry shortly"})
			c.Abort()
			return
		}

		c.Next()
	}
}
