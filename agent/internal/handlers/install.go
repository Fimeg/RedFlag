package handlers

import (
	"fmt"
	"log"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/acknowledgment"
	"github.com/Fimeg/RedFlag/agent/internal/cache"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/event"
	"github.com/Fimeg/RedFlag/agent/internal/installer"
	"github.com/Fimeg/RedFlag/agent/internal/models"
	"github.com/gofrs/uuid/v5"
)

// emitInstallEvent records a package-install outcome on the system-event
// channel so it reaches the History page (ETHOS #1: errors are history).
// It routes inward through the operational event buffer, which persists to disk
// and flushes to the server independently of the command-result path. A
// failed install is auditable even if the result report is lost.
func emitInstallEvent(buffer *event.Buffer, agentID uuid.UUID, subtype, severity, packageType, packageName, commandID, message string) {
	event.BufferSystemEvent(buffer, agentID,
		models.EventTypeAgentInstall, subtype, severity,
		packageType+"_installer", message,
		map[string]interface{}{
			"package_type": packageType,
			"package_name": packageName,
			"command_id":   commandID,
		},
	)
}

// ExpectedHashes maps package names to their expected SHA256 hashes
func ExpectedHashes(cfg *config.Config) map[string]string {
	hashes := make(map[string]string)
	if cfg.PackageHashes != nil {
		for name, hash := range cfg.PackageHashes {
			hashes[name] = hash
		}
	}
	return hashes
}

// FetchExpectedHash retrieves the expected SHA256 hash for a package from the server
func FetchExpectedHash(apiClient *client.Client, cfg *config.Config, packageName, version, packageType string) (string, error) {
	return apiClient.GetExpectedHash(packageType, packageName, cfg.AgentID)
}

func HandleInstallUpdates(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, eventBuffer *event.Buffer, params map[string]interface{}, commandID string) (err error) {
	packageType, _ := params["package_type"].(string)
	packageName, _ := params["package_name"].(string)

	// Any error return below lands on the History channel as a failed install
	// event — one inward funnel covering every failure path (factory, hash
	// verification, the install itself).
	defer func() {
		if err != nil {
			emitInstallEvent(eventBuffer, cfg.AgentID, models.SubtypeFailed, models.SeverityError, packageType, packageName, commandID, err.Error())
		}
	}()

	if packageType == "" {
		return fmt.Errorf("package_type parameter is required")
	}

	inst, err := installer.InstallerFactory(packageType, cfg.ServerURL)
	if err != nil {
		return fmt.Errorf("[ERROR] [agent] [installer] factory_failed type=%s error=%w", packageType, err)
	}

	if !inst.IsAvailable() {
		return fmt.Errorf("[ERROR] [agent] [installer] not_available type=%s", packageType)
	}

	// Layer 1: Hash Registry — verify package hash before installation
	// Fetch the expected hash from the server
	expectedHash, err := FetchExpectedHash(apiClient, cfg, packageName, "", packageType)
	if err != nil {
		return fmt.Errorf("[ERROR] [agent] [installer] hash_fetch_failed package=%s error=%w", packageName, err)
	}

	if expectedHash != "" {
		log.Printf("[INFO] [agent] [installer] verifying_hash package=%s expected_sha256=%s", packageName, expectedHash[:16]+"...")
		if err := inst.VerifyHash(packageName, "", expectedHash); err != nil {
			return fmt.Errorf("[ERROR] [agent] [installer] hash_verification_failed package=%s error=%w", packageName, err)
		}
		log.Printf("[INFO] [agent] [installer] hash_verified package=%s", packageName)
	}

	var result *installer.InstallResult
	var action string

	startTime := time.Now()

	// NEW: Check cache first for expected hash
	hashCache := cache.NewHashCache(100) // 100 entries max
	if cachedHash, cached := hashCache.Get(packageType, packageName, ""); cached {
		log.Printf("[INFO] [agent] [installer] hash_cached package=%s hash=%s", packageName, cachedHash[:16]+"...")
	}

	// Gated ecosystems (dnf, apt) — mutation is through capability-token path only.
	if packageType == "dnf" || packageType == "apt" {
		return fmt.Errorf("[SECURITY] [agent] [installer] direct_mutation_refused type=%s — mutation must go through capability-token path", packageType)
	}

	// Non-gated ecosystems mutate directly. dnf/apt are excluded above and
	// do not implement NonGatedInstaller, so the assertion fails for them.
	mut, ok := inst.(installer.NonGatedInstaller)
	if !ok {
		return fmt.Errorf("[ERROR] [agent] [installer] direct_mutation_not_supported type=%s", packageType)
	}

	if packageName != "" {
		action = "update"
		log.Printf("[INFO] [agent] [installer] updating_package package=%s type=%s", packageName, packageType)
		result, err = mut.UpdatePackage(packageName)
	} else {
		action = "upgrade"
		log.Printf("[INFO] [agent] [installer] upgrading_all type=%s", packageType)
		result, err = mut.Upgrade()
	}

	duration := int(time.Since(startTime).Seconds())

	if err != nil {
		stdout := ""
		stderr := err.Error()
		exitCode := 1
		if result != nil {
			stdout = result.Stdout
			stderr = result.Stderr
			exitCode = result.ExitCode
		}
		logReport := client.LogReport{
			CommandID:       commandID,
			Action:          action,
			Result:          "failed",
			Stdout:          stdout,
			Stderr:          stderr,
			ExitCode:        exitCode,
			DurationSeconds: duration,
			Metadata: map[string]string{
				"subsystem_label": "Package Install",
				"subsystem":       packageType,
			},
		}
		ReportLogWithAck(apiClient, cfg, ackTracker, logReport)
		return fmt.Errorf("[ERROR] [agent] [installer] install_failed action=%s type=%s error=%w", action, packageType, err)
	}

	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          result.Action,
		Result:          "success",
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		ExitCode:        result.ExitCode,
		DurationSeconds: result.DurationSeconds,
		Metadata: map[string]string{
			"subsystem_label": "Package Install",
			"subsystem":       packageType,
		},
	}

	if len(result.PackagesInstalled) > 0 {
		logReport.Stdout += fmt.Sprintf("\nPackages installed: %v", result.PackagesInstalled)
	}

	if reportErr := ReportLogWithAck(apiClient, cfg, ackTracker, logReport); reportErr != nil {
		log.Printf("[WARNING] [agent] [installer] report_failed error=%v", reportErr)
	}

	emitInstallEvent(eventBuffer, cfg.AgentID, models.SubtypeSuccess, models.SeverityInfo, packageType, packageName, commandID,
		fmt.Sprintf("%s of %s completed in %ds", result.Action, packageName, duration))

	log.Printf("[INFO] [agent] [installer] install_complete action=%s type=%s duration=%ds", action, packageType, duration)
	return nil
}
