package main

import (
	"context"
	"crypto/ed25519"
	"database/sql"
	"encoding/hex"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/api/handlers"
	"github.com/Fimeg/RedFlag/server/internal/api/middleware"
	"github.com/Fimeg/RedFlag/server/internal/circuitbreaker"
	"github.com/Fimeg/RedFlag/server/internal/command"
	"github.com/Fimeg/RedFlag/server/internal/config"
	"github.com/Fimeg/RedFlag/server/internal/database"
	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/integrations/wazuh"
	"github.com/Fimeg/RedFlag/server/internal/logging"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/Fimeg/RedFlag/server/internal/observability"
	"github.com/Fimeg/RedFlag/server/internal/orchestrator"
	"github.com/Fimeg/RedFlag/server/internal/routeaudit"
	"github.com/Fimeg/RedFlag/server/internal/scheduler"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/Fimeg/RedFlag/server/internal/services/notifier"
	"github.com/Fimeg/RedFlag/server/internal/services/upstream"
	"github.com/Fimeg/RedFlag/server/internal/taskrunner"
	"github.com/Fimeg/RedFlag/server/internal/version"
	"github.com/Fimeg/RedFlag/server/internal/webui"
	"github.com/gin-gonic/gin"
	"github.com/gofrs/uuid/v5"
)

// validateSigningService performs a test sign/verify to ensure the key is valid
func validateSigningService(signingService *services.SigningService) error {
	if signingService == nil {
		return fmt.Errorf("signing service is nil")
	}

	// Verify the key is accessible by getting public key and fingerprint
	publicKeyHex := signingService.GetPublicKey()
	if publicKeyHex == "" {
		return fmt.Errorf("failed to get public key from signing service")
	}

	fingerprint := signingService.GetPublicKeyFingerprint()
	if fingerprint == "" {
		return fmt.Errorf("failed to get public key fingerprint")
	}

	// Basic validation: Ed25519 public key should be 64 hex characters (32 bytes)
	if len(publicKeyHex) != 64 {
		return fmt.Errorf("invalid public key length: expected 64 hex chars, got %d", len(publicKeyHex))
	}

	return nil
}

// isSetupComplete checks if the server has been fully configured
// Returns true if all required components are ready for production
// Components checked: admin credentials, signing keys, database connectivity
func isSetupComplete(cfg *config.Config, signingService *services.SigningService, db *database.DB) bool {
	// Check if signing keys are configured
	if cfg.SigningPrivateKey == "" {
		log.Printf("Setup incomplete: Signing keys not configured")
		return false
	}

	// Check if admin password is configured (not empty)
	if cfg.Admin.Password == "" {
		log.Printf("Setup incomplete: Admin password not configured")
		return false
	}

	// Check if JWT secret is configured
	if cfg.Admin.JWTSecret == "" {
		log.Printf("Setup incomplete: JWT secret not configured")
		return false
	}

	// Check if database connection is working
	if err := db.DB.Ping(); err != nil {
		log.Printf("Setup incomplete: Database not accessible: %v", err)
		return false
	}

	// Check if database has been migrated (check for agents table)
	var agentCount int
	if err := db.DB.Get(&agentCount, "SELECT COUNT(*) FROM information_schema.tables WHERE table_name = 'agents'"); err != nil {
		log.Printf("Setup incomplete: Database migrations not complete - agents table does not exist")
		return false
	}

	// All critical checks passed
	log.Printf("Setup validation passed: All required components configured")
	return true
}

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

	// Setup endpoint for web configuration
	router.POST("/api/setup/configure", setupHandler.ConfigureServer)
	router.POST("/api/setup/generate-keys", setupHandler.GenerateSigningKeys)

	if webui.Present() {
		registerWebUI(router)
	} else {
		// Fallback for UI-less development binaries.
		router.GET("/", setupHandler.ShowSetupPage)
		router.GET("/setup", setupHandler.ShowSetupPage)
	}

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
	var showVersion bool
	flag.BoolVar(&setup, "setup", false, "Run setup wizard")
	flag.BoolVar(&migrate, "migrate", false, "Run database migrations only")
	flag.BoolVar(&showVersion, "version", false, "Show version information")
	flag.Parse()

	// Handle special commands
	if showVersion {
		fmt.Printf("RedFlag Server v%s\n", version.AgentVersion)
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
		fmt.Printf("[OK] Database migrations completed\n")
		return
	}

	// Run migrations — abort on failure (F-B1-11 fix)
	migrationsPath := filepath.Join("internal", "database", "migrations")
	if err := db.Migrate(migrationsPath); err != nil {
		log.Fatalf("[ERROR] [server] [database] migration_failed error=%q — server cannot start with incomplete schema", err)
	}
	log.Printf("[INFO] [server] [database] migrations_complete")

	agentQueries := queries.NewAgentQueries(db.DB)
	updateQueries := queries.NewUpdateQueries(db.DB)
	commandQueries := queries.NewCommandQueries(db.DB)
	refreshTokenQueries := queries.NewRefreshTokenQueries(db.DB)
	registrationTokenQueries, err := queries.NewRegistrationTokenQueries(db.DB, cfg.TokenEncryptionKey)
	if err != nil {
		log.Fatalf("[FATAL] [server] [config] registration token queries: %v", err)
	}
	subsystemQueries := queries.NewSubsystemQueries(db.DB)
	maintenanceWindowQueries := queries.NewMaintenanceWindowQueries(db.DB)
	upstreamQueries := queries.NewUpstreamQueries(db.DB)
	agentTrackedSoftwareQueries := queries.NewAgentTrackedSoftwareQueries(db.DB)
	repologyQueries := queries.NewRepologyQueries(db.DB)
	reconciliationQueries := queries.NewReconciliationQueries(db.DB)
	agentUpdateQueries := queries.NewAgentUpdateQueries(db.DB)
	metricsQueries := queries.NewMetricsQueries(db.DB)
	dockerQueries := queries.NewDockerQueries(db.DB)
	storageMetricsQueries := queries.NewStorageMetricsQueries(db.DB.DB)
	processQueries := queries.NewProcessQueries(db.DB.DB)
	adminQueries := queries.NewAdminQueries(db.DB)

	// Create PackageQueries for accessing signed agent update packages
	packageQueries := queries.NewPackageQueries(db.DB)

	signingKeyQueries := queries.NewSigningKeyQueries(db.DB)

	// Initialize services
	timezoneService := services.NewTimezoneService(cfg)

	// Initialize and validate signing service if private key is configured
	var signingService *services.SigningService
	if cfg.SigningPrivateKey != "" {
		var err error
		signingService, err = services.NewSigningService(cfg.SigningPrivateKey)
		if err != nil {
			log.Printf("[ERROR] Failed to initialize signing service: %v", err)
			log.Printf("[WARNING] Agent update signing is DISABLED - agents cannot be updated")
			log.Printf("[INFO] To fix: Generate signing keys at /api/setup/generate-keys and add to .env")
		} else {
			// Validate the signing key works by performing a test sign/verify
			if err := validateSigningService(signingService); err != nil {
				log.Printf("[ERROR] Signing key validation failed: %v", err)
				log.Printf("[WARNING] Agent update signing is DISABLED - key is corrupted")
				signingService = nil // Disable signing
			} else {
				log.Printf("[system] Ed25519 signing service initialized and validated")
				log.Printf("[system] Public key fingerprint: %s", signingService.GetPublicKeyFingerprint())
			}
		}
	} else {
		log.Printf("[WARNING] No signing private key configured - agent update signing disabled")
		log.Printf("[INFO] Generate keys: POST /api/setup/generate-keys")
	}

	if signingService != nil {
		signingService.SetSigningKeyQueries(signingKeyQueries)
		if err := signingService.InitializePrimaryKey(context.Background()); err != nil {
			log.Printf("[WARNING] Failed to register signing key in database: %v", err)
		} else {
			log.Printf("[system] Signing key registered in database")
		}
	}

	// ISSUE-002: Initialize BuildOrchestratorService to sign binaries at startup
	var buildOrchestrator *services.BuildOrchestratorService
	if signingService != nil && signingService.IsEnabled() {
		buildOrchestrator = services.NewBuildOrchestratorService(signingService, packageQueries, filepath.Join("/app"))
		log.Printf("[system] BuildOrchestratorService initialized - will sign agent binaries")
		// Sign all pre-built binaries at startup
		platforms := []string{"linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64", "windows-arm64"}
		for _, platform := range platforms {
			parts := strings.SplitN(platform, "-", 2)
			if len(parts) == 2 {
				_, err := buildOrchestrator.BuildAndSignAgent(version.AgentVersion, parts[0], parts[1])
				if err != nil {
					log.Printf("[WARNING] Failed to sign %s binary: %v", platform, err)
				} else {
					log.Printf("[system] Signed agent binary: %s", platform)
				}
			}
		}

		// Sign the capability-gate executor as a first-class artifact so the
		// installer can verify it against the signed manifest + per-binary
		// signature, exactly like the agent. Stored under platform
		// "helper-linux". Missing binary (e.g. arm64 not cross-built) is logged
		// and omitted — the manifest never lies about what it can attest.
		helperArches := []string{"amd64", "arm64"}
		for _, arch := range helperArches {
			helperPath := filepath.Join("/app", "binaries", "helper-linux-"+arch, "redflag-helper")
			if _, err := buildOrchestrator.SignExistingBinary(helperPath, version.AgentVersion, "helper-linux", arch); err != nil {
				log.Printf("[WARNING] Failed to sign helper binary (helper-linux-%s): %v", arch, err)
			} else {
				log.Printf("[system] Signed helper binary: helper-linux-%s", arch)
			}
		}

		// Sign native Desktop binaries.
		// Same pattern as helper: stored under "desktop-<os>", listed in
		// the release manifest. Missing binary is non-fatal — installer skips.
		desktopArches := []string{"amd64"}
		for _, arch := range desktopArches {
			desktopPath := filepath.Join("/app", "binaries", "linux-"+arch, "redflag-desktop")
			if _, err := buildOrchestrator.SignExistingBinary(desktopPath, version.AgentVersion, "desktop-linux", arch); err != nil {
				log.Printf("[WARNING] Failed to sign desktop binary (desktop-linux-%s): %v", arch, err)
			} else {
				log.Printf("[system] Signed desktop binary: desktop-linux-%s", arch)
			}
			winDesktopPath := filepath.Join("/app", "binaries", "windows-"+arch, "redflag-desktop.exe")
			if _, err := buildOrchestrator.SignExistingBinary(winDesktopPath, version.AgentVersion, "desktop-windows", arch); err != nil {
				log.Printf("[WARNING] Failed to sign desktop binary (desktop-windows-%s): %v", arch, err)
			} else {
				log.Printf("[system] Signed desktop binary: desktop-windows-%s", arch)
			}
		}
	} else {
		log.Printf("[WARNING] BuildOrchestratorService not initialized - signing disabled")
	}

	// Initialize default security settings (critical for v0.2.x)
	fmt.Println("[OK] Initializing default security settings...")
	securitySettingsQueries := queries.NewSecuritySettingsQueries(db.DB)
	securitySettingsService, err := services.NewSecuritySettingsService(securitySettingsQueries, signingService, cfg.SettingsEncryptionKey)
	if err != nil {
		fmt.Printf("Warning: Failed to create security settings service: %v\n", err)
		fmt.Println("Security settings will need to be configured manually via the dashboard")
	} else if err := securitySettingsService.InitializeDefaultSettings(); err != nil {
		fmt.Printf("Warning: Failed to initialize default security settings: %v\n", err)
		fmt.Println("Security settings will need to be configured manually via the dashboard")
	} else {
		fmt.Println("[OK] Default security settings initialized")
	}

	// Read operational timeout settings from DB (with hardcoded fallbacks)
	offlineCheckInterval := time.Duration(getOperationalSetting(securitySettingsService, "offline_check_interval_seconds", 120)) * time.Second
	offlineThreshold := time.Duration(getOperationalSetting(securitySettingsService, "offline_threshold_minutes", 10)) * time.Minute
	tokenCleanupInterval := time.Duration(getOperationalSetting(securitySettingsService, "token_cleanup_interval_hours", 24)) * time.Hour
	sentTimeout := time.Duration(getOperationalSetting(securitySettingsService, "sent_command_timeout_hours", 2)) * time.Hour
	pendingTimeout := time.Duration(getOperationalSetting(securitySettingsService, "pending_command_timeout_minutes", 30)) * time.Minute
	receivedTimeout := time.Duration(getOperationalSetting(securitySettingsService, "received_command_timeout_minutes", 30)) * time.Minute
	updateTimeout := time.Duration(getOperationalSetting(securitySettingsService, "agent_update_timeout_minutes", 15)) * time.Minute
	checkInterval := time.Duration(getOperationalSetting(securitySettingsService, "timeout_check_interval_minutes", 5)) * time.Minute
	stuckCommandTimeout := time.Duration(getOperationalSetting(securitySettingsService, "stuck_command_timeout_minutes", 5)) * time.Minute
	maxCommandRetries := getOperationalSetting(securitySettingsService, "max_command_retries", 5)

	log.Printf("[INFO] [server] [config] operational_timeouts_loaded offline_check=%s offline_threshold=%s token_cleanup=%s sent_cmd_timeout=%s pending_cmd_timeout=%s received_cmd_timeout=%s update_timeout=%s timeout_check=%s stuck_cmd_timeout=%s max_cmd_retries=%d",
		offlineCheckInterval, offlineThreshold, tokenCleanupInterval, sentTimeout, pendingTimeout, receivedTimeout, updateTimeout, checkInterval, stuckCommandTimeout, maxCommandRetries)

	timeoutService := services.NewTimeoutService(commandQueries, updateQueries, agentQueries, sentTimeout, pendingTimeout, receivedTimeout, updateTimeout, checkInterval)
	// Wire policy/operational settings so the reconciler can honor
	// operational.update_stuck_minutes at runtime without restart.
	if securitySettingsService != nil {
		timeoutService.SetSecuritySettings(securitySettingsService)
	}

	// Check if setup is complete
	if !isSetupComplete(cfg, signingService, db) {
		serverAddr := cfg.Server.Host
		if serverAddr == "" {
			serverAddr = "localhost"
		}
		log.Printf("Server setup incomplete - starting welcome mode")
		log.Printf("Setup required: Admin credentials, signing keys, and database configuration")
		log.Printf("Access setup at: http://%s:%d/setup", serverAddr, cfg.Server.Port)
		startWelcomeModeServer()
		return
	}

	// Initialize admin user from .env configuration
	fmt.Println("[OK] Initializing admin user...")
	if err := adminQueries.CreateAdminIfNotExists(cfg.Admin.Username, cfg.Admin.Email, cfg.Admin.Password); err != nil {
		log.Printf("[ERROR] Failed to initialize admin user: %v", err)
	} else {
		// Update admin password from .env (runs on every startup to keep in sync)
		if err := adminQueries.UpdateAdminPassword(cfg.Admin.Username, cfg.Admin.Password); err != nil {
			log.Printf("[WARNING] Failed to update admin password: %v", err)
		} else {
			fmt.Println("[OK] Admin user initialized")
		}
	}

	// Initialize security logger
	secConfig := logging.SecurityLogConfig{
		Enabled:         true, // Could be configurable in the future
		Level:           "warning",
		LogSuccesses:    false,
		FilePath:        "/var/log/redflag/security.json",
		MaxSizeMB:       100,
		MaxFiles:        10,
		RetentionDays:   90,
		LogToDatabase:   true,
		HashIPAddresses: true,
	}
	securityLogger, err := logging.NewSecurityLogger(secConfig, db.DB)
	if err != nil {
		log.Printf("Failed to initialize security logger: %v", err)
		securityLogger = nil
	}

	// Wazuh queue-socket mirror (INTEG-001). Opt-in, outbound-only; disabled
	// means the socket is never opened. The security journal stays authoritative.
	if securityLogger != nil && os.Getenv("REDFLAG_WAZUH_ENABLED") == "true" {
		socketPath := os.Getenv("REDFLAG_WAZUH_SOCKET")
		securityLogger.SetSink(wazuh.New(socketPath))
		log.Printf("[INFO] [server] [wazuh-emitter] enabled, socket=%s",
			func() string {
				if socketPath == "" {
					return wazuh.DefaultSocketPath
				}
				return socketPath
			}())
	}

	// Initialize rate limiter
	rateLimiter := middleware.NewRateLimiter()

	// Initialize handlers that don't depend on agentHandler (can be created now)
	authHandler := handlers.NewAuthHandler(cfg.Admin.JWTSecret, adminQueries)

	// AUDIT-002: Capture shared middleware instances for route audit pointer-identity.
	// One instance per trust boundary — calling the constructor twice produces a
	// different closure and breaks RegisterAuth matching.
	webAuthMW := authHandler.WebAuthMiddleware()
	agentAuthMW := middleware.AuthMiddleware()
	metricsAuthMW := middleware.MetricsBearerAuth(func() middleware.MetricsAuthConfig {
		return resolveMetricsAuthConfig(securitySettingsService)
	})
	statsHandler := handlers.NewStatsHandler(agentQueries, updateQueries, cfg.CheckInInterval)
	settingsHandler := handlers.NewSettingsHandler(timezoneService)
	dockerHandler := handlers.NewDockerHandler(dockerQueries, updateQueries, agentQueries, commandQueries, signingService, securityLogger)
	registrationTokenHandler := handlers.NewRegistrationTokenHandler(registrationTokenQueries, agentQueries, cfg)
	var fleetJoinHandler *handlers.FleetJoinHandler
	if signingService != nil && signingService.IsEnabled() {
		fleetJoinHandler = handlers.NewFleetJoinHandler(db.DB, registrationTokenQueries, agentQueries, signingService.GetPublicKey(), cfg)
	}
	maintenanceWindowHandler := handlers.NewMaintenanceWindowHandler(maintenanceWindowQueries)

	upstreamRegistry := upstream.NewRegistry()
	upstreamRegistry.Register(upstream.NewRepology())
	upstreamRegistry.Register(upstream.NewEndOfLife())
	upstreamRegistry.Register(upstream.NewGithubReleases())
	upstreamRegistry.Register(upstream.NewForgejoReleases())
	upstreamRegistry.Register(upstream.NewGiteaReleases())
	upstreamRegistry.Register(upstream.NewGitlabReleases())
	upstreamRegistry.Register(upstream.NewBitbucketTags())
	upstreamRegistry.Register(upstream.NewGitTags())
	selfEnabled := true
	trackPrereleases := true
	selfCurrentVersion := version.AgentVersion
	if selfCurrentVersion != "" && !strings.HasPrefix(selfCurrentVersion, "v") {
		selfCurrentVersion = "v" + selfCurrentVersion
	}
	if inserted, err := upstreamQueries.SeedSelf(models.TrackedSoftwareInput{
		Name:                  "RedFlag",
		Ecosystem:             "system",
		Source:                "forgejo",
		SourceRef:             "codeberg.org/Fimeg/RedFlag",
		CurrentVersion:        &selfCurrentVersion,
		Enabled:               &selfEnabled,
		TrackPrereleases:      &trackPrereleases,
		RepologySlug:          nil,
		ContainerImagePattern: nil,
	}); err != nil {
		log.Printf("[WARN] [server] [upstream] seed RedFlag self-tracking failed: %v", err)
	} else if inserted {
		log.Printf("[INFO] [server] [upstream] seeded RedFlag self-tracking source=forgejo ref=codeberg.org/Fimeg/RedFlag current=%s track_prereleases=true", selfCurrentVersion)
	}
	upstreamSyncer := upstream.NewSyncer(upstreamQueries, upstreamRegistry, time.Hour, 6*time.Hour, 50, repologyQueries)
	upstreamSyncer.Start(context.Background())

	// SCALE-001 S6/S2: bounded pool for the report path's fire-and-forget work
	// (OSV checks, reconcile, scan-set closure, immediate upstream sync). Caps
	// steady-state concurrency and makes saturation observable at /health/tasks
	// instead of spawning an unbounded goroutine per agent report.
	bgRunner := taskrunner.New(
		envInt("REDFLAG_TASKRUNNER_WORKERS", 8),
		envInt("REDFLAG_TASKRUNNER_QUEUE", 256),
	)

	// SCALE-001 S3: sweep expired rate-limit entries on a cadence so the
	// in-memory map can't grow unbounded. CleanupExpiredEntries was previously
	// only reachable via a manual admin endpoint — nothing called it. (Also the
	// first existing ticker migrated onto the runner, per S7.)
	bgRunner.Every("rate_limit_cleanup", 5*time.Minute, 30*time.Second, rateLimiter.CleanupExpiredEntries)

	upstreamHandler := handlers.NewUpstreamHandler(upstreamQueries, upstreamSyncer, upstreamRegistry)
	upstreamHandler.SetTaskRunner(bgRunner)

	// BRIDGE-001: Reconciliation service — auto-matches agent packages to tracked software
	repologyCache := upstream.NewRepologyCache(repologyQueries)
	reconciler := services.NewReconciler(agentTrackedSoftwareQueries, upstreamQueries, reconciliationQueries, repologyCache)
	// Run an immediate pass so existing packages are reconciled on startup, then
	// hand the periodic tick to bgRunner for shutdown, panic isolation, and
	// /health/tasks visibility (mirrors the old loop()'s initial ReconcileAll call).
	bgRunner.Go("sw_reconcile_initial", func() { reconciler.ReconcileAll(context.Background()) })
	bgRunner.Every("sw_reconcile", time.Hour, 5*time.Minute, func() { reconciler.ReconcileAll(context.Background()) })
	agentTrackedSoftwareHandler := handlers.NewAgentTrackedSoftwareHandler(agentTrackedSoftwareQueries, upstreamQueries)
	rateLimitHandler := handlers.NewRateLimitHandler(rateLimiter)
	// ISSUE-002: Pass signingService for install script signature verification
	downloadHandler := handlers.NewDownloadHandler(filepath.Join("/app"), cfg, packageQueries, signingService)

	// Create command factory for consistent command creation
	commandFactory := command.NewFactory(commandQueries)
	subsystemHandler := handlers.NewSubsystemHandler(subsystemQueries, commandQueries, agentQueries, commandFactory, signingService, securityLogger)

	metricsHandler := handlers.NewMetricsHandler(metricsQueries, agentQueries, commandQueries)
	dockerReportsHandler := handlers.NewDockerReportsHandler(dockerQueries, agentQueries, commandQueries)
	storageMetricsHandler := handlers.NewStorageMetricsHandler(storageMetricsQueries)
	processHandler := handlers.NewProcessHandler(processQueries, agentQueries, commandQueries, signingService)
	agentSetupHandler := handlers.NewAgentSetupHandler(agentQueries)

	// Initialize scanner config handler (for user-configurable scanner timeouts)
	scannerConfigHandler := handlers.NewScannerConfigHandler(db.DB)

	// Initialize update nonce service (for version upgrade middleware)
	var updateNonceService *services.UpdateNonceService
	if signingService != nil && cfg.SigningPrivateKey != "" {
		// Decode private key for nonce service
		privateKeyBytes, err := hex.DecodeString(cfg.SigningPrivateKey)
		if err == nil && len(privateKeyBytes) == ed25519.PrivateKeySize {
			updateNonceService = services.NewUpdateNonceService(ed25519.PrivateKey(privateKeyBytes))
			log.Printf("[system] Update nonce service initialized for version upgrades")
		} else {
			log.Printf("[WARNING] Failed to initialize update nonce service: invalid private key")
		}
	}

	// Initialize system handler
	systemHandler := handlers.NewSystemHandler(signingService, signingKeyQueries)

	// Initialize security handler
	securityHandler := handlers.NewSecurityHandler(signingService, agentQueries, commandQueries)

	// Wire security settings handler (F-A3-13: re-enable security settings routes)
	var securitySettingsHandler *handlers.SecuritySettingsHandler
	if securitySettingsService != nil {
		securitySettingsHandler = handlers.NewSecuritySettingsHandler(securitySettingsService)
	}

	// Setup router
	router := gin.Default()
	recorder := routeaudit.NewRecorder(router)

	// Add CORS middleware
	recorder.Use(middleware.CORSMiddleware())

	// Expose server version on every response so the frontend can detect
	// restarts without a dedicated health poll.
	router.Use(func(c *gin.Context) {
		c.Header("X-RedFlag-Version", version.AgentVersion)
		c.Next()
	})

	// Health check
	recorder.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "healthy",
			"version": version.AgentVersion,
		})
	})
	recorder.GET("/api/health", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"status":  "healthy",
			"version": version.AgentVersion,
		})
	})

	// API routes
	api := recorder.Group("/api/v1")
	{
		// Authentication routes (with rate limiting)
		api.POST("/auth/login", rateLimiter.RateLimit("public_access", middleware.KeyByIP), authHandler.Login)
		api.POST("/auth/logout", authHandler.Logout)
		api.GET("/auth/verify", webAuthMW, authHandler.VerifyToken)

		// Public system routes (no authentication required)
		api.GET("/public-key", rateLimiter.RateLimit("public_access", middleware.KeyByIP), systemHandler.GetPublicKey)
		api.GET("/public-keys", rateLimiter.RateLimit("public_access", middleware.KeyByIP), systemHandler.GetActivePublicKeys)
		api.GET("/info", rateLimiter.RateLimit("public_access", middleware.KeyByIP), systemHandler.GetSystemInfo)

		// Agent setup routes (no authentication required, with rate limiting)
		api.POST("/setup/agent", rateLimiter.RateLimit("agent_setup", middleware.KeyByIP), agentSetupHandler.SetupAgent)
		api.GET("/setup/templates", rateLimiter.RateLimit("public_access", middleware.KeyByIP), agentSetupHandler.GetTemplates)
		api.POST("/setup/validate", rateLimiter.RateLimit("agent_setup", middleware.KeyByIP), agentSetupHandler.ValidateConfiguration)

		// Build orchestrator routes (admin-only)
		buildRoutes := api.Group("/build")
		buildRoutes.Use(webAuthMW)
		{
			buildRoutes.POST("/new", rateLimiter.RateLimit("agent_build", middleware.KeyByAgentID), handlers.NewAgentBuild)
			buildRoutes.POST("/upgrade/:agentID", rateLimiter.RateLimit("agent_build", middleware.KeyByAgentID), handlers.UpgradeAgentBuild)
			buildRoutes.POST("/detect", rateLimiter.RateLimit("agent_build", middleware.KeyByAgentID), handlers.DetectAgentInstallation)
		}

		// Public download routes (agent binaries and install scripts remain public for bootstrapping)
		api.GET("/downloads/:platform", rateLimiter.RateLimit("public_access", middleware.KeyByIP), downloadHandler.DownloadAgent)
		api.GET("/install/:platform", rateLimiter.RateLimit("public_access", middleware.KeyByIP), downloadHandler.InstallScript)

		// Signed release manifest — cold-start trust root the installer verifies
		// before executing a freshly-downloaded binary. Own path (not under
		// /downloads/) to avoid colliding with the /downloads/:platform param route.
		api.GET("/manifest", rateLimiter.RateLimit("public_access", middleware.KeyByIP), downloadHandler.DownloadManifest)

		// Capability-gate executor (redflag-helper). Own path (not under
		// /downloads/) to avoid colliding with the /downloads/:platform param
		// route, same as /manifest. Signed + manifest-verified at install time.
		api.GET("/helper/:arch", rateLimiter.RateLimit("public_access", middleware.KeyByIP), downloadHandler.DownloadHelper)

		// Native Desktop app. Optional — 404
		// if not built for this arch, installer skips gracefully.
		api.GET("/desktop/:platform/:arch", rateLimiter.RateLimit("public_access", middleware.KeyByIP), downloadHandler.DownloadDesktop)

		// Package artifact download (for hash computation at approval time)
		api.GET("/downloads/artifact", rateLimiter.RateLimit("public_access", middleware.KeyByIP), downloadHandler.DownloadPackageArtifact)

		// Protected download routes (F-A3-6, F-A3-7: require authentication + machine binding)
		api.GET("/downloads/updates/:package_id",
			agentAuthMW,
			middleware.MachineBindingMiddleware(agentQueries, cfg.MinAgentVersion),
			rateLimiter.RateLimit("public_access", middleware.KeyByIP),
			downloadHandler.DownloadUpdatePackage)
		api.GET("/downloads/config/:agent_id", webAuthMW, rateLimiter.RateLimit("public_access", middleware.KeyByIP), downloadHandler.HandleConfigDownload)
	}

	// Mark offline agents on a cadence (F-E1-3: configurable). On bgRunner for
	// shutdown, panic isolation, and /health/tasks visibility (SCALE-001 S7).
	bgRunner.Every("mark_offline_agents", offlineCheckInterval, 0, func() {
		if err := agentQueries.MarkOfflineAgents(offlineThreshold); err != nil {
			log.Printf("[ERROR] [server] [agents] mark_offline_failed error=%v", err)
		}
	})

	// Refresh token cleanup (F-B1-10 fix, F-E1-3: configurable)
	bgRunner.Every("refresh_token_cleanup", tokenCleanupInterval, 30*time.Second, func() {
		count, err := refreshTokenQueries.CleanupExpiredTokens()
		if err != nil {
			log.Printf("[ERROR] [server] [database] refresh_token_cleanup_failed error=%q", err)
		} else if count > 0 {
			log.Printf("[INFO] [server] [database] refresh_token_cleanup_complete removed=%d", count)
		}
	})

	// Timeout service — periodic sweep for stuck commands and updating agents.
	// bgRunner owns the ticker, shutdown, and panic isolation (SCALE-001 S7).
	log.Printf("[INFO] [server] [timeout] service_registered sent_timeout=%v pending_timeout=%v received_timeout=%v check_interval=%v",
		sentTimeout, pendingTimeout, receivedTimeout, checkInterval)
	bgRunner.Every("command_timeouts", checkInterval, 30*time.Second, timeoutService.CheckTimeouts)

	// Initialize and start scheduler
	schedulerConfig := scheduler.DefaultConfig()
	// SCALE-001 S4: cap jobs dispatched per tick so an aligned fleet can't dump
	// the whole queue at the DB pool in one tick. Overflow rolls to the next tick.
	schedulerConfig.MaxDispatchPerTick = envInt("REDFLAG_SCHEDULER_MAX_DISPATCH_PER_TICK", schedulerConfig.MaxDispatchPerTick)
	subsystemScheduler := scheduler.NewScheduler(schedulerConfig, agentQueries, commandQueries, subsystemQueries, signingService)
	// Wire scheduler into SubsystemHandler so DisableSubsystem can evict
	// jobs from the in-memory priority queue.
	subsystemHandler.SetScheduler(subsystemScheduler)

	// Initialize agentHandler now that scheduler is available
	agentHandler := handlers.NewAgentHandler(agentQueries, commandQueries, refreshTokenQueries, registrationTokenQueries, subsystemQueries, subsystemScheduler, signingService, securityLogger, cfg, cfg.CheckInInterval, cfg.LatestAgentVersion, stuckCommandTimeout, maxCommandRetries)
	if securitySettingsService != nil {
		// Wires policy.auto_heartbeat_enabled into the dispatch chokepoint so
		// state-changing commands auto-queue enable_heartbeat (system source).
		agentHandler.SetSecuritySettings(securitySettingsService)
	}

	// Initialize agent update handler now that agentHandler is available
	var agentUpdateHandler *handlers.AgentUpdateHandler
	if signingService != nil {
		agentUpdateHandler = handlers.NewAgentUpdateHandler(agentQueries, agentUpdateQueries, commandQueries, signingService, updateNonceService, agentHandler)
		if securitySettingsService != nil {
			// Wires policy.require_nonce.
			agentUpdateHandler.SetSecuritySettings(securitySettingsService)
		}
	}

	// Initialize updateHandler with the agentHandler reference
	updateHandler := handlers.NewUpdateHandler(updateQueries, agentQueries, commandQueries, agentHandler, maintenanceWindowQueries, cfg)
	updateHandler.SetTaskRunner(bgRunner) // SCALE-001 S2: bound the report-path fire-and-forget work
	if securitySettingsService != nil {
		// Wires policy.allow_dry_runs.
		updateHandler.SetSecuritySettings(securitySettingsService)
	}

	// Supply Chain Gate — mint signed capability tokens at approval and deliver
	// them to agents. Active only when signing is enabled; the authority role
	// requires the signing key.
	if signingService != nil && signingService.IsEnabled() {
		capabilityTokenQueries := queries.NewCapabilityTokenQueries(db.DB)
		capabilityMinter := services.NewCapabilityMinter(signingService, capabilityTokenQueries)
		updateHandler.SetCapabilityMinter(capabilityMinter, capabilityTokenQueries)
		if agentUpdateHandler != nil {
			agentUpdateHandler.SetCapabilityTokenQueries(capabilityTokenQueries)
		}
		log.Printf("[system] Capability token gate enabled (key_id=%s)", signingService.GetCurrentKeyID())
	}

	// Lifecycle orchestrator (LIFECYCLE-003) — auto-approves packages by policy
	// and recovers packages stuck in active states on a timer sweep. Requires the
	// settings service (it is the policy/timing source); the safe default is
	// auto-approval off, so enabling the orchestrator changes nothing until an
	// operator sets policy.auto_approve_max_severity.
	if securitySettingsService != nil {
		sweepInterval := time.Duration(getOperationalSetting(securitySettingsService, "orchestrator_sweep_interval_seconds", 60)) * time.Second
		eventLogger := services.NewSystemEventLogger(db.DB)

		// Set up external notification dispatch (ntfy, SMTP) if configured.
		// Best-effort; failures here do not block the orchestrator.
		disp := notifier.NewDispatcher(5 * time.Minute)
		if n := notifier.Setup(disp, securitySettingsService); n > 0 {
			eventLogger.SetDispatcher(disp)
			log.Printf("[INFO] [server] [notifier] dispatcher_started sinks=%v", disp.SinkIDs())
		}
		lifecycleOrchestrator := orchestrator.New(updateQueries, updateHandler, updateHandler, maintenanceWindowQueries, securitySettingsService, eventLogger, sweepInterval)
		updateHandler.SetOrchestrator(lifecycleOrchestrator)
		lifecycleOrchestrator.Start()
		log.Printf("[INFO] [server] [orchestrator] enabled sweep_interval=%s", sweepInterval)
	}

	// Wire reconciler into update handler so ReportUpdates triggers reconciliation
	updateHandler.SetReconciler(reconciler)

	// Initialize events handler [TD-003]
	eventsHandler := handlers.NewEventsHandler(agentQueries)
	agentEventsHandler := handlers.NewAgentEventsHandler(agentQueries)
	agentSecurityEventsHandler := handlers.NewAgentSecurityEventsHandler(agentQueries)
	globalEventsHandler := handlers.NewGlobalEventsHandler(agentQueries)
	inventoryHandler := handlers.NewInventoryHandler(queries.NewInventoryQueries(db.DB), agentQueries)

	// Add routes that depend on agentHandler (must be after agentHandler creation)
	api.POST("/agents/register", rateLimiter.RateLimit("agent_registration", middleware.KeyByIP), agentHandler.RegisterAgent)
	api.POST("/agents/renew", rateLimiter.RateLimit("public_access", middleware.KeyByIP), agentHandler.RenewToken)
	if fleetJoinHandler != nil {
		api.POST("/fleet-join", rateLimiter.RateLimit("agent_registration", middleware.KeyByIP), fleetJoinHandler.JoinFleet)
	}

	// Protected agent routes (with machine binding security)
	agents := api.Group("/agents")
	agents.Use(agentAuthMW)
	agents.Use(middleware.MachineBindingMiddleware(agentQueries, cfg.MinAgentVersion)) // v0.1.22: Prevent config copying
	// SCALE-001 S8: shed inbound agent writes when the DB pool is saturated so
	// write pressure does not cascade into a full connection-pool deadlock. Applied
	// only to the agent-report group (the high-volume write path). Health, metrics,
	// auth, and dashboard reads are deliberately excluded so observability and login
	// remain reachable under pool saturation.
	agents.Use(middleware.DBPoolShed(
		func() sql.DBStats { return db.DB.Stats() },
		resolveDBPoolShedConfig(),
	))
	{
		agents.GET("/:id/commands", rateLimiter.RateLimit("agent_checkin", middleware.KeyByAgentID), agentHandler.GetCommands)
		agents.GET("/:id/capability-tokens", rateLimiter.RateLimit("agent_checkin", middleware.KeyByAgentID), updateHandler.GetCapabilityTokens)
		agents.POST("/:id/capability-tokens/:token_id/receipt", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), updateHandler.ReportCapabilityResult)
		agents.GET("/:id/config", agentHandler.GetAgentConfig)
		agents.POST("/:id/updates", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), updateHandler.ReportUpdates)
		agents.POST("/:id/logs", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), updateHandler.ReportLog)
		agents.POST("/:id/dependencies", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), updateHandler.ReportDependencies)
		agents.POST("/:id/system-info", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), agentHandler.ReportSystemInfo)
		agents.POST("/:id/rapid-mode", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), agentHandler.SetRapidPollingMode)
		// BUG-013: DELETE /agents/:id intentionally NOT registered here.
		// It's an admin operation invoked from the dashboard, not an
		// agent-self-service endpoint. Lives under the web-auth group
		// below so the request is authenticated as the admin user (not
		// expected to carry an agent JWT or X-Machine-ID header). The
		// previous registration here caused MachineBindingMiddleware to
		// reject the admin's request with 401, which the SPA interpreted
		// as a stale-session and logged the admin out.

		// New dedicated endpoints for metrics and docker images (data classification fix)
		agents.POST("/:id/metrics", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), metricsHandler.ReportMetrics)
		agents.POST("/:id/docker-images", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), dockerReportsHandler.ReportDockerImages)

		// Dedicated storage metrics endpoint (proper separation from generic metrics)
		agents.POST("/:id/storage-metrics", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), storageMetricsHandler.ReportStorageMetrics)

		// Process scan reporting (on-demand, triggered by dashboard)
		agents.POST("/:id/process-scan", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), processHandler.ReportProcessScan)

		// Circuit breaker health reporting [ISSUE-004]
		agents.POST("/:id/circuit-breakers", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), agentHandler.ReportCircuitBreakerStats)

		// Event reporting [TD-003]
		agents.POST("/:id/events", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), eventsHandler.ReportEvents)

		// Security event reporting (agent → server security_events table)
		agents.POST("/:id/security-events", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), agentSecurityEventsHandler.ReportSecurityEvents)

		// Inventory reporting (agent → server agent_inventory table)
		agents.POST("/:id/inventory", rateLimiter.RateLimit("agent_reports", middleware.KeyByAgentID), inventoryHandler.ReportInventory)
		agents.GET("/:id/inventory", inventoryHandler.GetAgentInventory)
	}

	// Dashboard/Web routes (protected by web auth)
	dashboard := api.Group("/")
	dashboard.Use(webAuthMW)
	{
		dashboard.GET("/stats/summary", statsHandler.GetDashboardStats)
		dashboard.GET("/inventory", inventoryHandler.GetFleetInventory)
		dashboard.GET("/agents", agentHandler.ListAgents)
		dashboard.GET("/agents/:id", agentHandler.GetAgent)
		dashboard.GET("/agents/:id/storage-metrics", storageMetricsHandler.GetStorageMetrics)
		dashboard.GET("/agents/:id/processes", processHandler.GetLatestProcessSnapshot)
		dashboard.GET("/agents/:id/processes/:processId", processHandler.GetProcessDetail)
		dashboard.POST("/agents/:id/processes/scan", processHandler.TriggerProcessScan)
		dashboard.POST("/agents/:id/heartbeat", agentHandler.TriggerHeartbeat)
		dashboard.GET("/agents/:id/heartbeat", agentHandler.GetHeartbeatStatus)
		dashboard.POST("/agents/:id/reboot", agentHandler.TriggerReboot)
		dashboard.POST("/agents/:id/screenshot", agentHandler.TriggerCaptureScreenshot)
		// BUG-013: admin-initiated agent removal lives in the web-auth group
		// (not the agent-auth group above), since the caller is the dashboard
		// admin, not the agent itself.
		dashboard.DELETE("/agents/:id", agentHandler.UnregisterAgent)

		// Subsystem routes for web dashboard
		dashboard.GET("/agents/:id/subsystems", subsystemHandler.GetSubsystems)
		dashboard.GET("/agents/:id/subsystems/:subsystem", subsystemHandler.GetSubsystem)
		dashboard.PATCH("/agents/:id/subsystems/:subsystem", subsystemHandler.UpdateSubsystem)
		dashboard.POST("/agents/:id/subsystems/:subsystem/enable", subsystemHandler.EnableSubsystem)
		dashboard.POST("/agents/:id/subsystems/:subsystem/disable", subsystemHandler.DisableSubsystem)
		dashboard.POST("/agents/:id/subsystems/:subsystem/trigger", subsystemHandler.TriggerSubsystem)
		dashboard.GET("/agents/:id/subsystems/:subsystem/stats", subsystemHandler.GetSubsystemStats)
		dashboard.POST("/agents/:id/subsystems/:subsystem/auto-run", subsystemHandler.SetAutoRun)
		dashboard.POST("/agents/:id/subsystems/:subsystem/interval", subsystemHandler.SetInterval)

		// Client error logging (authenticated)
		clientErrorHandler := handlers.NewClientErrorHandler(db.DB)
		dashboard.POST("/logs/client-error", clientErrorHandler.LogError)
		dashboard.GET("/logs/client-errors", clientErrorHandler.GetErrors)

		dashboard.GET("/updates", updateHandler.ListUpdates)
		dashboard.GET("/packages", updateHandler.ListPackages)
		dashboard.GET("/updates/:id", updateHandler.GetUpdate)
		dashboard.GET("/updates/:id/logs", updateHandler.GetUpdateLogs)
		dashboard.GET("/updates/:id/fleet", updateHandler.GetPackageFleet)
		dashboard.GET("/updates/:id/versions", updateHandler.GetPackageVersions)
		dashboard.GET("/updates/:id/lifecycle", updateHandler.GetUpdateLifecycleHistory)
		dashboard.GET("/updates/package/:type/:name", updateHandler.GetPackageSummaryByCoords)
		dashboard.GET("/updates/package/:type/:name/agents", updateHandler.GetPackageAgentsByCoords)
		dashboard.GET("/updates/package/:type/:name/versions", updateHandler.GetPackageVersionsByCoords)
		dashboard.GET("/updates/package/:type/:name/vulnerabilities", updateHandler.GetPackageVulnerabilitiesByCoords)
		dashboard.POST("/updates/:id/approve", updateHandler.ApproveUpdate)
		dashboard.POST("/updates/approve", updateHandler.ApproveUpdates)
		dashboard.POST("/updates/:id/reject", updateHandler.RejectUpdate)
		dashboard.POST("/updates/:id/install", updateHandler.InstallUpdate)
		dashboard.POST("/updates/:id/install-version", updateHandler.InstallVersion)
		dashboard.POST("/updates/:id/reopen", updateHandler.ReopenUpdate)
		dashboard.POST("/updates/:id/resolve", updateHandler.ResolveUpdate)
		dashboard.POST("/updates/:id/confirm-dependencies", updateHandler.ConfirmDependencies)

		// Agent update routes
		if agentUpdateHandler != nil {
			dashboard.POST("/agents/:id/update", agentUpdateHandler.UpdateAgent)
			dashboard.POST("/agents/:id/update-nonce", agentUpdateHandler.GenerateUpdateNonce)
			dashboard.POST("/agents/bulk-update", agentUpdateHandler.BulkUpdateAgents)
			dashboard.GET("/updates/packages", agentUpdateHandler.ListUpdatePackages)
			dashboard.POST("/updates/packages/sign", agentUpdateHandler.SignUpdatePackage)
			dashboard.GET("/agents/:id/updates/available", agentUpdateHandler.CheckForUpdateAvailable)
			dashboard.GET("/agents/:id/updates/status", agentUpdateHandler.GetUpdateStatus)
		}

		dashboard.GET("/logs", updateHandler.GetAllLogs)
		dashboard.GET("/logs/active", updateHandler.GetActiveOperations)

		// Hash verification endpoint (Layer 1: Hash Registry)
		dashboard.GET("/updates/verify-hash", updateHandler.GetExpectedHash)
		dashboard.GET("/capability-tokens/status", updateHandler.GetCapabilityTokenStatus)

		// Command routes
		dashboard.GET("/commands/active", updateHandler.GetActiveCommands)
		dashboard.GET("/commands/recent", updateHandler.GetRecentCommands)
		dashboard.GET("/commands/:id", updateHandler.GetCommandByID)
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

		// Metrics and Docker images routes (data classification fix)
		dashboard.GET("/agents/:id/metrics", metricsHandler.GetAgentMetrics)
		dashboard.GET("/agents/:id/metrics/storage", metricsHandler.GetAgentStorageMetrics)
		dashboard.GET("/agents/:id/metrics/system", metricsHandler.GetAgentSystemMetrics)
		dashboard.GET("/agents/:id/events", agentEventsHandler.GetAgentEvents)
		dashboard.GET("/agents/:id/docker-images", dockerReportsHandler.GetAgentDockerImages)
		dashboard.GET("/agents/:id/docker-info", dockerReportsHandler.GetAgentDockerInfo)
		dashboard.GET("/agents/:id/docker-containers", dockerReportsHandler.GetAgentDockerContainers)
		dashboard.GET("/agents/:id/docker-stacks", dockerReportsHandler.GetAgentDockerStacks)
		dashboard.GET("/events", globalEventsHandler.GetGlobalEvents)
		dashboard.GET("/docker/fleet-containers", dockerReportsHandler.GetDockerContainersFleet)
		dashboard.GET("/docker/fleet-stacks", dockerReportsHandler.GetDockerStacksFleet)

		// Admin/Registration Token routes (for agent enrollment management)
		auditMW := middleware.NewAuditMiddleware(db.DB)
		admin := dashboard.Group("/admin")
		// Trust boundary latch (ETHOS 11.4): the /admin group requires the admin
		// role at the group level, not per-route. Inert today (login mints role
		// "admin" only, auth.go), but the day role issuance diversifies the gate
		// is already structurally correct — a non-admin can't reach any /admin
		// handler, and there's no per-route annotation to forget.
		admin.Use(middleware.RequireAdmin())
		admin.Use(auditMW.Audit())
		{
			admin.POST("/registration-tokens", rateLimiter.RateLimit("admin_token_gen", middleware.KeyByUserID), registrationTokenHandler.GenerateRegistrationToken)
			admin.GET("/registration-tokens", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.ListRegistrationTokens)
			admin.GET("/registration-tokens/active", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.GetActiveRegistrationTokens)
			admin.DELETE("/registration-tokens/:token", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.RevokeRegistrationToken)
			admin.DELETE("/registration-tokens/delete/:id", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.DeleteRegistrationToken)
			admin.POST("/registration-tokens/cleanup", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.CleanupExpiredTokens)
			admin.GET("/registration-tokens/stats", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.GetTokenStats)
			admin.GET("/registration-tokens/validate", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.ValidateRegistrationToken)
			// Token-detail expansion: which agents took seats on this token.
			// Spec: docs/AGENT_LIFECYCLE.md "Operator surfaces".
			admin.GET("/registration-tokens/:token/agents", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), registrationTokenHandler.GetAgentsBoundToToken)

			// Per-agent revocation (explicit; no cascade from token revocation).
			// Spec: docs/AGENT_LIFECYCLE.md "Revocation".
			admin.POST("/agents/:id/revoke", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), agentHandler.RevokeAgent)

			// Machine ID Rebind (F-D1-2: recovery from machine ID mismatch)
			admin.POST("/agents/:id/rebind-machine-id", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), agentHandler.RebindMachineID)

			// Device type override (SERVER-002: operator reclassification)
			admin.PUT("/agents/:id/device-type", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), agentHandler.ReclassifyDeviceType)

			// Signing key management — Ed25519 key rotation surface for the dashboard.
			// Operators can review every key the server has ever held and deprecate
			// retired keys (the queries layer refuses to deprecate the current primary).
			admin.GET("/signing-keys", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), systemHandler.ListSigningKeys)
			admin.POST("/signing-keys/:key_id/deprecate", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), systemHandler.DeprecateSigningKey)

			// Rate Limit Management
			admin.GET("/rate-limits", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), rateLimitHandler.GetRateLimitSettings)
			admin.PUT("/rate-limits", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), rateLimitHandler.UpdateRateLimitSettings)
			admin.POST("/rate-limits/reset", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), rateLimitHandler.ResetRateLimitSettings)
			admin.GET("/rate-limits/stats", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), rateLimitHandler.GetRateLimitStats)
			admin.POST("/rate-limits/cleanup", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), rateLimitHandler.CleanupRateLimitEntries)

			// Scanner Configuration (user-configurable timeouts)
			admin.GET("/scanner-timeouts", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), scannerConfigHandler.GetScannerTimeouts)
			admin.PUT("/scanner-timeouts/:scanner_name", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), scannerConfigHandler.UpdateScannerTimeout)
			admin.POST("/scanner-timeouts/:scanner_name/reset", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), scannerConfigHandler.ResetScannerTimeout)

			// Maintenance Windows (gating for install operations)
			admin.GET("/maintenance-windows", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), maintenanceWindowHandler.ListWindows)
			admin.GET("/maintenance-windows/:id", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), maintenanceWindowHandler.GetWindow)
			admin.POST("/maintenance-windows", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), maintenanceWindowHandler.CreateWindow)
			admin.PUT("/maintenance-windows/:id", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), maintenanceWindowHandler.UpdateWindow)
			admin.DELETE("/maintenance-windows/:id", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), maintenanceWindowHandler.DeleteWindow)
			admin.GET("/maintenance-windows/check", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), maintenanceWindowHandler.CheckWindow)

			// Upstream version sync (Repology, endoflife.date, forge releases, tags)
			admin.GET("/upstream", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), upstreamHandler.List)
			admin.GET("/upstream/drift", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), upstreamHandler.ListDrifted)
			admin.GET("/upstream/drift/events", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), upstreamHandler.RecentDrift)
			admin.POST("/upstream", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), upstreamHandler.Create)
			admin.PATCH("/upstream/:id", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), upstreamHandler.UpdateSettings)
			admin.DELETE("/upstream/:id", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), upstreamHandler.Delete)
			admin.POST("/upstream/:id/sync", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), upstreamHandler.SyncNow)
			admin.GET("/upstream/:id/installations", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), agentTrackedSoftwareHandler.ListInstallations)

			// Agent <-> tracked_software bindings: per-host view of upstream drift.
			// Routes live under the agent subtree so the binding's authorization
			// boundary is the agent id (queries.Delete enforces the scope).
			admin.GET("/agents/:id/tracked-software", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), agentTrackedSoftwareHandler.ListByAgent)
			admin.POST("/agents/:id/tracked-software", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), agentTrackedSoftwareHandler.Upsert)
			admin.DELETE("/agents/:id/tracked-software/:bindingID", rateLimiter.RateLimit("admin_operations", middleware.KeyByUserID), agentTrackedSoftwareHandler.Delete)
		}

		// Security Health Check endpoints
		dashboard.GET("/security/overview", securityHandler.SecurityOverview)
		dashboard.GET("/security/signing", securityHandler.SigningStatus)
		dashboard.GET("/security/nonce", securityHandler.NonceValidationStatus)
		dashboard.GET("/security/commands", securityHandler.CommandValidationStatus)
		dashboard.GET("/security/machine-binding", securityHandler.MachineBindingStatus)
		dashboard.GET("/security/metrics", securityHandler.SecurityMetrics)

		// Security Settings Management endpoints (admin-only)
		// F-A3-13 fix: RequireAdmin() middleware implemented, routes re-enabled
		if securitySettingsHandler != nil {
			securitySettings := dashboard.Group("/security/settings")
			securitySettings.Use(middleware.RequireAdmin())
			{
				securitySettings.GET("", securitySettingsHandler.GetAllSecuritySettings)
				securitySettings.GET("/audit", securitySettingsHandler.GetSecurityAuditTrail)
				securitySettings.GET("/overview", securitySettingsHandler.GetSecurityOverview)
				securitySettings.GET("/:category", securitySettingsHandler.GetSecuritySettingsByCategory)
				securitySettings.PUT("/:category/:key", securitySettingsHandler.UpdateSecuritySetting)
				securitySettings.POST("/validate", securitySettingsHandler.ValidateSecuritySettings)
				securitySettings.POST("/apply", securitySettingsHandler.ApplySecuritySettings)
			}
		}
	}

	// Load subsystems into queue
	ctx := context.Background()
	if err := subsystemScheduler.LoadSubsystems(ctx); err != nil {
		log.Printf("Warning: Failed to load subsystems: %v", err)
	} else {
		log.Println("Subsystems loaded into scheduler")
	}

	// Start scheduler
	if err := subsystemScheduler.Start(); err != nil {
		log.Printf("Warning: Failed to start scheduler: %v", err)
	}

	// Backfill OSV.dev checks for packages discovered before the supply-chain
	// check was wired at discovery time. Runs once at startup, async, best-effort.
	bgRunner.Go("osv_backfill", func() { backfillOSVChecks(updateQueries) })

	// Add scheduler stats endpoint (after scheduler is initialized)
	// F-A3-10 fix: use WebAuthMiddleware (admin only), not AuthMiddleware (agent JWT)
	recorder.GET("/api/v1/scheduler/stats", webAuthMW, func(c *gin.Context) {
		stats := subsystemScheduler.GetStats()
		queueStats := subsystemScheduler.GetQueueStats()
		c.JSON(200, gin.H{
			"scheduler": stats,
			"queue":     queueStats,
		})
	})

	// SCALE-001 S6: background-task observability — the live goroutine count plus
	// the bounded runner's counters and registered periodic tasks. Admin-only;
	// it exposes internal queue depth and saturation.
	recorder.GET("/api/v1/health/tasks", webAuthMW, func(c *gin.Context) {
		c.JSON(200, gin.H{
			"goroutines": runtime.NumGoroutine(),
			"taskrunner": bgRunner.Snapshot(),
			"breakers": gin.H{
				"osv":      services.OSVBreakerStats(),
				"repology": upstream.RepologyBreakerStats(),
			},
		})
	})

	// SCALE-001 S8 / OBS-001: operator-facing advisory-feed health. The OSV/repology
	// breakers fail OPEN (an unreachable advisory feed never blocks a patch — the
	// sovereignty bet), but the auto-confirm gate is fail-CLOSED, so a degraded feed
	// quietly suspends auto-approval and leaves packages unvetted. This makes that
	// state visible where the operator lives instead of a grep-the-logs secret.
	// Admin-only, cheaper than /health/tasks (a single COUNT), safe to poll.
	recorder.GET("/api/v1/health/advisory", webAuthMW, func(c *gin.Context) {
		osv := services.OSVBreakerStats()
		repology := upstream.RepologyBreakerStats()
		deferred, err := updateQueries.CountSupplyChainDeferred()
		if err != nil {
			log.Printf("[WARNING] [api] [health] advisory_deferred_count_failed error=%v", err)
			deferred = -1 // unknown — never claim zero deferred when we couldn't count
		}
		c.JSON(200, gin.H{
			"osv":               osv,
			"repology":          repology,
			"deferred_packages": deferred,
			// degraded when OSV isn't closed (auto-approval at risk) or anything's
			// still awaiting recheck. -1 (count unknown) also counts as degraded.
			"degraded": osv.State != "closed" || deferred != 0,
		})
	})

	metricsExporter := observability.NewExporter(
		func() sql.DBStats { return db.DB.Stats() },
		bgRunner.Snapshot,
		subsystemScheduler,
		map[string]func() circuitbreaker.Stats{
			"osv":      services.OSVBreakerStats,
			"repology": upstream.RepologyBreakerStats,
		},
		updateQueries.CountSupplyChainDeferred,
	)
	recorder.GET(
		"/metrics",
		metricsAuthMW,
		gin.WrapH(metricsExporter),
	)

	registerWebUI(router)

	// Add graceful shutdown for services
	defer func() {
		log.Println("Shutting down services...")

		// Stop scheduler first
		if err := subsystemScheduler.Stop(); err != nil {
			log.Printf("Error stopping scheduler: %v", err)
		}

		// Stop the upstream syncer (has its own ticker outside bgRunner).
		upstreamSyncer.Stop()

		// Stop the background runner last so in-flight fire-and-forget work from
		// the other services has a chance to drain. command_timeouts and sw_reconcile
		// are managed by bgRunner — no separate Stop calls needed.
		bgRunner.Stop()
		log.Println("Services stopped")
	}()

	// AUDIT-002: Register auth middleware instances with the route auditor.
	// RETAIN-001: Wire retention sweep onto the background runner.
	retentionInterval := time.Duration(getOperationalSetting(securitySettingsService, "retention_sweep_interval_seconds", 3600)) * time.Second
	retentionJitter := time.Duration(getOperationalSetting(securitySettingsService, "retention_sweep_jitter_seconds", 60)) * time.Second
	auditor := routeaudit.NewAuditor()
	wireAuth(auditor, webAuthMW, agentAuthMW, metricsAuthMW)
	wireRetention(bgRunner, db.DB, securitySettingsService, retentionInterval, retentionJitter)

	// Route audit: verify every route carries the auth its trust boundary requires.
	// Refuses to boot if any route is missing auth and not on the public allowlist.
	auditor.AuditAndExit(recorder)

	// Start server
	addr := fmt.Sprintf("%s:%d", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("\nRedFlag Aggregator Server starting on %s\n", addr)
	fmt.Printf("Admin interface: http://%s:%d/admin\n", cfg.Server.Host, cfg.Server.Port)
	fmt.Printf("Dashboard: http://%s:%d\n\n", cfg.Server.Host, cfg.Server.Port)

	if err := router.Run(addr); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}

// registerWebUI serves the embedded dashboard build (server/internal/webui)
// from the server binary itself — no nginx, no separate web container. The
// frontend calls relative /api/v1 paths, so same-origin serving needs no
// frontend changes. Binaries built without the UI copy step run API-only.
func registerWebUI(router *gin.Engine) {
	if !webui.Present() {
		log.Printf("[INFO] [server] [webui] no embedded UI build in this binary — serving API only")
		return
	}
	uiFS, err := webui.FS()
	if err != nil {
		log.Printf("[ERROR] [server] [webui] embedded UI unavailable: %v", err)
		return
	}
	fileServer := http.FileServer(http.FS(uiFS))

	router.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		// Unmatched API/infra paths stay JSON 404s — never fall through to HTML.
		if strings.HasPrefix(p, "/api/") || p == "/metrics" || p == "/health" {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}
		if c.Request.Method != http.MethodGet && c.Request.Method != http.MethodHead {
			c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
			return
		}

		clean := strings.TrimPrefix(path.Clean(p), "/")
		if clean == "" {
			clean = "index.html"
		}
		if _, statErr := fs.Stat(uiFS, clean); statErr == nil {
			// Vite asset filenames are content-hashed — safe to cache hard.
			if strings.HasPrefix(clean, "assets/") {
				c.Header("Cache-Control", "public, max-age=31536000, immutable")
			}
			fileServer.ServeHTTP(c.Writer, c.Request)
			return
		}

		// Client-side route (e.g. /agents/123) — serve the SPA shell.
		index, readErr := fs.ReadFile(uiFS, "index.html")
		if readErr != nil {
			log.Printf("[ERROR] [server] [webui] index.html read failed: %v", readErr)
			c.JSON(http.StatusInternalServerError, gin.H{"error": "ui unavailable"})
			return
		}
		c.Header("Cache-Control", "no-cache")
		c.Data(http.StatusOK, "text/html; charset=utf-8", index)
	})

	log.Printf("[INFO] [server] [webui] embedded dashboard enabled")
}

// getOperationalSetting reads a timeout value from the DB settings with fallback to a default.
func getOperationalSetting(svc *services.SecuritySettingsService, key string, defaultVal int) int {
	if svc == nil {
		return defaultVal
	}
	val, err := svc.GetSetting("operational", key)
	if err != nil {
		return defaultVal
	}
	if f, ok := val.(float64); ok && f > 0 {
		return int(f)
	}
	return defaultVal
}

// resolveMetricsAuthConfig reads the dedicated Prometheus scrape auth settings.
// Plaintext REDFLAG_METRICS_TOKEN is bootstrap-only; persisted/UI-managed
// rotation should store observability.metrics_token_hash (SHA-256 hex).
func resolveMetricsAuthConfig(svc *services.SecuritySettingsService) middleware.MetricsAuthConfig {
	enabled := false
	tokenHash := ""

	if svc != nil {
		if val, err := svc.GetSetting("observability", "metrics_enabled"); err == nil {
			if b, ok := val.(bool); ok {
				enabled = b
			}
		}
		if val, err := svc.GetSetting("observability", "metrics_token_hash"); err == nil {
			if s, ok := val.(string); ok {
				tokenHash = strings.TrimSpace(s)
			}
		}
	} else {
		enabled = strings.EqualFold(os.Getenv("REDFLAG_OBSERVABILITY_METRICS_ENABLED"), "true")
		tokenHash = strings.TrimSpace(os.Getenv("REDFLAG_OBSERVABILITY_METRICS_TOKEN_HASH"))
	}

	if tokenHash == "" {
		if token := os.Getenv("REDFLAG_METRICS_TOKEN"); token != "" {
			tokenHash = middleware.HashMetricsToken(token)
		}
	}

	return middleware.MetricsAuthConfig{
		Enabled:        enabled,
		TokenSHA256Hex: tokenHash,
	}
}

// envInt reads a positive integer from env, falling back to def when unset,
// empty, unparseable, or non-positive. A non-positive override is ignored so a
// stray "0"/"-1" can't silently disable a bound.
func envInt(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		log.Printf("[WARN] [server] [config] invalid_env key=%s value=%q falling_back=%d", key, v, def)
		return def
	}
	return n
}

// resolveDBPoolShedConfig reads REDFLAG_DB_SHED_UTILIZATION and
// REDFLAG_DB_SHED_RETRY_AFTER_SECONDS from env and returns a DBPoolShedConfig.
// Invalid or out-of-range values fall back to the middleware defaults (1.0 / 2s).
// Config is resolved once at startup — it is static operational tuning, not per-request.
func resolveDBPoolShedConfig() middleware.DBPoolShedConfig {
	threshold := envFloat("REDFLAG_DB_SHED_UTILIZATION", 1.0)
	retryAfter := envInt("REDFLAG_DB_SHED_RETRY_AFTER_SECONDS", 2)
	log.Printf("[INFO] [server] [db_shed] config_resolved utilization_threshold=%.2f retry_after_seconds=%d", threshold, retryAfter)
	return middleware.DBPoolShedConfig{
		UtilizationThreshold: threshold,
		RetryAfterSeconds:    retryAfter,
	}
}

// envFloat reads a float64 in the (0, 1] range from env, falling back to def when
// unset, empty, unparseable, or out of range. The range guard is specific to the
// shed-utilization fraction so a stray value can't silently disable or invert the
// threshold. The shed middleware independently clamps as defense in depth.
func envFloat(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil || f <= 0 || f > 1 {
		log.Printf("[WARNING] [server] [config] invalid_env key=%s value=%q falling_back=%.2f", key, v, def)
		return def
	}
	return f
}

// backfillOSVChecks queries current_package_state for packages that have never
// had a supply-chain check and enqueues OSV.dev queries for each unique
// (package_type, package_name, version). Runs once at startup, async, best-effort.
func backfillOSVChecks(updateQueries *queries.UpdateQueries) {
	log.Printf("[INFO] [server] [supply_chain] backfill_start")
	rows, err := updateQueries.GetUncheckedPackages()
	if err != nil {
		log.Printf("[WARNING] [server] [supply_chain] backfill_query_failed error=%v", err)
		return
	}

	var reqs []services.OSVCheckRequest
	for _, r := range rows {
		if !services.NeedsSupplyChainCheck(r.PackageType) {
			continue
		}
		reqs = append(reqs, services.OSVCheckRequest{
			AgentID: r.AgentID,
			PkgType: r.PackageType,
			PkgName: r.PackageName,
			Version: r.Version,
		})
	}
	log.Printf("[INFO] [server] [supply_chain] backfill_candidates count=%d", len(reqs))

	services.RunOSVChecks(reqs, func(agentID uuid.UUID, pkgType, pkgName string, meta map[string]interface{}) error {
		return updateQueries.StoreSupplyChainMetadata(agentID, pkgType, pkgName, models.JSONB(meta))
	})
	log.Printf("[INFO] [server] [supply_chain] backfill_complete count=%d", len(reqs))
}
