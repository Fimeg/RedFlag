package main

import (
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/routeaudit"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/Fimeg/RedFlag/server/internal/taskrunner"
	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
)

// wireAuth registers the shared middleware instances with the route auditor.
// Must be called after all routes are registered and before AuditAndExit.
// One instance per trust boundary — calling the constructor twice produces a
// different closure and breaks RegisterAuth pointer-identity matching.
func wireAuth(auditor *routeaudit.Auditor, webMW, agentMW, metricsMW gin.HandlerFunc) {
	auditor.RegisterAuth("web", webMW)
	auditor.RegisterAuth("agent", agentMW)
	auditor.RegisterAuth("metrics", metricsMW)
}

// wireRetention creates the retention service and registers its periodic sweep
// on the background runner. Interval and jitter are passed in from operational
// settings (read by the caller via getOperationalSetting).
func wireRetention(bgRunner *taskrunner.Runner, db *sqlx.DB, settings *services.SecuritySettingsService, interval, jitter time.Duration) {
	retentionQueries := queries.NewRetentionQueries(db)
	retentionService := services.NewRetentionService(retentionQueries, settings)
	bgRunner.Every("retention_sweep", interval, jitter, retentionService.Sweep)
}
