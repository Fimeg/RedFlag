package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/acknowledgment"
	"github.com/Fimeg/RedFlag/agent/internal/cache"
	"github.com/Fimeg/RedFlag/agent/internal/circuitbreaker"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/constants"
	"github.com/Fimeg/RedFlag/agent/internal/crypto"
	"github.com/Fimeg/RedFlag/agent/internal/desktop"
	"github.com/Fimeg/RedFlag/agent/internal/event"
	"github.com/Fimeg/RedFlag/agent/internal/handlers"
	"github.com/Fimeg/RedFlag/agent/internal/integrations"
	"github.com/Fimeg/RedFlag/agent/internal/kernel"
	"github.com/Fimeg/RedFlag/agent/internal/localapi"
	"github.com/Fimeg/RedFlag/agent/internal/logging"
	"github.com/Fimeg/RedFlag/agent/internal/models"
	"github.com/Fimeg/RedFlag/agent/internal/orchestrator"
	"github.com/Fimeg/RedFlag/agent/internal/receipt"
	"github.com/Fimeg/RedFlag/agent/internal/recovery"
	"github.com/Fimeg/RedFlag/agent/internal/scanner"
	"github.com/Fimeg/RedFlag/agent/internal/startup"
	"github.com/Fimeg/RedFlag/agent/internal/supplychain"
	"github.com/Fimeg/RedFlag/agent/internal/system"
	"github.com/Fimeg/RedFlag/agent/internal/version"
	"github.com/gofrs/uuid/v5"
)

// newCircuitBreaker creates a circuit breaker from config
func newCircuitBreaker(name string, cfg config.CircuitBreakerConfig) *circuitbreaker.CircuitBreaker {
	return circuitbreaker.New(name, circuitbreaker.Config{
		FailureThreshold: cfg.FailureThreshold,
		FailureWindow:    cfg.FailureWindow,
		OpenDuration:     cfg.OpenDuration,
		HalfOpenAttempts: cfg.HalfOpenAttempts,
	})
}

// RunAgentLoop runs the main agent polling loop
func RunAgentLoop(cfg *config.Config) error {
	// Panic recovery for the main agent loop [TD-002]
	defer recovery.Recover("agent_main_loop")

	log.Printf("RedFlag Agent v%s starting...", version.Version)
	mode := "fleet"
	if cfg.IsStandalone() {
		mode = "standalone"
	}
	log.Printf("Agent ID: %s  Mode: %s  Server: %s  Interval: %ds",
		cfg.AgentID, mode, cfg.ServerURL, cfg.CheckInInterval)

	loopCtx, err := NewLoopContext(cfg, LoopContextOptions{
		Ctx:           context.Background(),
		EnableDesktop: true,
	})
	if err != nil {
		return err
	}

	// Post-upgrade attestation and healthcheck run after the canonical context
	// exists so any warning path can use the operational event channel.
	if cfg.IsRegistered() {
		handlers.RunUpgradeAttestation(loopCtx.APIClient, cfg, loopCtx.AckTracker)
	}
	if gaps := handlers.RunPostUpgradeHealthcheck(cfg); gaps > 0 {
		log.Printf("[INFO] [agent] [healthcheck] gaps=%d — upgrade may be incomplete; re-run install script to reconcile", gaps)
	}

	return RunPollingLoop(loopCtx)
}

// LoopContextOptions controls platform-specific loop setup around the shared
// dependency graph. Windows service mode supplies StopCh; console mode enables
// the desktop manager.
type LoopContextOptions struct {
	Ctx           context.Context
	StopCh        <-chan struct{}
	EnableDesktop bool
}

// NewLoopContext builds the canonical dependency graph for the agent polling
// loop. Both console mode and Windows service mode use this so operational
// event plumbing cannot silently diverge at the construction boundary.
func NewLoopContext(cfg *config.Config, opts LoopContextOptions) (*LoopContext, error) {
	if opts.Ctx == nil {
		opts.Ctx = context.Background()
	}

	apiClient := client.NewClient(cfg.ServerURL, cfg.Token)

	// Apply the locally-configured stale-key window (env/config file) before the
	// first check-in, so the public-key fetch is governed from boot. The server's
	// fleet policy overrides it on the next config refresh. crypto clamps to its
	// doctrinal ceiling regardless of the value here (SEC-028).
	crypto.SetStaleKeyMaxAge(cfg.CommandSigning.StaleKeyMaxAgeHours)

	// TeeLogger: dual-output logger that emits ETHOS-tagged log lines AND buffers
	// SystemEvents for the operator dashboard.
	teeLogger := event.NewTeeLogger(
		event.NewBuffer(filepath.Join(constants.GetAgentStateDir(), "events_buffer.json")),
		cfg.AgentID,
	)
	config.InitLogger(teeLogger)
	crypto.InitLogger(teeLogger)
	recovery.SetHandler(func(component string, err interface{}, stack []byte) {
		stackText := string(stack)
		if len(stackText) > 8192 {
			stackText = stackText[:8192]
		}
		errText := fmt.Sprint(err)
		log.Printf("[CRITICAL] [%s] panic_recovered error=%q", component, err)
		log.Printf("[CRITICAL] [%s] stack_trace=%s", component, stackText)
		teeLogger.Log(event.LogParams{
			Level:           "CRITICAL",
			System:          "agent",
			Component:       "recovery",
			EventType:       models.EventTypeError,
			EventSubtype:    models.SubtypePanicRecovered,
			Severity:        models.SeverityCritical,
			ServerComponent: "panic_recovery",
			Message:         fmt.Sprintf("Recovered panic in %s: %s", component, errText),
			Metadata: map[string]interface{}{
				"component": component,
				"error":     errText,
				"stack":     stackText,
			},
		})
	})

	startupLogger := startup.NewLogger(constants.GetAgentStateDir(), version.Version)
	if err := startupLogger.LogEvent(startup.EventTypeStartup, true, nil, map[string]interface{}{
		"agent_id": cfg.AgentID.String(),
		"server":   cfg.ServerURL,
	}); err != nil {
		teeLogger.Warning("agent", "startup", "startup", "startup_event_local_write_failed", map[string]interface{}{"error": err.Error()})
	}
	teeLogger.Log(event.LogParams{
		Level:           "INFO",
		System:          "agent",
		Component:       "startup",
		EventType:       models.EventTypeAgentStartup,
		EventSubtype:    models.SubtypeStarted,
		Severity:        models.SeverityInfo,
		ServerComponent: "startup",
		Message:         "agent startup",
		Metadata: map[string]interface{}{
			"agent_id": cfg.AgentID.String(),
			"server":   cfg.ServerURL,
			"version":  version.Version,
		},
	})

	defaults := config.GetDefaultSubsystemsConfig()
	resolve := func(sub, def config.SubsystemConfig) config.SubsystemConfig {
		if sub == (config.SubsystemConfig{}) {
			return def
		}
		return sub
	}
	aptSub := resolve(cfg.Subsystems.APT, defaults.APT)
	dnfSub := resolve(cfg.Subsystems.DNF, defaults.DNF)
	pacmanSub := resolve(cfg.Subsystems.Pacman, defaults.Pacman)
	windowsSub := resolve(cfg.Subsystems.Windows, defaults.Windows)
	wingetSub := resolve(cfg.Subsystems.Winget, defaults.Winget)
	storageSub := resolve(cfg.Subsystems.Storage, defaults.Storage)
	systemSub := resolve(cfg.Subsystems.System, defaults.System)
	dockerSub := resolve(cfg.Subsystems.Docker, defaults.Docker)

	// Initialize scanners
	aptScanner := scanner.NewAPTScanner()
	dnfScanner := scanner.NewDNFScanner()
	pacmanScanner := scanner.NewPacmanScanner()
	windowsUpdateScanner := scanner.NewWindowsUpdateScanner()
	wingetScanner := scanner.NewWingetScanner()
	storageScanner := orchestrator.NewStorageScanner(version.Version)
	systemScanner := orchestrator.NewSystemScanner(version.Version)
	dockerScanner, _ := orchestrator.NewDockerScanner()

	// Initialize circuit breakers
	aptCB := newCircuitBreaker("APT", aptSub.CircuitBreaker)
	dnfCB := newCircuitBreaker("DNF", dnfSub.CircuitBreaker)
	pacmanCB := newCircuitBreaker("Pacman", pacmanSub.CircuitBreaker)
	windowsCB := newCircuitBreaker("Windows Update", windowsSub.CircuitBreaker)
	wingetCB := newCircuitBreaker("Winget", wingetSub.CircuitBreaker)
	storageCB := newCircuitBreaker("Storage", storageSub.CircuitBreaker)
	systemCB := newCircuitBreaker("System", systemSub.CircuitBreaker)
	dockerCB := newCircuitBreaker("Docker", dockerSub.CircuitBreaker)

	// Initialize orchestrator with event buffering
	scanOrchestrator := orchestrator.NewOrchestratorWithEvents(teeLogger)

	// Register all scanners
	scanOrchestrator.RegisterScanner("apt", aptScanner, aptCB, aptSub.Timeout, aptSub.Enabled)
	scanOrchestrator.RegisterScanner("dnf", dnfScanner, dnfCB, dnfSub.Timeout, dnfSub.Enabled)
	scanOrchestrator.RegisterScanner("pacman", pacmanScanner, pacmanCB, pacmanSub.Timeout, pacmanSub.Enabled)
	scanOrchestrator.RegisterScanner("windows", windowsUpdateScanner, windowsCB, windowsSub.Timeout, windowsSub.Enabled)
	scanOrchestrator.RegisterScanner("winget", wingetScanner, wingetCB, wingetSub.Timeout, wingetSub.Enabled)
	scanOrchestrator.RegisterScanner("storage", storageScanner, storageCB, storageSub.Timeout, storageSub.Enabled)
	scanOrchestrator.RegisterScanner("system", systemScanner, systemCB, systemSub.Timeout, systemSub.Enabled)
	if dockerScanner != nil {
		scanOrchestrator.RegisterScanner("docker", dockerScanner, dockerCB, dockerSub.Timeout, dockerSub.Enabled)
	} else {
		teeLogger.Warning("agent", "docker", "docker", "docker_scanner_init_failed", nil)
	}

	// Register inventory scanners (DockerScanner implements both Scanner and InventoryScanner)
	if dockerScanner != nil {
		scanOrchestrator.RegisterInventoryScanner("docker", dockerScanner, dockerCB, dockerSub.Timeout, dockerSub.Enabled)
	}

	// Initialize acknowledgment tracker (result acks — pending_acks.json)
	ackTracker := acknowledgment.NewTracker(constants.GetAgentStateDir())
	if err := ackTracker.Load(); err != nil {
		teeLogger.Warning("agent", "acknowledgment", "acknowledgment_tracker", "load_pending_acks_failed", map[string]interface{}{"error": err.Error()})
	}

	// Initialize kernel enforcement enforcer
	kernelEnforcer, err := kernel.NewEnforcerWithLogger(cfg, teeLogger)
	if err != nil {
		teeLogger.Warning("agent", "kernel", "kernel_enforcer", "enforcer_init_failed", map[string]interface{}{"error": err.Error()})
	} else {
		log.Printf("[INFO] [agent] [kernel] %s_enforcer_started", kernelEnforcer.GetPackageType())
	}

	// Initialize receipt tracker (command-receipt confirmation — pending_receipts.json)
	// Migration 033 §2: doctrine TODO-full-command-lifecycle.md
	receiptTracker := receipt.NewTracker(constants.GetAgentStateDir())
	if err := receiptTracker.Load(); err != nil {
		teeLogger.Warning("agent", "receipt", "receipt_tracker", "load_pending_receipts_failed", map[string]interface{}{"error": err.Error()})
	}

	// Initialize confirmed tracker (command-completion confirmation from server)
	// This tracks commands the server has acknowledged via ReportLog, preventing
	// false-positive duplicate rejections when server hasn't processed the log yet.
	confirmedTracker := orchestrator.NewConfirmedTracker(constants.GetAgentStateDir())
	if err := confirmedTracker.Load(); err != nil {
		teeLogger.Warning("agent", "confirmed", "confirmed_tracker", "load_confirmed_completed_failed", map[string]interface{}{"error": err.Error()})
	}

	// Initialize command handler
	securityLogger, _ := logging.NewSecurityLogger(cfg, constants.GetAgentStateDir())
	commandHandler, err := orchestrator.NewCommandHandler(cfg, constants.GetAgentStateDir(), securityLogger, log.New(os.Stdout, "", log.LstdFlags))
	if err != nil {
		teeLogger.Critical("agent", "cmd_handler", "cmd_handler", fmt.Sprintf("init_failed error=%v", err), map[string]interface{}{"error": err.Error()})
		return nil, fmt.Errorf("failed to initialize command handler: %w", err)
	}

	// Initialize the native Desktop manager.
	var desktopMgr *desktop.Manager
	if opts.EnableDesktop {
		desktopMgr = desktop.NewManager(
			"", // auto-detect binary alongside agent
			cfg.Desktop.Enabled,
			cfg.Desktop.MaxRestarts,
			cfg.Desktop.RestartDelaySec,
		)
	}

	return &LoopContext{
		Ctx:              opts.Ctx,
		Cfg:              cfg,
		APIClient:        apiClient,
		AckTracker:       ackTracker,
		ReceiptTracker:   receiptTracker,
		ConfirmedTracker: confirmedTracker,
		CommandHandler:   commandHandler,
		ScanOrchestrator: scanOrchestrator,
		DesktopManager:   desktopMgr,
		KernelEnforcer:   kernelEnforcer,
		SecurityLogger:   securityLogger,
		TeeLogger:        teeLogger,
		EventBuffer:      teeLogger.Buffer(),
		StopCh:           opts.StopCh,
		CircuitBreakers: map[string]*circuitbreaker.CircuitBreaker{
			"apt":     aptCB,
			"dnf":     dnfCB,
			"windows": windowsCB,
			"winget":  wingetCB,
			"storage": storageCB,
			"system":  systemCB,
			"docker":  dockerCB,
		},
	}, nil
}

// LoopContext holds all dependencies for the polling loop.
// Fields are exported so the Windows service can fully construct one.
type LoopContext struct {
	Cfg              *config.Config
	APIClient        *client.Client
	AckTracker       *acknowledgment.Tracker
	ReceiptTracker   *receipt.Tracker
	ConfirmedTracker *orchestrator.ConfirmedTracker // tracks commands server confirmed as completed
	CommandHandler   *orchestrator.CommandHandler
	ScanOrchestrator *orchestrator.Orchestrator
	CircuitBreakers  map[string]*circuitbreaker.CircuitBreaker
	KernelEnforcer   kernel.Enforcer
	DesktopManager   *desktop.Manager
	SecurityLogger   *logging.SecurityLogger
	TeeLogger        *event.TeeLogger
	EventBuffer      *event.Buffer
	Ctx              context.Context
	StopCh           <-chan struct{} // non-nil causes loop to exit cleanly when closed
}

func runStandaloneLoop(ctx *LoopContext, triggerScan func(string) error) error {
	recordLocalAgentStatus(ctx.Cfg, "standalone", false)
	log.Printf("[INFO] [agent] [standalone] local_mode_started agent_id=%s", ctx.Cfg.AgentID)
	if err := triggerScan("standalone-startup"); err != nil {
		ctx.TeeLogger.Warning("agent", "standalone", "scan", "startup_scan_not_started", map[string]interface{}{"error": err.Error()})
	}

	interval := time.Duration(ctx.Cfg.CheckInInterval) * time.Second
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Ctx.Done():
			return nil
		case <-ctx.StopCh:
			return nil
		case <-ticker.C:
			if err := triggerScan("standalone-interval"); err != nil && !errors.Is(err, localapi.ErrScanInFlight) {
				ctx.TeeLogger.Warning("agent", "standalone", "scan", "periodic_scan_not_started", map[string]interface{}{"error": err.Error()})
			}
		}
	}
}

// RunPollingLoop runs the main agent polling loop.
// It is called by RunAgentLoop and may also be called by the Windows service
// with a stop channel. When stopCh is non-nil, the loop selects on it and exits cleanly.
func RunPollingLoop(loopCtx *LoopContext) error {
	// Panic recovery for the polling loop [TD-002]
	defer recovery.Recover("agent_polling_loop")

	ctx := loopCtx

	// FEAT-002 write path: single-flight scan trigger for the local API.
	// Authorization is the socket/pipe group ACL; the scan itself runs through
	// the same handler primitives a signed server scan command uses.
	var scanInFlight atomic.Bool
	triggerScan := func(source string) error {
		if !scanInFlight.CompareAndSwap(false, true) {
			return localapi.ErrScanInFlight
		}
		go func() {
			defer recovery.Recover("local_triggered_scan")
			defer scanInFlight.Store(false)
			if err := handlers.HandleLocalTriggeredScan(ctx.APIClient, ctx.Cfg, ctx.AckTracker, ctx.ScanOrchestrator, source); err != nil {
				ctx.TeeLogger.Error("agent", "localapi", "localapi", fmt.Sprintf("local_scan_failed source=%s error=%v", source, err), map[string]interface{}{"source": source, "error": err.Error()})
			}
		}()
		return nil
	}

	// FEAT-003 write path: standalone local approval. Single-flight; the
	// callback decodes the body, runs gates → mint → execute, and translates
	// typed failures into localapi sentinels for honest HTTP codes.
	var approveInFlight atomic.Bool
	approveUpdate := func(body []byte) (interface{}, error) {
		if !approveInFlight.CompareAndSwap(false, true) {
			return nil, fmt.Errorf("%w: approval already in flight", localapi.ErrApprovalConflict)
		}
		defer approveInFlight.Store(false)

		var req handlers.LocalApproveRequest
		if err := json.Unmarshal(body, &req); err != nil {
			return nil, fmt.Errorf("invalid approval request: %w", err)
		}
		result, err := handlers.HandleLocalApprove(ctx.Ctx, ctx.Cfg, req)
		switch {
		case err == nil:
			return result, nil
		case errors.Is(err, handlers.ErrApprovalFleetMode),
			errors.Is(err, handlers.ErrApprovalBlocked),
			errors.Is(err, supplychain.ErrMintGateRefused),
			errors.Is(err, supplychain.ErrMintStale),
			errors.Is(err, supplychain.ErrMintDuplicate):
			return nil, fmt.Errorf("%w: %v", localapi.ErrApprovalConflict, err)
		case errors.Is(err, supplychain.ErrMintNoAuthority):
			return nil, fmt.Errorf("%w: %v", localapi.ErrApprovalUnavailable, err)
		case errors.Is(err, handlers.ErrApprovalNoStandaloneIdentity):
			return nil, fmt.Errorf("%w: %v", localapi.ErrApprovalUnavailable, err)
		default:
			return nil, err
		}
	}

	var onDesktopHealth func(version string, windowOpen bool)
	if ctx.DesktopManager != nil {
		onDesktopHealth = ctx.DesktopManager.RecordHealth
	}
	dockerProvider := func() (*localapi.DockerResponse, error) {
		scanner, err := orchestrator.NewDockerScanner()
		if err != nil {
			return &localapi.DockerResponse{Available: false, CollectedAt: time.Now().UTC()}, nil
		}
		defer scanner.Close()
		if !scanner.IsAvailable() {
			return &localapi.DockerResponse{Available: false, CollectedAt: time.Now().UTC()}, nil
		}
		containers, err := scanner.ScanContainers()
		if err != nil {
			return nil, err
		}
		response := &localapi.DockerResponse{
			Available: true, Version: scanner.GetEngineVersion(), Containers: containers,
			Stacks: scanner.ScanStacks(containers), Count: len(containers), CollectedAt: time.Now().UTC(),
		}
		for _, container := range containers {
			if container.State == "running" {
				response.Running++
			}
			if container.Health == "unhealthy" {
				response.Unhealthy++
			}
		}
		return response, nil
	}
	localAPIServer, err := localapi.Start(localapi.Options{
		Config:          ctx.Cfg,
		DesktopProvider: ctx.DesktopManager,
		TriggerScan:     triggerScan,
		ApproveUpdate:   approveUpdate,
		OnDesktopHealth: onDesktopHealth,
		DockerProvider:  dockerProvider,
		EventsProvider:  ctx.EventBuffer.ReadBufferedEvents,
	})
	if err != nil {
		ctx.TeeLogger.Error("agent", "localapi", "localapi", fmt.Sprintf("start_failed error=%v", err), map[string]interface{}{"error": err.Error()})
	} else {
		defer localAPIServer.Stop()
	}

	// Start desktop app (connects to the local API socket above).
	// On Linux Desktop is launched by XDG autostart via the user's desktop
	// environment — the agent service must not spawn a second copy.
	if ctx.DesktopManager != nil && runtime.GOOS != "linux" {
		go ctx.DesktopManager.Start(ctx.Ctx)
		defer ctx.DesktopManager.Stop()
	}

	// Start kernel enforcement enforcer
	if ctx.KernelEnforcer != nil {
		if err := ctx.KernelEnforcer.Start(ctx.Ctx); err != nil {
			ctx.TeeLogger.Error("agent", "kernel", "kernel_enforcer", fmt.Sprintf("enforcer_start_failed error=%v", err), map[string]interface{}{"error": err.Error()})
		}
		defer func() {
			if err := ctx.KernelEnforcer.Stop(); err != nil {
				ctx.TeeLogger.Error("agent", "kernel", "kernel_enforcer", fmt.Sprintf("enforcer_stop_failed error=%v", err), map[string]interface{}{"error": err.Error()})
			}
		}()
	}

	if ctx.Cfg.IsStandalone() {
		return runStandaloneLoop(ctx, triggerScan)
	}

	consecutiveFailures := 0
	lastSystemInfoUpdate := time.Time{}
	lastConfigRefresh := time.Time{} // zero → refresh on first successful check-in
	tickCount := 0                   // incremented each poll; used for N-tick throttling

	for {
		// Stop-channel check before each iteration
		if ctx.StopCh != nil {
			select {
			case <-ctx.StopCh:
				log.Printf("[INFO] [agent] [loop] stop_signal_received")
				return nil
			default:
			}
		}

		tickCount++

		// Calculate jitter — always use the base check-in interval for
		// pre-fetch jitter; rapid-polling acceleration applies to the
		// post-processing sleep (recalculated after commands are handled).
		baseInterval := time.Duration(ctx.Cfg.CheckInInterval) * time.Second
		maxJitter := baseInterval / 2
		jitterCap := time.Duration(resolveJitterMaxSeconds(ctx.Cfg)) * time.Second
		if maxJitter > jitterCap {
			maxJitter = jitterCap
		}
		if maxJitter < 1*time.Second {
			maxJitter = 1 * time.Second
		}

		jitter := time.Duration(rand.Intn(int(maxJitter.Seconds())+1)) * time.Second
		time.Sleep(jitter)

		// Check for system info update
		if time.Since(lastSystemInfoUpdate) >= 1*time.Hour {
			if err := reportSystemInfo(ctx.APIClient, ctx.Cfg, ctx.DesktopManager); err != nil {
				ctx.TeeLogger.Warning("agent", "system", "system_info", fmt.Sprintf("report_system_info_failed error=%v", err), map[string]interface{}{"error": err.Error()})
			} else {
				lastSystemInfoUpdate = time.Now()
			}
		}

		// Refresh server public key if needed
		if ctx.CommandHandler.ShouldRefreshKey() {
			if err := ctx.CommandHandler.RefreshPrimaryKey(ctx.Cfg.ServerURL); err != nil {
				ctx.TeeLogger.Warning("agent", "crypto", "command_handler", fmt.Sprintf("refresh_public_key_failed error=%v", err), map[string]interface{}{"error": err.Error()})
			}
			ctx.CommandHandler.CleanupExecutedIDs()
		}

		log.Printf("Checking in with server... (Agent v%s)", version.Version)

		// Collect system metrics, then bolt on the tracked IDs so the server can both
		// (a) acknowledge command results we're still buffering (pending_acks.json) and
		// (b) confirm receipt of commands we received last round but haven't completed
		// (pending_receipts.json, Migration 033 §2).
		metrics := collectMetrics(ctx.Cfg)
		if metrics != nil {
			metrics.PendingAcknowledgments = ctx.AckTracker.GetPending()
			metrics.ReceivedCommandIDs = ctx.ReceiptTracker.GetPending()
		}

		// Get commands from server
		response, err := ctx.APIClient.GetCommands(ctx.Cfg.AgentID, metrics)
		if err != nil {
			class := classifyFailure(err)
			if errors.Is(err, client.ErrMachineMismatch) {
				// Terminal: the server no longer recognizes this host as the one the
				// agent registered on — config moved or copied. Renewal can't fix it
				// (renewal is now machine-bound too), so don't even try. Surface loudly.
				// We keep polling rather than exit, so the agent stays visible and
				// self-heals the moment an operator rebinds it server-side.
				log.Printf("[ERROR] [agent] [auth] machine_id_mismatch identity_moved_or_copied re_registration_required agent_id=%s", ctx.Cfg.AgentID)
				event.BufferSystemEvent(ctx.EventBuffer, ctx.Cfg.AgentID,
					models.EventTypeError, "machine_id_mismatch", models.SeverityCritical,
					models.ComponentAgent,
					fmt.Sprintf("Server rejected check-in with machine ID mismatch: %v", err),
					map[string]interface{}{"agent_id": ctx.Cfg.AgentID.String(), "server_url": ctx.Cfg.ServerURL})
			} else if errors.Is(err, client.ErrUnauthorized) && ctx.Cfg.RefreshToken != "" {
				log.Printf("[INFO] [agent] [auth] jwt_expired attempting_renewal agent_id=%s", ctx.Cfg.AgentID)
				renewErr := ctx.APIClient.RenewToken(ctx.Cfg.AgentID, ctx.Cfg.RefreshToken, version.Version)
				switch {
				case renewErr == nil:
					ctx.Cfg.Token = ctx.APIClient.GetToken()
					// The refresh token rotates on each renewal (migration 045) — persist
					// the new one or the next renewal will look like a replay. If the Save
					// fails here, the server's accept-previous-once grace recovers us on
					// the next attempt with the old token still on disk.
					if rt := ctx.APIClient.GetRefreshToken(); rt != "" {
						ctx.Cfg.RefreshToken = rt
					}
					if saveErr := ctx.Cfg.Save(constants.GetAgentConfigPath()); saveErr != nil {
						log.Printf("[WARNING] [agent] [auth] token_persist_failed error=%v", saveErr)
					}
					log.Printf("[INFO] [agent] [auth] token_renewed_successfully")
					consecutiveFailures = 0
					continue
				case errors.Is(renewErr, client.ErrRefreshTokenInvalid):
					// Terminal: the refresh token is dead. Backing off won't help —
					// the agent needs re-registration. Surface it loudly.
					log.Printf("[ERROR] [agent] [auth] refresh_token_invalid re_registration_required agent_id=%s error=%v", ctx.Cfg.AgentID, renewErr)
					event.BufferSystemEvent(ctx.EventBuffer, ctx.Cfg.AgentID,
						models.EventTypeError, "refresh_token_invalid", models.SeverityCritical,
						models.ComponentAgent,
						fmt.Sprintf("Refresh token rejected; re-registration required: %v", renewErr),
						map[string]interface{}{"agent_id": ctx.Cfg.AgentID.String(), "server_url": ctx.Cfg.ServerURL})
					class = classifyFailure(renewErr)
				default:
					// Transient renewal failure (network, 502). Fall through to backoff and retry.
					log.Printf("[ERROR] [agent] [auth] token_renewal_failed error=%v", renewErr)
				}
			}
			consecutiveFailures++
			recordLocalAgentStatus(ctx.Cfg, "backoff", false)
			backoffDelay := delayForFailure(class, consecutiveFailures, resolveBackoffBase(ctx.Cfg), resolveBackoffMax(ctx.Cfg))
			if class == failureTerminal {
				ctx.TeeLogger.Error("agent", "auth", "auth", fmt.Sprintf("terminal_state waiting_for_operator_intervention delay=%s agent_id=%s", backoffDelay, ctx.Cfg.AgentID), map[string]interface{}{"delay": backoffDelay.String(), "agent_id": ctx.Cfg.AgentID.String()})
			} else {
				ctx.TeeLogger.Warning("agent", "loop", "agent_loop", fmt.Sprintf("server_unavailable attempt=%d retrying_in=%s error=%v", consecutiveFailures, backoffDelay, err), map[string]interface{}{"attempt": consecutiveFailures, "retry_delay": backoffDelay.String(), "error": err.Error()})
			}
			// Non-blocking stop check during backoff
			if ctx.StopCh != nil {
				select {
				case <-ctx.StopCh:
					log.Printf("[INFO] [agent] [loop] stop_signal_received_during_backoff")
					return nil
				default:
				}
			}
			time.Sleep(backoffDelay)
			continue
		}

		consecutiveFailures = 0
		recordLocalAgentStatus(ctx.Cfg, "online", true)

		// Refresh fleet-wide operational config (polling resilience tuning) on
		// first check-in and periodically thereafter. Server is the source of
		// the fleet default; non-zero values are merged into the local config so
		// an operator change in the dashboard propagates without touching hosts.
		// Failure here is non-fatal — the agent keeps its current tuning.
		if time.Since(lastConfigRefresh) >= 15*time.Minute {
			if cfgResp, err := ctx.APIClient.GetConfig(ctx.Cfg.AgentID); err != nil {
				ctx.TeeLogger.Warning("agent", "config", "config", "config_refresh_failed", map[string]interface{}{"error": err.Error()})
			} else {
				pollingChanged := applyServerPolling(ctx.Cfg, cfgResp)
				signingChanged := applyServerCommandSigning(ctx.Cfg, cfgResp)
				if signingChanged {
					applied := crypto.SetStaleKeyMaxAge(ctx.Cfg.CommandSigning.StaleKeyMaxAgeHours)
					log.Printf("[INFO] [agent] [config] stale_key_window_updated requested_hours=%d applied=%s",
						ctx.Cfg.CommandSigning.StaleKeyMaxAgeHours, applied)
				}
				if pollingChanged || signingChanged {
					if saveErr := ctx.Cfg.Save(constants.GetAgentConfigPath()); saveErr != nil {
						ctx.TeeLogger.Error("agent", "config", "config", fmt.Sprintf("config_persist_failed error=%v", saveErr), map[string]interface{}{"error": saveErr.Error()})
					} else if pollingChanged {
						log.Printf("[INFO] [agent] [config] polling_updated jitter=%d backoff_base=%d backoff_max=%d",
							ctx.Cfg.Polling.JitterMaxSeconds, ctx.Cfg.Polling.BackoffBaseSeconds, ctx.Cfg.Polling.BackoffMaxSeconds)
					}
				}
				lastConfigRefresh = time.Now()
			}
		}

		// Log check-in success event [TD-003]
		event.BufferSystemEvent(ctx.EventBuffer, ctx.Cfg.AgentID,
			models.EventTypeAgentCheckIn, models.SubtypeSuccess, models.SeverityInfo,
			models.ComponentAgent, "Agent checked in successfully", map[string]interface{}{
				"commands_received": len(response.Commands),
				"rapid_polling":     ctx.Cfg.RapidPollingEnabled && time.Now().Before(ctx.Cfg.RapidPollingUntil),
			})

		// Drop result-acks the server confirmed.
		if response != nil && len(response.AcknowledgedIDs) > 0 {
			ctx.AckTracker.Acknowledge(response.AcknowledgedIDs)
			log.Printf("[INFO] [agent] [acknowledgment] results_acknowledged count=%d", len(response.AcknowledgedIDs))
			if err := ctx.AckTracker.Save(); err != nil {
				teeTrackerSaveFailure(ctx, "acknowledgment", "save_failed", err)
			}
		}

		// Drop receipts the server confirmed (sent→received transition successful).
		if response != nil && len(response.ReceiptConfirmedIDs) > 0 {
			ctx.ReceiptTracker.Confirm(response.ReceiptConfirmedIDs)
			log.Printf("[INFO] [agent] [receipt] receipts_confirmed count=%d", len(response.ReceiptConfirmedIDs))
			if err := ctx.ReceiptTracker.Save(); err != nil {
				teeTrackerSaveFailure(ctx, "receipt", "save_failed", err)
			}
		}

		// Bound the delivery trackers and journal whatever they abandon. A dropped
		// result-ack or receipt is the silent loss of an auditable event — the agent
		// only ever redelivers the command ID, never the payload, so a result the
		// server never recorded is unrecoverable once it ages out. ETHOS #1: it
		// becomes history, not a stdout line. BufferEvent persists to disk and flushes
		// later, so the record survives the same network loss that caused the drop.
		if dropped := ctx.AckTracker.Cleanup(); len(dropped) > 0 {
			for _, d := range dropped {
				log.Printf("[WARNING] [agent] [acknowledgment] result_ack_dropped command_id=%s reason=%s retries=%d age_s=%d",
					d.CommandID, d.Reason, d.RetryCount, d.AgeSeconds)
				event.BufferSystemEvent(ctx.EventBuffer, ctx.Cfg.AgentID,
					models.EventTypeError, models.SubtypeFailed, models.SeverityWarning,
					models.ComponentAgent,
					fmt.Sprintf("Abandoned delivery of command result %s (%s) — server never confirmed receipt", d.CommandID, d.Reason),
					map[string]interface{}{
						"kind":        "result_ack",
						"command_id":  d.CommandID,
						"reason":      d.Reason,
						"retry_count": d.RetryCount,
						"age_seconds": d.AgeSeconds,
					})
			}
			if err := ctx.AckTracker.Save(); err != nil {
				teeTrackerSaveFailure(ctx, "acknowledgment", "save_failed", err)
			}
		}
		if dropped := ctx.ReceiptTracker.Cleanup(); len(dropped) > 0 {
			for _, d := range dropped {
				log.Printf("[WARNING] [agent] [receipt] receipt_dropped command_id=%s age_s=%d", d.CommandID, d.AgeSeconds)
				event.BufferSystemEvent(ctx.EventBuffer, ctx.Cfg.AgentID,
					models.EventTypeError, models.SubtypeFailed, models.SeverityWarning,
					models.ComponentAgent,
					fmt.Sprintf("Abandoned receipt confirmation for command %s — server never acknowledged receipt before max-age", d.CommandID),
					map[string]interface{}{
						"kind":        "receipt",
						"command_id":  d.CommandID,
						"age_seconds": d.AgeSeconds,
					})
			}
			if err := ctx.ReceiptTracker.Save(); err != nil {
				teeTrackerSaveFailure(ctx, "receipt", "save_failed", err)
			}
		}

		// Drop confirmed completions the server acknowledged via ReportLog.
		// This is the key fix for duplicate command rejections: if the server
		// has confirmed a command as completed, the agent should NOT reject it
		// as a duplicate even if it's in the executed set. The race was:
		// 1. Server sends command -> agent executes -> agent reports log
		// 2. Server hasn't processed log yet -> sends command again on next poll
		// 3. Agent rejects as duplicate (BUG)
		// With this fix: agent checks if server confirmed -> if yes, allow.
		if response != nil && len(response.ConfirmedCommandIDs) > 0 {
			ctx.ConfirmedTracker.Confirm(response.ConfirmedCommandIDs)
			log.Printf("[INFO] [agent] [confirmed] completions_confirmed count=%d", len(response.ConfirmedCommandIDs))
			if err := ctx.ConfirmedTracker.Save(); err != nil {
				teeTrackerSaveFailure(ctx, "confirmed", "save_failed", err)
			}
		}

		// Report circuit breaker health
		go reportCircuitBreakerHealth(ctx)

		// Report buffered events [TD-003]
		go reportBufferedEvents(ctx)

		// Report buffered security events to server
		go reportSecurityEvents(ctx)

		// Process commands — record receipt BEFORE verification/dispatch so the server
		// stops re-issuing immediately, even for commands that turn out malformed.
		if len(response.Commands) > 0 {
			log.Printf("Received %d command(s)", len(response.Commands))
			for _, cmd := range response.Commands {
				ctx.ReceiptTracker.Add(cmd.ID)
			}
			if err := ctx.ReceiptTracker.Save(); err != nil {
				teeTrackerSaveFailure(ctx, "receipt", "save_after_receive_failed", err)
			}
			processCommands(ctx, response.Commands)
		}

		// Supply Chain Gate — pull and execute any signed capability tokens the
		// server minted for this host. Independent of the command path: tokens
		// authorize package operations directly, verified by the privileged
		// executor. A gate that is not enabled server-side returns no tokens.
		processCapabilityTokens(ctx)

		// Recalculate polling interval AFTER processing commands — a freshly
		// enabled heartbeat takes effect this cycle, not next time.
		pollingInterval := time.Duration(ctx.Cfg.CheckInInterval) * time.Second
		if ctx.Cfg.RapidPollingEnabled && time.Now().Before(ctx.Cfg.RapidPollingUntil) {
			pollingInterval = 5 * time.Second
		}

		// Sleep until next poll (or stop signal)
		if ctx.StopCh != nil {
			select {
			case <-ctx.StopCh:
				log.Printf("[INFO] [agent] [loop] stop_signal_received")
				return nil
			case <-time.After(pollingInterval):
			}
		} else {
			time.Sleep(pollingInterval)
		}
	}
}

// processCapabilityTokens fetches and processes this host's capability tokens.
// Best-effort per poll: errors are logged and the loop continues. The executor
// binary path is taken from REDFLAG_HELPER_BIN, defaulting inside the consumer.
func processCapabilityTokens(ctx *LoopContext) {
	tokens, err := ctx.APIClient.GetCapabilityTokens(ctx.Cfg.AgentID)
	if err != nil {
		ctx.TeeLogger.Warning("agent", "supplychain", "supplychain", fmt.Sprintf("token_fetch_failed error=%v", err), map[string]interface{}{"error": err.Error()})
		recordLocalCapabilityTokenFetch(ctx.Cfg, 0, err)
		return
	}
	recordLocalCapabilityTokenFetch(ctx.Cfg, len(tokens), nil)
	if len(tokens) == 0 {
		return
	}

	log.Printf("[INFO] [agent] [supplychain] tokens_received count=%d", len(tokens))
	executor := supplychain.NewExecutor(os.Getenv("REDFLAG_HELPER_BIN"))
	consumer := supplychain.NewConsumer(ctx.Cfg.AgentID, executor, ctx.APIClient)
	summary := consumer.ProcessTokensWithSummary(ctx.Ctx, tokens)
	recordLocalCapabilityTokenProcess(ctx.Cfg, summary.Processed, summary.Failed)
}

// collectMetrics collects system metrics for the check-in
func collectMetrics(cfg *config.Config) *client.SystemMetrics {
	sysMetrics, err := system.GetLightweightMetrics()
	if err != nil {
		return nil
	}

	metrics := &client.SystemMetrics{
		CPUPercent:    sysMetrics.CPUPercent,
		MemoryPercent: sysMetrics.MemoryPercent,
		MemoryUsedGB:  sysMetrics.MemoryUsedGB,
		MemoryTotalGB: sysMetrics.MemoryTotalGB,
		DiskUsedGB:    sysMetrics.DiskUsedGB,
		DiskTotalGB:   sysMetrics.DiskTotalGB,
		DiskPercent:   sysMetrics.DiskPercent,
		Uptime:        sysMetrics.Uptime,
		Version:       version.Version,
		// ARC-001: re-advertise scanner capabilities every check-in so the
		// server can pick up scanners installed/removed after registration.
		// Stateless detection from the scanner package — does NOT re-run
		// registration (that's a one-time TOFU flow).
		AvailableScanners: scanner.DetectAvailable(),
	}

	if cfg.RapidPollingEnabled && time.Now().Before(cfg.RapidPollingUntil) {
		metrics.Metadata = map[string]interface{}{
			"rapid_polling_enabled":          true,
			"rapid_polling_until":            cfg.RapidPollingUntil.Format(time.RFC3339),
			"rapid_polling_duration_minutes": int(time.Until(cfg.RapidPollingUntil).Minutes()),
		}
	}

	return metrics
}

// reportCircuitBreakerHealth reports circuit breaker status to server
func reportCircuitBreakerHealth(ctx *LoopContext) {
	cbReport := client.CircuitBreakerReport{
		Timestamp: time.Now().UTC(),
	}

	for name, cb := range ctx.CircuitBreakers {
		stats := cb.GetStats()
		cbReport.Subsystems = append(cbReport.Subsystems, client.CircuitBreakerStatus{
			Name:               name,
			State:              cb.State().String(),
			RecentFailures:     stats.RecentFailures,
			ConsecutiveSuccess: stats.ConsecutiveSuccess,
		})
	}

	if err := ctx.APIClient.ReportCircuitBreakerStats(ctx.Cfg.AgentID, cbReport); err != nil {
		log.Printf("[WARNING] Failed to report circuit breaker stats: %v", err)
	}
}

// reportBufferedEvents sends buffered events to the server [TD-003]
func reportBufferedEvents(ctx *LoopContext) {
	if ctx.EventBuffer == nil {
		return
	}
	events, err := ctx.EventBuffer.ReadBufferedEvents()
	if err != nil {
		log.Printf("[WARNING] Failed to get buffered events: %v", err)
		return
	}

	if len(events) == 0 {
		return // No events to report
	}

	accepted, rejected, err := ctx.APIClient.ReportEvents(ctx.Cfg.AgentID, events)
	if err != nil {
		log.Printf("[WARNING] Failed to report %d buffered events: %v", len(events), err)
		return
	}

	if rejected > 0 {
		log.Printf("[WARNING] Server rejected %d/%d events", rejected, len(events))
	}

	if accepted > 0 {
		log.Printf("[INFO] Successfully reported %d buffered event(s)", accepted)
	}
	if accepted+rejected > 0 {
		if err := ctx.EventBuffer.Clear(); err != nil {
			log.Printf("[WARNING] Failed to clear reported event buffer: %v", err)
		}
	}
}

// reportSecurityEvents sends buffered security events to the server.
// Runs in parallel with reportBufferedEvents after each successful check-in.
func reportSecurityEvents(ctx *LoopContext) {
	if ctx.SecurityLogger == nil {
		return
	}

	events := ctx.SecurityLogger.GetBatch()
	if len(events) == 0 {
		return
	}

	// Convert agent SecurityEvent to wire format
	wire := make([]client.AgentSecurityEvent, len(events))
	for i, e := range events {
		wire[i] = client.AgentSecurityEvent{
			Timestamp: e.Timestamp,
			Level:     e.Level,
			EventType: e.EventType,
			Message:   e.Message,
			Details:   e.Details,
		}
	}

	accepted, rejected, err := ctx.APIClient.ReportSecurityEvents(ctx.Cfg.AgentID, wire)
	if err != nil {
		log.Printf("[WARNING] [agent] [security] report_security_events_failed count=%d error=%v", len(events), err)
		return
	}
	ctx.SecurityLogger.ClearBatch(len(events))

	if rejected > 0 {
		log.Printf("[WARNING] [agent] [security] server_rejected_security_events rejected=%d accepted=%d", rejected, accepted)
	}

	if accepted > 0 {
		log.Printf("[INFO] [agent] [security] security_events_reported count=%d", accepted)
	}
}

func recordLocalAgentStatus(cfg *config.Config, status string, checkedIn bool) {
	localCache, err := cache.Load()
	if err != nil {
		log.Printf("[WARNING] [agent] [local_state] load_failed error=%v", err)
		localCache = &cache.LocalCache{}
	}
	if cfg != nil && cfg.AgentID != uuid.Nil {
		localCache.SetAgentInfo(cfg.AgentID, cfg.ServerURL)
	}
	localCache.SetAgentStatus(status)
	if checkedIn {
		localCache.UpdateCheckIn()
	}
	if err := localCache.Save(); err != nil {
		log.Printf("[WARNING] [agent] [local_state] save_failed error=%v", err)
	}
}

func recordLocalCapabilityTokenFetch(cfg *config.Config, fetched int, fetchErr error) {
	localCache, err := cache.Load()
	if err != nil {
		log.Printf("[WARNING] [agent] [local_state] load_failed error=%v", err)
		localCache = &cache.LocalCache{}
	}
	if cfg != nil && cfg.IsRegistered() {
		localCache.SetAgentInfo(cfg.AgentID, cfg.ServerURL)
	}
	localCache.RecordCapabilityTokenFetch(fetched, fetchErr)
	if err := localCache.Save(); err != nil {
		log.Printf("[WARNING] [agent] [local_state] save_failed error=%v", err)
	}
}

func recordLocalCapabilityTokenProcess(cfg *config.Config, processed, failed int) {
	localCache, err := cache.Load()
	if err != nil {
		log.Printf("[WARNING] [agent] [local_state] load_failed error=%v", err)
		localCache = &cache.LocalCache{}
	}
	if cfg != nil && cfg.IsRegistered() {
		localCache.SetAgentInfo(cfg.AgentID, cfg.ServerURL)
	}
	localCache.RecordCapabilityTokenProcess(processed, failed)
	if err := localCache.Save(); err != nil {
		log.Printf("[WARNING] [agent] [local_state] save_failed error=%v", err)
	}
}

// processCommands processes commands from the server
func processCommands(ctx *LoopContext, commands []client.Command) {
	for _, cmd := range commands {
		log.Printf("Processing command: %s (%s)", cmd.Type, cmd.ID)

		// Panic recovery for individual commands
		func() {
			defer recovery.RecoverWithCallback(fmt.Sprintf("command_%s", cmd.Type), func(err interface{}, stack []byte) {
				logReport := client.LogReport{
					CommandID: cmd.ID,
					Action:    cmd.Type,
					Result:    "panic",
					Stderr:    fmt.Sprintf("Command panic: %v\nStack: %s", err, string(stack)),
					ExitCode:  -1,
				}
				ctx.APIClient.ReportLog(ctx.Cfg.AgentID, logReport)
				ctx.AckTracker.Add(cmd.ID)
			})

			// Verify command signature
			if err := ctx.CommandHandler.ProcessCommand(cmd, ctx.Cfg, ctx.Cfg.AgentID); err != nil {
				log.Printf("[ERROR] [agent] [commands] command_rejected error=%v", err)
				logReport := client.LogReport{
					CommandID: cmd.ID,
					Action:    "verify_command",
					Result:    "failed",
					Stderr:    fmt.Sprintf("Command verification failed: %s", err),
					ExitCode:  1,
				}
				ctx.APIClient.ReportLog(ctx.Cfg.AgentID, logReport)
				ctx.AckTracker.Add(cmd.ID)
				return
			}

			// Dispatch cross-platform commands (scans + update_agent) via the
			// shared dispatcher so the cross-platform agent loop and the
			// Windows service path stay aligned.
			if !handlers.DispatchCrossPlatformCommand(ctx.APIClient, ctx.Cfg, ctx.AckTracker, ctx.ScanOrchestrator, ctx.EventBuffer, cmd) {
				log.Printf("Command type %s has no registered handler", cmd.Type)
			}
		}()
	}
}

// Polling resilience defaults. Applied when the config carries no override
// (zero value), so older config files and unset server policy keep working.
const (
	defaultJitterMaxSeconds   = 30
	defaultBackoffBaseSeconds = 10
	defaultBackoffMaxSeconds  = 300
)

// resolveJitterMaxSeconds returns the operator-configured jitter cap, or the
// built-in default when unset.
func resolveJitterMaxSeconds(cfg *config.Config) int {
	if cfg != nil && cfg.Polling.JitterMaxSeconds > 0 {
		return cfg.Polling.JitterMaxSeconds
	}
	return defaultJitterMaxSeconds
}

func resolveBackoffBase(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.Polling.BackoffBaseSeconds > 0 {
		return time.Duration(cfg.Polling.BackoffBaseSeconds) * time.Second
	}
	return defaultBackoffBaseSeconds * time.Second
}

func resolveBackoffMax(cfg *config.Config) time.Duration {
	if cfg != nil && cfg.Polling.BackoffMaxSeconds > 0 {
		return time.Duration(cfg.Polling.BackoffMaxSeconds) * time.Second
	}
	return defaultBackoffMaxSeconds * time.Second
}

// applyServerPolling merges server-delivered polling tuning into cfg.Polling.
// It returns true if any value changed. Zero/absent server values are ignored
// so the agent keeps its current (possibly locally-configured) tuning rather
// than being reset to zero. The polling loop reads cfg.Polling through the
// resolve* helpers on every iteration, so a merged change takes effect next
// loop without a restart.
func applyServerPolling(cfg *config.Config, resp *client.AgentConfigResponse) bool {
	if cfg == nil || resp == nil || resp.Polling == nil {
		return false
	}
	changed := false
	if v := resp.Polling.JitterMaxSeconds; v > 0 && v != cfg.Polling.JitterMaxSeconds {
		cfg.Polling.JitterMaxSeconds = v
		changed = true
	}
	if v := resp.Polling.BackoffBaseSeconds; v > 0 && v != cfg.Polling.BackoffBaseSeconds {
		cfg.Polling.BackoffBaseSeconds = v
		changed = true
	}
	if v := resp.Polling.BackoffMaxSeconds; v > 0 && v != cfg.Polling.BackoffMaxSeconds {
		cfg.Polling.BackoffMaxSeconds = v
		changed = true
	}
	return changed
}

// applyServerCommandSigning merges fleet-wide command-signing policy delivered
// by the server into the local config. Returns true if anything changed so the
// caller persists and re-applies it. The agent still clamps the stale-key window
// to its doctrinal range at use (crypto.SetStaleKeyMaxAge); this only records
// the operator's requested value.
func applyServerCommandSigning(cfg *config.Config, resp *client.AgentConfigResponse) bool {
	if cfg == nil || resp == nil || resp.CommandSigning == nil {
		return false
	}
	if v := resp.CommandSigning.StaleKeyMaxAgeHours; v > 0 && v != cfg.CommandSigning.StaleKeyMaxAgeHours {
		cfg.CommandSigning.StaleKeyMaxAgeHours = v
		return true
	}
	return false
}

// teeTrackerSaveFailure journals a delivery-tracker persistence failure inward
// (ETHOS #1). A tracker that cannot persist risks double-delivery or
// replay-rejection after a crash — the server needs the record, not just the
// local journal. TeeLogger both logs and buffers a SystemEvent.
func teeTrackerSaveFailure(ctx *LoopContext, component, action string, err error) {
	ctx.TeeLogger.Error("agent", component, "tracker",
		fmt.Sprintf("%s error=%v", action, err),
		map[string]interface{}{"action": action, "error": err.Error()})
}

// failureClass partitions polling failures by how they recover (BUG-014).
type failureClass int

const (
	// failureTransient — network blips, 5xx, DNS, transient renewal failures.
	// Self-healing: exponential backoff with full jitter.
	failureTransient failureClass = iota
	// failureTerminal — dead credentials (refresh-token reuse/revocation) or
	// machine-binding mismatch. Retrying cannot fix these; an operator must
	// rebind or re-register. Long flat delay keeps the agent visible without
	// hammering the server with requests that can only fail.
	failureTerminal
)

// terminalRetryDelay is the flat poll interval in a terminal credential state —
// long enough not to hammer the server, short enough that an operator-side
// rebind is picked up within the same working session.
const terminalRetryDelay = 10 * time.Minute

// classifyFailure maps a polling or renewal error to its failure class. This is
// the single source of truth for which states are terminal.
func classifyFailure(err error) failureClass {
	if errors.Is(err, client.ErrMachineMismatch) || errors.Is(err, client.ErrRefreshTokenInvalid) {
		return failureTerminal
	}
	return failureTransient
}

// delayForFailure is the backoff policy: it returns the wait before the next
// poll attempt for the given failure class.
func delayForFailure(class failureClass, attempt int, base, maxDelay time.Duration) time.Duration {
	if class == failureTerminal {
		return terminalRetryDelay
	}
	return calculateBackoff(attempt, base, maxDelay)
}

// calculateBackoff returns an exponential backoff delay with full jitter,
// bounded by the admin-adjustable base (floor) and maxDelay (ceiling).
// It is the transient curve behind delayForFailure.
func calculateBackoff(attempt int, base, maxDelay time.Duration) time.Duration {
	ceiling := base * time.Duration(1<<uint(attempt))
	if ceiling > maxDelay || ceiling <= 0 {
		ceiling = maxDelay
	}

	delay := time.Duration(rand.Int63n(int64(ceiling)))
	if delay < base {
		delay = base
	}
	return delay
}

// reportSystemInfo reports detailed system information to server
func reportSystemInfo(apiClient *client.Client, cfg *config.Config, desktopMgr *desktop.Manager) error {
	sysInfo, err := system.GetSystemInfo(version.Version)
	if err != nil {
		return err
	}

	report := client.SystemInfoReport{
		Timestamp:   time.Now().UTC(),
		CPUModel:    sysInfo.CPUInfo.ModelName,
		CPUCores:    sysInfo.CPUInfo.Cores,
		CPUThreads:  sysInfo.CPUInfo.Threads,
		MemoryTotal: uint64(sysInfo.MemoryInfo.Total),
		IPAddress:   sysInfo.IPAddress,
		Processes:   sysInfo.RunningProcesses,
		Uptime:      sysInfo.Uptime,
		DeviceType:  sysInfo.DeviceType,
		DeviceModel: sysInfo.DeviceModel,
		OSDistro:    sysInfo.OSDistro,
	}

	if len(sysInfo.DiskInfo) > 0 {
		report.DiskTotal = uint64(sysInfo.DiskInfo[0].Total)
		report.DiskUsed = uint64(sysInfo.DiskInfo[0].Used)
	}

	// Fold in any locally-observed integrations (Sunshine, etc.). Observe-only:
	// the server merges this under agent.metadata["integrations"], which the
	// dashboard renders. Empty when nothing is detected.
	if detected := integrations.Detect(sysInfo.IPAddress); len(detected) > 0 {
		report.Metadata = map[string]interface{}{"integrations": detected}
	}

	// Collect top processes for the dashboard's "Top Processes" table.
	// Stored in Metadata so it flows through the existing merge path —
	// no schema change needed on the server side.
	if topProcs, err := system.GetTopProcesses(5); err == nil && len(topProcs) > 0 {
		procs := make([]interface{}, len(topProcs))
		for i, p := range topProcs {
			procs[i] = map[string]interface{}{
				"name": p.Name, "pid": p.PID, "cpu": p.CPU, "mem": p.Mem,
			}
		}
		if report.Metadata == nil {
			report.Metadata = map[string]interface{}{}
		}
		report.Metadata["top_processes"] = procs
	}

	// Desktop component state (UPDATE-002/INSTALL-004): installed/running/
	// version, sourced from Desktop's own health reports since the Linux app
	// is autostart-launched, not agent-spawned. Merges under
	// agent.metadata["desktop"] — the backend for a fleet "components
	// installed" indicator.
	if desktopMgr != nil {
		if report.Metadata == nil {
			report.Metadata = map[string]interface{}{}
		}
		report.Metadata["desktop"] = desktopMgr.Health()
	}

	return apiClient.ReportSystemInfo(cfg.AgentID, report)
}
