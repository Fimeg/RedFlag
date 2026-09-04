package handlers

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/constants"
)

// RunPostUpgradeHealthcheck checks the agent's environment against what this
// version expects. Called once at startup after RunUpgradeAttestation, before
// the polling loop. All findings are warnings — the agent continues regardless.
// Findings are logged and a non-zero return indicates gaps found.
func RunPostUpgradeHealthcheck(cfg *config.Config) int {
	gaps := 0

	// --- Config key completeness ---
	// The install template's default JSON is the source of truth for freshly
	// installed agents. Upgraded agents carry over their old config verbatim,
	// so new keys added this release may be absent. The agent's merge logic
	// (loadFromFile → getDefaultConfig chain) fills zero values on access, but
	// visible gaps help the operator understand why defaults are shipping.
	check := func(present bool, format string, args ...interface{}) {
		if !present {
			log.Printf("[WARN] [agent] [healthcheck] "+format, args...)
			gaps++
		}
	}

	// Top-level keys the template should have set. The agent.Config struct
	// with json tags is the canonical list — these are the non-omitempty keys
	// the template writes for fresh installs. We check the config struct
	// was populated (not zero-valued per the template JSON) and not missing.
	check(cfg.ServerURL != "", "config key 'server_url' is empty — should be set by installer")
	check(cfg.CheckInInterval > 0, "config key 'check_in_interval' is zero — should be 300 from template")

	// PollingConfig: this version added these keys; warn if empty.
	check(cfg.Polling.JitterMaxSeconds > 0 || cfg.Polling.BackoffBaseSeconds > 0,
		"config key 'polling' missing — polling resilience tuning will use defaults")
	// CommandSigning: recommended on.
	check(cfg.CommandSigning.Enabled,
		"config key 'command_signing.enabled' is false — recommend true")

	// DesktopConfig: warn about missing keys if enabled.
	if cfg.Desktop.Enabled {
		check(cfg.Desktop.MaxRestarts > 0,
			"config key 'desktop.max_restarts' is zero — agent will not auto-restart the desktop app on crash")
	}

	// --- Binary presence ---
	// The helper binary (redflag-helper) should exist when command signing is on.
	if cfg.CommandSigning.Enabled {
		helperPath := filepath.Join(filepath.Dir(os.Args[0]), "redflag-helper")
		if runtime.GOOS == "windows" {
			helperPath += ".exe"
		}
		check(fileExists(helperPath),
			"helper binary not found at %s — capability-gated installs will fail", helperPath)
	}

	// Desktop binary should exist when desktop is enabled.
	if cfg.Desktop.Enabled {
		desktopPath := filepath.Join(filepath.Dir(os.Args[0]), "redflag-desktop")
		if runtime.GOOS == "windows" {
			desktopPath += ".exe"
		}
		check(fileExists(desktopPath),
			"desktop binary not found at %s — local operations console will not appear", desktopPath)
	}

	// --- Autostart entry (Linux only) ---
	if cfg.Desktop.Enabled && runtime.GOOS == "linux" {
		autostartPath := "/etc/xdg/autostart/redflag-desktop.desktop"
		check(fileExists(autostartPath),
			"desktop autostart entry not found at %s — RedFlag Desktop will not start on next login", autostartPath)
	}

	// --- Socket directory permissions ---
	// The local API socket lives under GetAgentStateDir()'s parent with 0710.
	socketDir := filepath.Join(constants.GetAgentStateDir(), "..", "localapi")
	resolved, _ := filepath.EvalSymlinks(socketDir)
	if resolved == "" {
		resolved = socketDir
	}
	if info, err := os.Stat(resolved); err == nil {
		perm := info.Mode().Perm()
		check(perm&0o010 != 0,
			"localapi socket dir %s has permissions %#o — the Desktop user cannot traverse to the socket (needs 0o10 group execute)", resolved, perm)
	} else {
		log.Printf("[WARN] [agent] [healthcheck] localapi socket dir %s not found — Desktop may not work: %v", resolved, err)
		gaps++
	}

	if gaps > 0 {
		log.Printf("[WARN] [agent] [healthcheck] complete gaps=%d — re-run the install script or correct manually", gaps)
	} else {
		log.Printf("[INFO] [agent] [healthcheck] all_ok")
	}
	return gaps
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// ReportHealthcheckGapsAsEvents sends system event entries for each gap found.
// Called when gaps > 0 after RunPostUpgradeHealthcheck.
// TODO: wire into system event logger once that's available from the handler context.
func ReportHealthcheckGapsAsEvents(gaps int, cfg *config.Config) {
	// Placeholder — gaps are logged above.
	// A future iteration will emit proper system events via the agent's
	// security logger or the server's event logging endpoint.
	_ = gaps
	_ = cfg
}
