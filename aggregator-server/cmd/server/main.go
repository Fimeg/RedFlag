package main

import (
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/aggregator-project/aggregator-server/internal/api/handlers"
	"github.com/aggregator-project/aggregator-server/internal/api/middleware"
	"github.com/aggregator-project/aggregator-server/internal/config"
	"github.com/aggregator-project/aggregator-server/internal/database"
	"github.com/aggregator-project/aggregator-server/internal/database/queries"
	"github.com/aggregator-project/aggregator-server/internal/services"
	"github.com/gin-gonic/gin"
)

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Set JWT secret
	middleware.JWTSecret = cfg.JWTSecret

	// Connect to database
	db, err := database.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Run migrations
	migrationsPath := filepath.Join("internal", "database", "migrations")
	if err := db.Migrate(migrationsPath); err != nil {
		// For development, continue even if migrations fail
		// In production, you might want to handle this more gracefully
		fmt.Printf("Warning: Migration failed (tables may already exist): %v\n", err)
	}

	// Initialize queries
	agentQueries := queries.NewAgentQueries(db.DB)
	updateQueries := queries.NewUpdateQueries(db.DB)
	commandQueries := queries.NewCommandQueries(db.DB)
	refreshTokenQueries := queries.NewRefreshTokenQueries(db.DB)

	// Initialize services
	timezoneService := services.NewTimezoneService(cfg)
	timeoutService := services.NewTimeoutService(commandQueries, updateQueries)

	// Initialize handlers
	agentHandler := handlers.NewAgentHandler(agentQueries, commandQueries, refreshTokenQueries, cfg.CheckInInterval, cfg.LatestAgentVersion)
	updateHandler := handlers.NewUpdateHandler(updateQueries, agentQueries, commandQueries)
	authHandler := handlers.NewAuthHandler(cfg.JWTSecret)
	statsHandler := handlers.NewStatsHandler(agentQueries, updateQueries)
	settingsHandler := handlers.NewSettingsHandler(timezoneService)
	dockerHandler := handlers.NewDockerHandler(updateQueries, agentQueries, commandQueries)

	// Setup router
	router := gin.Default()

	// Add CORS middleware
	router.Use(middleware.CORSMiddleware())

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})

	// API routes
	api := router.Group("/api/v1")
	{
		// Authentication routes
		api.POST("/auth/login", authHandler.Login)
		api.POST("/auth/logout", authHandler.Logout)
		api.GET("/auth/verify", authHandler.VerifyToken)

		// Public routes (no authentication required)
		api.POST("/agents/register", agentHandler.RegisterAgent)
		api.POST("/agents/renew", agentHandler.RenewToken)

		// Protected agent routes
		agents := api.Group("/agents")
		agents.Use(middleware.AuthMiddleware())
		{
			agents.GET("/:id/commands", agentHandler.GetCommands)
			agents.POST("/:id/updates", updateHandler.ReportUpdates)
			agents.POST("/:id/logs", updateHandler.ReportLog)
			agents.POST("/:id/dependencies", updateHandler.ReportDependencies)
		}

		// Dashboard/Web routes (protected by web auth)
		dashboard := api.Group("/")
		dashboard.Use(authHandler.WebAuthMiddleware())
		{
			dashboard.GET("/stats/summary", statsHandler.GetDashboardStats)
			dashboard.GET("/agents", agentHandler.ListAgents)
			dashboard.GET("/agents/:id", agentHandler.GetAgent)
			dashboard.POST("/agents/:id/scan", agentHandler.TriggerScan)
			dashboard.POST("/agents/:id/update", agentHandler.TriggerUpdate)
			dashboard.DELETE("/agents/:id", agentHandler.UnregisterAgent)
			dashboard.GET("/updates", updateHandler.ListUpdates)
			dashboard.GET("/updates/:id", updateHandler.GetUpdate)
			dashboard.GET("/updates/:id/logs", updateHandler.GetUpdateLogs)
			dashboard.POST("/updates/:id/approve", updateHandler.ApproveUpdate)
			dashboard.POST("/updates/approve", updateHandler.ApproveUpdates)
			dashboard.POST("/updates/:id/reject", updateHandler.RejectUpdate)
			dashboard.POST("/updates/:id/install", updateHandler.InstallUpdate)
			dashboard.POST("/updates/:id/confirm-dependencies", updateHandler.ConfirmDependencies)

			// Log routes
			dashboard.GET("/logs", updateHandler.GetAllLogs)
			dashboard.GET("/logs/active", updateHandler.GetActiveOperations)

			// Command routes
			dashboard.GET("/commands/active", updateHandler.GetActiveCommands)
			dashboard.GET("/commands/recent", updateHandler.GetRecentCommands)
			dashboard.POST("/commands/:id/retry", updateHandler.RetryCommand)
			dashboard.POST("/commands/:id/cancel", updateHandler.CancelCommand)

			// Settings routes
			dashboard.GET("/settings/timezone", settingsHandler.GetTimezone)
			dashboard.GET("/settings/timezones", settingsHandler.GetTimezones)
			dashboard.PUT("/settings/timezone", settingsHandler.UpdateTimezone)

			// Docker routes
			dashboard.GET("/docker/containers", dockerHandler.GetContainers)
			dashboard.GET("/docker/stats", dockerHandler.GetStats)
			dashboard.POST("/docker/containers/:container_id/images/:image_id/approve", dockerHandler.ApproveUpdate)
			dashboard.POST("/docker/containers/:container_id/images/:image_id/reject", dockerHandler.RejectUpdate)
			dashboard.POST("/docker/containers/:container_id/images/:image_id/install", dockerHandler.InstallUpdate)
		}
	}

	// Start background goroutine to mark offline agents
	// TODO: Make these values configurable via settings:
	// - Check interval (currently 2 minutes, should match agent heartbeat setting)
	// - Offline threshold (currently 10 minutes, should be based on agent check-in interval + missed checks)
	// - Missed checks before offline (default 2, so 300s agent interval * 2 = 10 minutes)
	go func() {
		ticker := time.NewTicker(2 * time.Minute) // Check every 2 minutes
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Mark agents as offline if they haven't checked in within 10 minutes
				if err := agentQueries.MarkOfflineAgents(10 * time.Minute); err != nil {
					log.Printf("Failed to mark offline agents: %v", err)
				}
			}
		}
	}()

	// Start timeout service
	timeoutService.Start()
	log.Println("Timeout service started")

	// Add graceful shutdown for timeout service
	defer func() {
		timeoutService.Stop()
		log.Println("Timeout service stopped")
	}()

	// Start server
	addr := ":" + cfg.ServerPort
	fmt.Printf("\n🚩 RedFlag Aggregator Server starting on %s\n\n", addr)
	if err := router.Run(addr); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
