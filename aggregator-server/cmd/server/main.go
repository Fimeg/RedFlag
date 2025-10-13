package main

import (
	"fmt"
	"log"
	"path/filepath"

	"github.com/aggregator-project/aggregator-server/internal/api/handlers"
	"github.com/aggregator-project/aggregator-server/internal/api/middleware"
	"github.com/aggregator-project/aggregator-server/internal/config"
	"github.com/aggregator-project/aggregator-server/internal/database"
	"github.com/aggregator-project/aggregator-server/internal/database/queries"
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
		log.Fatal("Failed to run migrations:", err)
	}

	// Initialize queries
	agentQueries := queries.NewAgentQueries(db.DB)
	updateQueries := queries.NewUpdateQueries(db.DB)
	commandQueries := queries.NewCommandQueries(db.DB)

	// Initialize handlers
	agentHandler := handlers.NewAgentHandler(agentQueries, commandQueries, cfg.CheckInInterval)
	updateHandler := handlers.NewUpdateHandler(updateQueries)

	// Setup router
	router := gin.Default()

	// Health check
	router.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "healthy"})
	})

	// API routes
	api := router.Group("/api/v1")
	{
		// Public routes
		api.POST("/agents/register", agentHandler.RegisterAgent)

		// Protected agent routes
		agents := api.Group("/agents")
		agents.Use(middleware.AuthMiddleware())
		{
			agents.GET("/:id/commands", agentHandler.GetCommands)
			agents.POST("/:id/updates", updateHandler.ReportUpdates)
			agents.POST("/:id/logs", updateHandler.ReportLog)
		}

		// Dashboard/Web routes (will add proper auth later)
		api.GET("/agents", agentHandler.ListAgents)
		api.GET("/agents/:id", agentHandler.GetAgent)
		api.POST("/agents/:id/scan", agentHandler.TriggerScan)
		api.GET("/updates", updateHandler.ListUpdates)
		api.GET("/updates/:id", updateHandler.GetUpdate)
		api.POST("/updates/:id/approve", updateHandler.ApproveUpdate)
	}

	// Start server
	addr := ":" + cfg.ServerPort
	fmt.Printf("\n🚩 RedFlag Aggregator Server starting on %s\n\n", addr)
	if err := router.Run(addr); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
