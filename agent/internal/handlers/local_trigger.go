package handlers

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"

	"github.com/Fimeg/RedFlag/agent/internal/acknowledgment"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/orchestrator"
	"github.com/Fimeg/RedFlag/agent/internal/scanner"
)

// HandleLocalTriggeredScan runs the full package-update scan on behalf of the
// local API write path (FEAT-002). There is no signed server command behind
// it, so no command ID exists and nothing is ack-tracked.
//
// Registered agents run the same HandleScanUpdates path a server scan command
// uses — results are reported so the server ingests and reconciles them
// (RECONCILE-001 close-by-absence included). Unregistered (standalone) agents
// run the scanners through the orchestrator and record the local read model
// only; no doomed HTTP calls are attempted.
func HandleLocalTriggeredScan(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, orch *orchestrator.Orchestrator, source string) error {
	log.Printf("[INFO] [agent] [localapi] local_scan_started source=%s registered=%v", source, cfg.IsRegistered())

	if cfg.IsRegistered() {
		return HandleScanUpdates(apiClient, cfg, ackTracker, orch, "")
	}
	return runStandaloneUpdateScan(cfg, orch)
}

// runStandaloneUpdateScan mirrors HandleScanUpdates's scanner set (the virtual
// "updates" subsystem) without any server reporting: orchestrator-managed
// scans (circuit breakers, timeouts) feeding the local read model.
func runStandaloneUpdateScan(cfg *config.Config, orch *orchestrator.Orchestrator) error {
	ctx := context.Background()
	var results []orchestrator.ScanResult
	var errs []string

	type updateScanner struct {
		name      string
		available func() bool
	}
	var candidates []updateScanner
	switch runtime.GOOS {
	case "linux":
		candidates = []updateScanner{
			{"apt", scanner.NewAPTScanner().IsAvailable},
			{"dnf", scanner.NewDNFScanner().IsAvailable},
			{"pacman", scanner.NewPacmanScanner().IsAvailable},
		}
	case "windows":
		candidates = []updateScanner{
			{"windows", scanner.NewWindowsUpdateScanner().IsAvailable},
			{"winget", scanner.NewWingetScanner().IsAvailable},
		}
	}

	ran := 0
	for _, cand := range candidates {
		if !cand.available() {
			continue
		}
		ran++
		result, err := orch.ScanSingle(ctx, cand.name)
		results = append(results, result)
		if err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", cand.name, err))
		}
	}

	recordLocalScanResults(cfg, results, true)

	if len(errs) > 0 {
		err := fmt.Errorf("standalone scan errors: %s", strings.Join(errs, "; "))
		log.Printf("[ERROR] [agent] [localapi] standalone_scan_failed scanners_run=%d error=%v", ran, err)
		return err
	}
	log.Printf("[INFO] [agent] [localapi] standalone_scan_completed scanners_run=%d", ran)
	return nil
}
