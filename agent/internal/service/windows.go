//go:build windows

package service

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/agent"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/recovery"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

var (
	elog        debug.Log
	serviceName = "RedFlagAgent"
)

type redflagService struct {
	agent   *config.Config
	stop    chan struct{}
	loopCtx *agent.LoopContext
}

func (s *redflagService) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (ssec bool, errno uint32) {
	const cmdsAccepted = svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPauseAndContinue
	changes <- svc.Status{State: svc.StartPending}

	// Initialize event logging
	var err error
	elog, err = eventlog.Open(serviceName)
	if err != nil {
		log.Printf("Failed to open event log: %v", err)
		elog = debug.New("RedFlagAgent")
	}
	defer elog.Close()

	elog.Info(1, fmt.Sprintf("Starting %s service", serviceName))

	// Create stop channel
	s.stop = make(chan struct{})

	// Initialize service (synchronous - fail fast on critical errors)
	if err := s.initialize(); err != nil {
		elog.Error(1, fmt.Sprintf("Service initialization failed, stopping service: %v", err))
		changes <- svc.Status{State: svc.Stopped}
		return true, 1 // Signal failure to Service Manager
	}

	// Start the agent check-in loop in a goroutine
	go s.runAgent()

	// Signal that service is running
	changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}

	elog.Info(1, fmt.Sprintf("%s service is now running", serviceName))

	// Handle service control requests
loop:
	for {
		select {
		case c := <-r:
			switch c.Cmd {
			case svc.Interrogate:
				changes <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				elog.Info(1, fmt.Sprintf("Stopping %s service", serviceName))
				changes <- svc.Status{State: svc.StopPending}
				close(s.stop) // Signal agent to stop gracefully
				break loop
			case svc.Pause:
				elog.Info(1, fmt.Sprintf("Pausing %s service", serviceName))
				changes <- svc.Status{State: svc.Paused, Accepts: cmdsAccepted}
			case svc.Continue:
				elog.Info(1, fmt.Sprintf("Continuing %s service", serviceName))
				changes <- svc.Status{State: svc.Running, Accepts: cmdsAccepted}
			default:
				elog.Error(1, fmt.Sprintf("Unexpected control request #%d", c))
			}
		case <-s.stop:
			break loop
		}
	}

	elog.Info(1, fmt.Sprintf("%s service stopped", serviceName))
	changes <- svc.Status{State: svc.Stopped}
	return
}

// initialize performs all critical service initialization.
// This runs synchronously before the service enters Running state.
// If critical initialization fails, the service will NOT start.
func (s *redflagService) initialize() error {
	log.Printf("[INFO] [windows] [service] initialization_starting")

	loopCtx, err := agent.NewLoopContext(s.agent, agent.LoopContextOptions{
		Ctx:           context.Background(),
		StopCh:        s.stop,
		EnableDesktop: false,
	})
	if err != nil {
		log.Printf("[ERROR] [windows] [service] context_init_failed error=\"%v\"", err)
		elog.Error(1, fmt.Sprintf("Agent context init failed: %v", err))
		return fmt.Errorf("failed to initialize agent context: %w", err)
	}
	s.loopCtx = loopCtx

	log.Printf("[INFO] [windows] [service] initialization_complete")
	return nil
}

func (s *redflagService) runAgent() {
	// [TD-002] Panic recovery for Windows service agent loop
	defer recovery.Recover("windows_service_agent")

	log.Printf("[INFO] [agent] [service] starting agent_id=%s server=%s interval=%ds",
		s.agent.AgentID, s.agent.ServerURL, s.agent.CheckInInterval)

	if s.loopCtx == nil {
		log.Printf("[ERROR] [agent] [service] polling_loop_missing_context")
		elog.Error(1, "Polling loop missing agent context")
		return
	}

	if err := agent.RunPollingLoop(s.loopCtx); err != nil {
		log.Printf("[ERROR] [agent] [service] polling_loop_failed error=%v", err)
		elog.Error(1, fmt.Sprintf("Polling loop failed: %v", err))
	}
}

// RunService executes the agent as a Windows service
func RunService(cfg *config.Config) error {
	elog, err := eventlog.Open(serviceName)
	if err != nil {
		return fmt.Errorf("failed to open event log: %w", err)
	}
	defer elog.Close()

	elog.Info(1, fmt.Sprintf("Starting %s service", serviceName))

	s := &redflagService{
		agent: cfg,
	}

	// Run as service
	if err := svc.Run(serviceName, s); err != nil {
		elog.Error(1, fmt.Sprintf("%s service failed: %v", serviceName, err))
		return fmt.Errorf("service failed: %w", err)
	}

	elog.Info(1, fmt.Sprintf("%s service stopped", serviceName))
	return nil
}

// IsService returns true if running as Windows service
func IsService() bool {
	isService, _ := svc.IsWindowsService()
	return isService
}

// InstallService installs the agent as a Windows service
func InstallService() error {
	exePath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to get executable path: %w", err)
	}

	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err == nil {
		s.Close()
		return fmt.Errorf("service %s already exists", serviceName)
	}

	// Create service with proper configuration
	s, err = m.CreateService(serviceName, exePath, mgr.Config{
		DisplayName:  "RedFlag Update Agent",
		Description:  "RedFlag agent for automated system updates and monitoring",
		StartType:    mgr.StartAutomatic,
		Dependencies: []string{"Tcpip", "Dnscache"},
	})
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}
	defer s.Close()

	// Set recovery actions
	if err := s.SetRecoveryActions([]mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 120 * time.Second},
	}, 0); err != nil {
		return fmt.Errorf("failed to set recovery actions: %w", err)
	}

	log.Printf("Service %s installed successfully", serviceName)
	return nil
}

// RemoveService removes the Windows service
func RemoveService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s not found", serviceName)
	}
	defer s.Close()

	// Stop service if running
	status, err := s.Query()
	if err != nil {
		return fmt.Errorf("failed to query service status: %w", err)
	}

	if status.State != svc.Stopped {
		if _, err := s.Control(svc.Stop); err != nil {
			return fmt.Errorf("failed to stop service: %w", err)
		}
		log.Printf("Stopping service...")
		time.Sleep(5 * time.Second) // Wait for service to stop
	}

	// Delete service
	if err := s.Delete(); err != nil {
		return fmt.Errorf("failed to delete service: %w", err)
	}

	log.Printf("Service %s removed successfully", serviceName)
	return nil
}

// StartService starts the Windows service
func StartService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s not found", serviceName)
	}
	defer s.Close()

	if err := s.Start(); err != nil {
		return fmt.Errorf("failed to start service: %w", err)
	}

	log.Printf("Service %s started successfully", serviceName)
	return nil
}

// StopService stops the Windows service
func StopService() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s not found", serviceName)
	}
	defer s.Close()

	if _, err := s.Control(svc.Stop); err != nil {
		return fmt.Errorf("failed to stop service: %w", err)
	}

	log.Printf("Service %s stopped successfully", serviceName)
	return nil
}

// ServiceStatus returns the current status of the Windows service
func ServiceStatus() error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("failed to connect to service manager: %w", err)
	}
	defer m.Disconnect()

	s, err := m.OpenService(serviceName)
	if err != nil {
		return fmt.Errorf("service %s not found", serviceName)
	}
	defer s.Close()

	status, err := s.Query()
	if err != nil {
		return fmt.Errorf("failed to query service status: %w", err)
	}

	state := "UNKNOWN"
	switch status.State {
	case svc.Stopped:
		state = "STOPPED"
	case svc.StartPending:
		state = "STARTING"
	case svc.Running:
		state = "RUNNING"
	case svc.StopPending:
		state = "STOPPING"
	case svc.Paused:
		state = "PAUSED"
	case svc.PausePending:
		state = "PAUSING"
	case svc.ContinuePending:
		state = "RESUMING"
	}

	log.Printf("Service %s status: %s", serviceName, state)
	return nil
}

func RunConsole(cfg *config.Config) error {
	log.Printf("[INFO] [agent] [service]RedFlag Agent starting in console mode...")
	log.Printf("Press Ctrl+C to stop")

	// Handle console signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Create stop channel for graceful shutdown
	stopChan := make(chan struct{})

	// Start agent in goroutine
	go func() {
		defer close(stopChan)
		log.Printf("Agent console mode running...")
		ticker := time.NewTicker(time.Duration(cfg.CheckInInterval) * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				log.Printf("Checking in with server...")
			case <-stopChan:
				log.Printf("Shutting down console agent...")
				return
			}
		}
	}()

	// Wait for signal
	<-sigChan
	log.Printf("Received shutdown signal, stopping agent...")

	// Graceful shutdown
	close(stopChan)
	time.Sleep(2 * time.Second) // Allow cleanup

	log.Printf("Agent stopped")
	return nil
}
