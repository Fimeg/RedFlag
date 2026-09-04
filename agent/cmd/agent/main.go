package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"runtime/debug"

	"github.com/Fimeg/RedFlag/agent/internal/agent"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/constants"
	"github.com/Fimeg/RedFlag/agent/internal/handlers"
	"github.com/Fimeg/RedFlag/agent/internal/instancelock"
	agentLogging "github.com/Fimeg/RedFlag/agent/internal/logging"
	"github.com/Fimeg/RedFlag/agent/internal/migration"
	"github.com/Fimeg/RedFlag/agent/internal/registration"
	"github.com/Fimeg/RedFlag/agent/internal/service"
	"github.com/Fimeg/RedFlag/agent/internal/version"
)

func main() {
	if err := agentLogging.ConfigureProcessLogger(); err != nil {
		log.Printf("[ERROR] [agent] [logging] process_logger_init_failed error=%v", err)
	}

	// Panic recovery - prevents agent crashes from unhandled panics
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[CRITICAL] Agent panic recovered: %v", r)
			log.Printf("[CRITICAL] Stack trace: %s", debug.Stack())
			os.Exit(1)
		}
	}()

	// Parse CLI flags
	cli := ParseFlags()

	// Handle version command
	if cli.Version {
		HandleVersionCommand()
	}

	if cli.LocalStatus {
		if err := HandleLocalStatusCommand(cli.ExportFormat); err != nil {
			log.Fatal("Local status command failed:", err)
		}
		return
	}

	// Handle Windows service management commands
	if HandleWindowsServiceCommands(cli) {
		return
	}

	// Determine config path
	configPath := constants.GetAgentConfigPath()
	if cli.ConfigFile != "" {
		configPath = cli.ConfigFile
	}

	// Check for migration requirements
	if err := handleMigration(configPath); err != nil {
		log.Printf("Warning: Migration handling failed: %v", err)
	}

	// Load configuration with priority: CLI > env > file > defaults
	cfg, err := config.Load(configPath, cli.ToConfigFlags())
	if err != nil {
		log.Fatal("Failed to load configuration:", err)
	}

	// Update agent version in config if changed
	if cfg.AgentVersion != version.Version {
		cfg.AgentVersion = version.Version
		if err := cfg.Save(configPath); err != nil {
			log.Printf("Warning: Failed to update agent version in config: %v", err)
		}
	}

	if cli.InitStandalone {
		if err := cfg.InitializeStandalone(); err != nil {
			log.Fatal("Standalone initialization failed: ", err)
		}
		if err := cfg.Save(configPath); err != nil {
			log.Fatal("Standalone configuration save failed: ", err)
		}
		fmt.Println(cfg.AgentID.String())
		return
	}

	// Handle registration command
	if cli.Register {
		if cfg.IsStandalone() {
			log.Fatal("Registration refused: standalone fleet join is not implemented; do not add fleet credentials beside local authority")
		}
		if err := handleRegistration(cfg, cli.ServerURL); err != nil {
			log.Fatal("Registration failed:", err)
		}
		return
	}

	// Handle scan command
	if cli.Scan {
		if err := handlers.ScanCommand(cfg, cli.ExportFormat); err != nil {
			log.Fatal("Scan failed:", err)
		}
		return
	}

	// Handle status command
	if cli.Status {
		if err := handlers.StatusCommand(cfg); err != nil {
			log.Fatal("Status command failed:", err)
		}
		return
	}

	// Handle list-updates command
	if cli.ListUpdates {
		if err := handlers.ListUpdatesCommand(cfg, cli.ExportFormat); err != nil {
			log.Fatal("List updates failed:", err)
		}
		return
	}

	// Acquire an exclusive instance lock to prevent two agent processes
	// from sharing the same config.json and renewal state. The lock is
	// released when this process exits (fd closes on os.Exit too).
	unlock, err := instancelock.Acquire()
	if err != nil {
		log.Fatalf("[FATAL] instance_lock_failed another_instance_running error=%v", err)
	}
	defer unlock()

	// Check if registered
	if !cfg.IsRegistered() && !cfg.IsStandalone() {
		log.Fatal("Agent has no complete identity. Register with a fleet or run the standalone provisioning script.")
	}

	// Check if running as Windows service
	if runtime.GOOS == "windows" && service.IsService() {
		if err := service.RunService(cfg); err != nil {
			log.Fatal("Service failed:", err)
		}
		return
	}

	// Start agent service (console mode)
	if err := agent.RunAgentLoop(cfg); err != nil {
		log.Fatal("Agent failed:", err)
	}
}

// handleMigration checks and executes migrations if needed
func handleMigration(configPath string) error {
	migrationConfig := migration.NewFileDetectionConfig()
	migrationConfig.OldConfigPath = constants.LegacyConfigPath
	migrationConfig.OldStatePath = constants.LegacyStatePath
	migrationConfig.NewConfigPath = constants.GetAgentConfigDir()
	migrationConfig.NewStatePath = constants.GetAgentStateDir()

	migrationDetection, err := migration.DetectMigrationRequirements(migrationConfig)
	if err != nil {
		return fmt.Errorf("failed to detect migration requirements: %w", err)
	}

	if !migrationDetection.RequiresMigration {
		return nil
	}

	log.Printf("[RedFlag Server Migrator] Migration detected: %s → %s",
		migrationDetection.CurrentAgentVersion, version.Version)
	log.Printf("[RedFlag Server Migrator] Required migrations: %v",
		migrationDetection.RequiredMigrations)

	migrationPlan := &migration.MigrationPlan{
		Detection:     migrationDetection,
		TargetVersion: version.Version,
		Config:        migrationConfig,
		BackupPath:    constants.GetMigrationBackupDir(),
	}

	executor := migration.NewMigrationExecutor(migrationPlan, configPath)
	result, err := executor.ExecuteMigration()
	if err != nil {
		log.Printf("[RedFlag Server Migrator] Migration failed: %v", err)
		log.Printf("[RedFlag Server Migrator] Backup available at: %s", result.BackupPath)
		return err
	}

	log.Printf("[RedFlag Server Migrator] Migration completed successfully")
	if result.RollbackAvailable {
		log.Printf("[RedFlag Server Migrator] Rollback available at: %s", result.BackupPath)
	}

	return nil
}

// handleRegistration handles the agent registration flow
func handleRegistration(cfg *config.Config, serverURL string) error {
	// Validate server URL for Windows users
	if runtime.GOOS == "windows" && serverURL == "" {
		fmt.Println("❌ CONFIGURATION REQUIRED!")
		fmt.Println("==================================================================")
		fmt.Println("Please configure the server URL before registering:")
		fmt.Println("")
		fmt.Println("Option 1 - Use the -server flag:")
		fmt.Println("   redflag-agent.exe -register -server https://your-server.com")
		fmt.Println("")
		fmt.Println("Option 2 - Use environment variable:")
		fmt.Println("   set REDFLAG_SERVER_URL=https://your-server.com")
		fmt.Println("   redflag-agent.exe -register")
		fmt.Println("")
		fmt.Println("Option 3 - Create a .env file:")
		fmt.Println("   REDFLAG_SERVER_URL=https://your-server.com")
		fmt.Println("==================================================================")
		os.Exit(1)
	}

	// Use registration package for the actual registration
	if err := registration.RegisterAgent(cfg, serverURL); err != nil {
		return err
	}

	fmt.Println("==================================================================")
	fmt.Println("🎉 AGENT REGISTRATION SUCCESSFUL!")
	fmt.Println("==================================================================")
	fmt.Printf("📋 Agent ID: %s\n", cfg.AgentID)
	fmt.Printf("🌐 Server: %s\n", cfg.ServerURL)
	fmt.Printf("⏱️  Check-in Interval: %ds\n", cfg.CheckInInterval)
	fmt.Println("==================================================================")
	fmt.Println("💡 Save this Agent ID for your records!")
	fmt.Println("🚀 You can now start the agent without flags")
	fmt.Println("")

	return nil
}
