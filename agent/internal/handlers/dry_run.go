package handlers

import (
	"fmt"
	"log"

	"github.com/Fimeg/RedFlag/agent/internal/acknowledgment"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/event"
	"github.com/Fimeg/RedFlag/agent/internal/installer"
	"github.com/Fimeg/RedFlag/agent/internal/models"
)

// HandleDryRunUpdate runs a package-manager dry-run for the named update so
// dependency resolution can be surfaced to the dashboard operator before the
// real install fires. Reports dependencies via the dedicated endpoint and the
// command result via the standard log path; the install itself does not happen
// here — the operator's "Confirm" produces a separate confirm_dependencies
// command (see HandleConfirmDependencies).
func HandleDryRunUpdate(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, params map[string]interface{}, commandID string) error {
	packageType, _ := params["package_type"].(string)
	packageName, _ := params["package_name"].(string)
	updateID, _ := params["update_id"].(string)
	availableVersion, _ := params["available_version"].(string)
	targetVersion, _ := params["target_version"].(string)
	if targetVersion == "" {
		targetVersion = availableVersion
	}

	if packageType == "" || packageName == "" {
		return fmt.Errorf("package_type and package_name parameters are required")
	}

	inst, err := installer.InstallerFactory(packageType, cfg.ServerURL)
	if err != nil {
		return fmt.Errorf("[ERROR] [agent] [installer] factory_failed type=%s error=%w", packageType, err)
	}

	if !inst.IsAvailable() {
		return fmt.Errorf("[ERROR] [agent] [installer] not_available type=%s", packageType)
	}

	log.Printf("[INFO] [agent] [installer] dry_run_start package=%s version=%s type=%s", packageName, targetVersion, packageType)

	result, err := inst.DryRun(packageName, targetVersion)
	if err != nil {
		stdout, stderr, exitCode, duration := "", err.Error(), 1, 0
		if result != nil {
			stdout, stderr, exitCode, duration = result.Stdout, result.Stderr, result.ExitCode, result.DurationSeconds
		}
		logReport := client.LogReport{
			CommandID:       commandID,
			Action:          "dry_run",
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
		return fmt.Errorf("dry run failed: %w", err)
	}

	// Resolve the canonical artifact hashes for the closure (top-level package +
	// dependencies) from the agent's signed repo metadata. This is the registry's
	// hash source for OS package managers — the server cannot reach the agent's
	// repos. Best-effort per package: a resolution failure is logged and that
	// entry is omitted, never guessed. The server mints only over what resolved.
	closure := resolveClosureHashes(packageType, packageName, targetVersion, result.Dependencies)

	// Mirror the result into the client's wire type and post dependencies.
	depReport := client.DependencyReport{
		PackageName:   packageName,
		PackageType:   packageType,
		TargetVersion: targetVersion,
		Dependencies:  result.Dependencies,
		UpdateID:      updateID,
		DryRunResult: &client.InstallResult{
			Success:           result.Success,
			ErrorMessage:      result.ErrorMessage,
			Stdout:            result.Stdout,
			Stderr:            result.Stderr,
			ExitCode:          result.ExitCode,
			DurationSeconds:   result.DurationSeconds,
			Action:            result.Action,
			PackagesInstalled: result.PackagesInstalled,
			ContainersUpdated: result.ContainersUpdated,
			Dependencies:      result.Dependencies,
			IsDryRun:          true,
		},
		Closure: closure,
	}

	if err := apiClient.ReportDependencies(cfg.AgentID, depReport); err != nil {
		log.Printf("[ERROR] [agent] [installer] report_dependencies_failed error=%v", err)
		return fmt.Errorf("failed to report dependencies: %w", err)
	}

	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "dry_run",
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
	if len(result.Dependencies) > 0 {
		logReport.Stdout += fmt.Sprintf("\nDependencies found: %v", result.Dependencies)
	}

	if reportErr := ReportLogWithAck(apiClient, cfg, ackTracker, logReport); reportErr != nil {
		log.Printf("[WARNING] [agent] [installer] report_dry_run_failed error=%v", reportErr)
	}

	log.Printf("[INFO] [agent] [installer] dry_run_complete package=%s deps=%d duration=%ds",
		packageName, len(result.Dependencies), result.DurationSeconds)
	return nil
}

// resolveClosureHashes resolves the canonical SHA256 for the top-level package
// and each dependency via the agent's signed repo metadata. The top-level entry
// is required: if it cannot be resolved the closure is dropped entirely (no
// partial pin). Dependencies are best-effort and logged on failure. Returns nil
// for package types whose hash is sourced server-side (npm/PyPI) or unsupported.
func resolveClosureHashes(packageType, packageName, targetVersion string, dependencies []string) []client.ClosureItem {
	top, err := installer.ResolveArtifactSHA256(packageType, packageName, targetVersion)
	if err != nil {
		log.Printf("[WARNING] [agent] [installer] closure_toplevel_unresolved pkg=%s type=%s error=%v",
			packageName, packageType, err)
		return nil
	}

	closure := []client.ClosureItem{{
		Name:    top.Name,
		Version: top.Version,
		SHA256:  top.SHA256,
		Source:  "registry",
	}}

	for _, dep := range dependencies {
		if dep == "" || dep == packageName {
			continue
		}
		resolved, err := installer.ResolveArtifactSHA256(packageType, dep, "")
		if err != nil {
			log.Printf("[WARNING] [agent] [installer] closure_dependency_unresolved dep=%s type=%s error=%v",
				dep, packageType, err)
			continue
		}
		closure = append(closure, client.ClosureItem{
			Name:    resolved.Name,
			Version: resolved.Version,
			SHA256:  resolved.SHA256,
			Source:  "registry",
		})
	}

	log.Printf("[INFO] [agent] [installer] closure_resolved pkg=%s type=%s entries=%d",
		packageName, packageType, len(closure))
	return closure
}

// HandleConfirmDependencies installs a package together with the dependencies
// the operator confirmed during the dry-run review. Empty dependency list is
// allowed — that path resolves to a simple UpdatePackage on the named target.
func HandleConfirmDependencies(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, eventBuffer *event.Buffer, params map[string]interface{}, commandID string) (err error) {
	packageType, _ := params["package_type"].(string)
	packageName, _ := params["package_name"].(string)

	// Every error return below surfaces on the History channel as a failed
	// install event (ETHOS #1), inward via the operational event buffer.
	defer func() {
		if err != nil {
			emitInstallEvent(eventBuffer, cfg.AgentID, models.SubtypeFailed, models.SeverityError, packageType, packageName, commandID, err.Error())
		}
	}()

	if packageType == "" || packageName == "" {
		return fmt.Errorf("package_type and package_name parameters are required")
	}

	// Gated ecosystems — confirmation goes through capability-token path.
	if packageType == "dnf" || packageType == "apt" {
		return fmt.Errorf("[SECURITY] [agent] [installer] direct_confirm_refused type=%s — confirmation must go through capability-token path", packageType)
	}

	var dependencies []string
	if deps, ok := params["dependencies"].([]interface{}); ok {
		for _, dep := range deps {
			if s, ok := dep.(string); ok && s != "" {
				dependencies = append(dependencies, s)
			}
		}
	}

	inst, err := installer.InstallerFactory(packageType, cfg.ServerURL)
	if err != nil {
		return fmt.Errorf("[ERROR] [agent] [installer] factory_failed type=%s error=%w", packageType, err)
	}

	if !inst.IsAvailable() {
		return fmt.Errorf("[ERROR] [agent] [installer] not_available type=%s", packageType)
	}

	var result *installer.InstallResult
	var action string

	// Non-gated ecosystems mutate directly. dnf/apt do not implement
	// NonGatedInstaller, so the assertion fails for them — the gate boundary
	// is structural here, not a packageType guard.
	mut, ok := inst.(installer.NonGatedInstaller)
	if !ok {
		return fmt.Errorf("[ERROR] [agent] [installer] direct_confirm_not_supported type=%s", packageType)
	}

	if len(dependencies) > 0 {
		action = "install_with_dependencies"
		log.Printf("[INFO] [agent] [installer] install_with_deps package=%s deps=%v", packageName, dependencies)
		allPackages := append([]string{packageName}, dependencies...)
		result, err = mut.InstallMultiple(allPackages)
	} else {
		action = "update"
		log.Printf("[INFO] [agent] [installer] update_package package=%s", packageName)
		result, err = mut.UpdatePackage(packageName)
	}

	if err != nil {
		stdout, stderr, exitCode, duration := "", err.Error(), 1, 0
		if result != nil {
			stdout, stderr, exitCode, duration = result.Stdout, result.Stderr, result.ExitCode, result.DurationSeconds
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
		return fmt.Errorf("installation failed: %w", err)
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
	if len(dependencies) > 0 {
		logReport.Stdout += fmt.Sprintf("\nDependencies included: %v", dependencies)
	}

	if reportErr := ReportLogWithAck(apiClient, cfg, ackTracker, logReport); reportErr != nil {
		log.Printf("[WARNING] [agent] [installer] report_install_failed error=%v", reportErr)
	}

	emitInstallEvent(eventBuffer, cfg.AgentID, models.SubtypeSuccess, models.SeverityInfo, packageType, packageName, commandID,
		fmt.Sprintf("%s of %s completed in %ds", action, packageName, result.DurationSeconds))

	log.Printf("[INFO] [agent] [installer] install_complete action=%s package=%s duration=%ds",
		action, packageName, result.DurationSeconds)
	return nil
}
