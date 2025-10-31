package main

import (
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"time"

	"github.com/Fimeg/RedFlag/aggregator-server/internal/api/handlers"
	"github.com/Fimeg/RedFlag/aggregator-server/internal/api/middleware"
	"github.com/Fimeg/RedFlag/aggregator-server/internal/config"
	"github.com/Fimeg/RedFlag/aggregator-server/internal/database"
	"github.com/Fimeg/RedFlag/aggregator-server/internal/database/queries"
	"github.com/Fimeg/RedFlag/aggregator-server/internal/services"
	"github.com/gin-gonic/gin"
)

func startWelcomeModeServer() {
	setupHandler := handlers.NewSetupHandler("/app/config")
	router := gin.Default()

	// Add CORS middleware
	router.Use(middleware.CORSMiddleware())

	// Health check (all endpoints for compatibility)
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "waiting for configuration"})
	})
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "waiting for configuration"})
	})
	router.GET("/api/v1/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "waiting for configuration"})
	})

	// Welcome page with setup instructions
	router.GET("/", setupHandler.ShowSetupPage)

	// Setup endpoint for web configuration
	router.POST("/api/setup/configure", setupHandler.ConfigureServer)

	// Setup endpoint for web configuration
	router.GET("/setup", setupHandler.ShowSetupPage)

	log.Printf("Welcome mode server started on :8080")
	log.Printf("Waiting for configuration...")

	if err := router.Run(":8080"); err != nil {
		log.Fatal("Failed to start welcome mode server:", err)
	}
}

func main() {
	// Parse command line flags
	var setup bool
	var migrate bool
	var version bool
	flag.BoolVar(&setup, "setup", false, "Run setup wizard")
	flag.BoolVar(&migrate, "migrate", false, "Run database migrations only")
	flag.BoolVar(&version, "version", false, "Show version information")
	flag.Parse()

	// Handle special commands
	if version {
		fmt.Printf("RedFlag Server v0.1.0-alpha\n")
		fmt.Printf("Self-hosted update management platform\n")
		return
	}

	if setup {
		if err := config.RunSetupWizard(); err != nil {
			log.Fatal("Setup failed:", err)
		}
		return
	}

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Printf("Server waiting for configuration: %v", err)
		log.Printf("Run: docker-compose exec server ./redflag-server --setup")
		log.Printf("Or configure via web interface at: http://localhost:8080/setup")

		// Start welcome mode server
		startWelcomeModeServer()
		return
	}

	// Set JWT secret
	middleware.JWTSecret = cfg.Admin.JWTSecret

	// Build database URL from new config structure
	databaseURL := fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=disable",
		cfg.Database.Username, cfg.Database.Password, cfg.Database.Host, cfg.Database.Port, cfg.Database.Database)

	// Connect to database
	db, err := database.Connect(databaseURL)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer db.Close()

	// Handle migrate-only flag
	if migrate {
		migrationsPath := filepath.Join("internal", "database", "migrations")
		if err := db.Migrate(migrationsPath); err != nil {
			log.Fatal("Migration failed:", err)
		}
		fmt.Printf("✅ Database migrations completed\n")
		return
	}

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
	registrationTokenQueries := queries.NewRegistrationTokenQueries(db.DB)
	userQueries := queries.NewUserQueries(db.DB)

	// Ensure admin user exists
	if err := userQueries.EnsureAdminUser(cfg.Admin.Username, cfg.Admin.Username+"@redflag.local", cfg.Admin.Password); err != nil {
		fmt.Printf("Warning: Failed to create admin user: %v\n", err)
	} else {
		fmt.Println("✅ Admin user ensured")
	}

	// Initialize services
	timezoneService := services.NewTimezoneService(cfg)
	timeoutService := services.NewTimeoutService(commandQueries, updateQueries)

	// Initialize rate limiter
	rateLimiter := middleware.NewRateLimiter()

	// Initialize handlers
	agentHandler := handlers.NewAgentHandler(agentQueries, commandQueries, refreshTokenQueries, registrationTokenQueries, cfg.CheckInInterval, cfg.LatestAgentVersion)
	updateHandler := handlers.NewUpdateHandler(updateQueries, agentQueries, commandQueries, agentHandler)
	authHandler := handlers.NewAuthHandler(cfg.Admin.JWTSecret, userQueries)
	statsHandler := handlers.NewStatsHandler(agentQueries, updateQueries)
	settingsHandler := handlers.NewSettingsHandler(timezoneService)
	dockerHandler := handlers.NewDockerHandler(updateQueries, agentQueries, commandQueries)
	registrationTokenHandler := handlers.NewRegistrationTokenHandler(registrationTokenQueries, agentQueries, cfg)
	rateLimitHandler := handlers.NewRateLimitHandler(rateLimiter)
	downloadHandler := handlers.NewDownloadHandler(filepath.Join("/app"), cfg)

	// Setup router
	router := gin.Default()

	// Add CORS middleware
	router.Use(middleware.CORSMiddleware())

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})
	router.GET("/api/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})

	// API routes
	api := router.Group("/api/v1")
	{
		// Authentication routes (with rate limiting)
		api.POST("/auth/login", rateLimiter.RateLimit("public_access", middleware.KeyByIP), authHandler.Login)
		api.POST("/auth/logout", authHandler.Logout)
		api.GET("/auth/verify", authHandler.VerifyToken)

		// Public routes (no authentication required, with rate limiting)
		api.POST("/agents/register", rateLimiter.RateLimit("agent_registration", middleware.KeyByIP), agentHandler.RegisterAgent)
		api.POST("/agents/renew", rateLimiter.RateLimit("public_access", middleware.KeyByIP), agentHandler.RenewToken)

		// Public download routes (no authentication - agents need these!)
		api.GET("/downloads/:platform", rateLimiter.RateLimit("public_access", middleware.KeyByIP), downloadHandler.DownloadAgent)
		api.GET("/install/:platform", rateLimiter.RateLimit("public_access", middleware.KeyByIP), downloadHandler.InstallScript)

		// Protected agent routes
		agents := api.Group("/agents")
		agents.Use(middleware.AuthMiddleware())
		{
			agents.GET("/:id/commands", agentHandler.GetCommands)
			agents.POST("/:id/updates", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), updateHandler.ReportUpdates)
			agents.POST("/:id/logs", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), updateHandler.ReportLog)
			agents.POST("/:id/dependencies", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), updateHandler.ReportDependencies)
			agents.POST("/:id/system-info", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), agentHandler.ReportSystemInfo)
			agents.POST("/:id/rapid-mode", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), agentHandler.SetRapidPollingMode)
			agents.DELETE("/:id", agentHandler.UnregisterAgent)
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
			dashboard.POST("/agents/:id/heartbeat", agentHandler.TriggerHeartbeat)
			dashboard.GET("/agents/:id/heartbeat", agentHandler.GetHeartbeatStatus)
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
			dashboard.DELETE("/commands/failed", updateHandler.ClearFailedCommands)

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

			// Admin/Registration Token routes (for agent enrollment management)
			admin := dashboard.Group("/admin")
			{
				admin.POST("/registration-tokens", rateLimiter.RateLimit("admin_token_gen", middleware.KeyByUserID), registrationTokenHandler.GenerateRegistrationToken)
				admin.GET("/registration-tokens", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.ListRegistrationTokens)
				admin.GET("/registration-tokens/active", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.GetActiveRegistrationTokens)
				admin.DELETE("/registration-tokens/:token", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.RevokeRegistrationToken)
				admin.DELETE("/registration-tokens/delete/:id", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.DeleteRegistrationToken)
				admin.POST("/registration-tokens/cleanup", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.CleanupExpiredTokens)
				admin.GET("/registration-tokens/stats", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.GetTokenStats)
				admin.GET("/registration-tokens/validate", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.ValidateRegistrationToken)

				// Rate Limit Management
				admin.GET("/rate-limits", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), rateLimitHandler.GetRateLimitSettings)
				admin.PUT("/rate-limits", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), rateLimitHandler.UpdateRateLimitSettings)
				admin.POST("/rate-limits/reset", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), rateLimitHandler.ResetRateLimitSettings)
				admin.GET("/rate-limits/stats", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), rateLimitHandler.GetRateLimitStats)
				admin.POST("/rate-limits/cleanup", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), rateLimitHandler.CleanupRateLimitEntries)
			}
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
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("\nRedFlag Aggregator Server starting on %s\n", addr)
	fmt.Printf("Admin interface: http://%s:%d/admin\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("Dashboard: http://%s:%d\n\n", cfg.Server.Host, cfg.Server.Port)

	if err := router.Run(addr); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
