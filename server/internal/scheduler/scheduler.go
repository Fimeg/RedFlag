package scheduler

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/database/queries"
	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/Fimeg/RedFlag/server/internal/services"
	"github.com/gofrs/uuid/v5"
)

// Config holds scheduler configuration
type Config struct {
	// CheckInterval is how often to check the queue for due jobs
	CheckInterval time.Duration

	// LookaheadWindow is how far ahead to look for jobs
	// Jobs due within this window will be batched and jittered
	LookaheadWindow time.Duration

	// MaxJitter is the maximum random delay added to job execution
	MaxJitter time.Duration

	// NumWorkers is the number of parallel workers for command creation
	NumWorkers int

	// BackpressureThreshold is max pending commands per agent before skipping
	BackpressureThreshold int

	// RateLimitPerSecond is max commands created per second (0 = unlimited)
	RateLimitPerSecond int

	// MaxDispatchPerTick caps how many due jobs are popped and dispatched in a
	// single tick (SCALE-001 S4). Without a cap, a fleet whose subsystem jobs
	// phase-align can dump thousands of jobs into one tick, a thundering herd
	// against the DB pool. The remainder stays queued for the next tick. 0 =
	// unlimited (legacy behavior).
	MaxDispatchPerTick int
}

// DefaultConfig returns default configuration values
func DefaultConfig() Config {
	return Config{
		CheckInterval:         10 * time.Second,
		LookaheadWindow:       60 * time.Second,
		MaxJitter:             30 * time.Second,
		NumWorkers:            10,
		BackpressureThreshold: 5,
		RateLimitPerSecond:    100,
		MaxDispatchPerTick:    200,
	}
}

// Scheduler manages subsystem job scheduling with priority queue and worker pool
type Scheduler struct {
	config Config
	queue  *PriorityQueue

	// Database queries
	agentQueries     *queries.AgentQueries
	commandQueries   *queries.CommandQueries
	subsystemQueries *queries.SubsystemQueries

	// Signing
	signingService *services.SigningService

	// Worker pool
	jobChan chan *SubsystemJob
	workers []*worker

	// Rate limiting
	rateLimiter chan struct{}

	// Lifecycle management
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	shutdown chan struct{}

	// Metrics
	mu    sync.RWMutex
	stats Stats
}

// Stats holds scheduler statistics
type Stats struct {
	JobsProcessed       int64
	JobsSkipped         int64
	CommandsCreated     int64
	CommandsFailed      int64
	BackpressureSkips   int64
	LastProcessedAt     time.Time
	QueueSize           int
	WorkerPoolUtilized  int
	AverageProcessingMS int64
}

// NewScheduler creates a new scheduler instance
func NewScheduler(config Config, agentQueries *queries.AgentQueries, commandQueries *queries.CommandQueries, subsystemQueries *queries.SubsystemQueries, signingService *services.SigningService) *Scheduler {
	ctx, cancel := context.WithCancel(context.Background())

	s := &Scheduler{
		config:           config,
		queue:            NewPriorityQueue(),
		agentQueries:     agentQueries,
		commandQueries:   commandQueries,
		subsystemQueries: subsystemQueries,
		signingService:   signingService,
		jobChan:          make(chan *SubsystemJob, 1000), // Buffer 1000 jobs
		workers:          make([]*worker, config.NumWorkers),
		shutdown:         make(chan struct{}),
		ctx:              ctx,
		cancel:           cancel,
	}

	// Initialize rate limiter if configured
	if config.RateLimitPerSecond > 0 {
		s.rateLimiter = make(chan struct{}, config.RateLimitPerSecond)
		go s.refillRateLimiter()
	}

	// Initialize workers
	for i := 0; i < config.NumWorkers; i++ {
		s.workers[i] = &worker{
			id:        i,
			scheduler: s,
		}
	}

	return s
}

// LoadSubsystems loads all enabled auto-run subsystems from database into queue
func (s *Scheduler) LoadSubsystems(ctx context.Context) error {
	log.Println("[Scheduler] Loading subsystems from database...")

	// Get all agents (pass empty strings to get all agents regardless of status/os)
	agents, err := s.agentQueries.ListAgents("", "")
	if err != nil {
		return fmt.Errorf("failed to get agents: %w", err)
	}

	// Build the online-agent ID slice first so we only fetch rows we need.
	// Agents that haven't checked in for 10+ minutes are considered offline
	// and contribute no jobs, so there is no point fetching their subsystems.
	onlineAgentIDs := make([]uuid.UUID, 0, len(agents))
	onlineAgentByID := make(map[uuid.UUID]struct{ Hostname string }, len(agents))
	for _, agent := range agents {
		if time.Since(agent.LastSeen) > 10*time.Minute {
			continue
		}
		onlineAgentIDs = append(onlineAgentIDs, agent.ID)
		onlineAgentByID[agent.ID] = struct{ Hostname string }{agent.Hostname}
	}

	if len(onlineAgentIDs) == 0 {
		log.Printf("[Scheduler] Loaded 0 subsystem jobs for 0 agents (respecting database settings)\n")
		return nil
	}

	// Single query for all subsystem rows across all online agents (SCALE-001 S10).
	allSubsystems, err := s.subsystemQueries.GetSubsystemsForAgents(onlineAgentIDs)
	if err != nil {
		return fmt.Errorf("failed to get subsystems for agents: %w", err)
	}

	// Group subsystem rows by agent ID for O(1) lookup below.
	byAgent := groupSubsystemsByAgent(allSubsystems)

	loaded := 0
	for _, agentID := range onlineAgentIDs {
		agentInfo := onlineAgentByID[agentID]
		for _, dbSub := range byAgent[agentID] {
			// ARC-004: legacy "updates" virtual subsystem is deprecated.
			// It pre-dates per-scanner subsystems (apt/dnf/winget/windows)
			// and fans out to every package scanner regardless of what's
			// actually present on the host. With ARC-001 capability
			// advertisement and ARC-002 scheduler filter, the per-scanner
			// rows now do this correctly. Skip "updates" rows so we stop
			// generating fan-out commands without requiring a DB migration
			// to delete them. Migration to remove these rows is a follow-up.
			if dbSub.Subsystem == "updates" {
				continue
			}
			if dbSub.Enabled && dbSub.AutoRun {
				// Use database interval, fallback to default
				intervalMinutes := dbSub.IntervalMinutes
				if intervalMinutes <= 0 {
					intervalMinutes = s.getDefaultInterval(dbSub.Subsystem)
				}

				var nextRun time.Time
				if dbSub.NextRunAt != nil {
					nextRun = *dbSub.NextRunAt
				} else {
					// If no next run is set, schedule it with default interval
					nextRun = time.Now().UTC().Add(time.Duration(intervalMinutes) * time.Minute)
				}

				job := &SubsystemJob{
					AgentID:         agentID,
					AgentHostname:   agentInfo.Hostname,
					Subsystem:       dbSub.Subsystem,
					IntervalMinutes: intervalMinutes,
					NextRunAt:       nextRun,
					Enabled:         dbSub.Enabled,
				}

				s.queue.Push(job)
				loaded++
			}
		}
	}

	log.Printf("[Scheduler] Loaded %d subsystem jobs for %d agents (respecting database settings)\n", loaded, len(onlineAgentIDs))
	return nil
}

// groupSubsystemsByAgent groups a flat slice of AgentSubsystem rows into a map
// keyed by agent ID. This is a pure helper so it can be tested without a DB.
func groupSubsystemsByAgent(rows []models.AgentSubsystem) map[uuid.UUID][]models.AgentSubsystem {
	out := make(map[uuid.UUID][]models.AgentSubsystem, len(rows))
	for _, row := range rows {
		out[row.AgentID] = append(out[row.AgentID], row)
	}
	return out
}

// getDefaultInterval returns default interval minutes for a subsystem
// TODO: These intervals need to correlate with agent health scanning settings
// Each subsystem should be variable based on user-configurable agent health policies
func (s *Scheduler) getDefaultInterval(subsystem string) int {
	// ARC-004: "updates" intentionally absent — it's the deprecated virtual
	// subsystem that fans out to all package scanners. Per-scanner rows
	// (apt/dnf/winget/windows) replaced it via ARC-001 capability advertisement.
	defaults := map[string]int{
		"apt":     30,  // 30 minutes
		"dnf":     240, // 4 hours
		"docker":  120, // 2 hours
		"storage": 360, // 6 hours
		"windows": 480, // 8 hours
		"winget":  360, // 6 hours
		"system":  30,  // 30 minutes
	}

	if interval, exists := defaults[subsystem]; exists {
		return interval
	}
	return 30 // Default fallback
}

// Start begins the scheduler main loop and workers
func (s *Scheduler) Start() error {
	log.Printf("[Scheduler] Starting with %d workers, check interval %v\n",
		s.config.NumWorkers, s.config.CheckInterval)

	// Start workers
	for _, w := range s.workers {
		s.wg.Add(1)
		go w.run()
	}

	// Start main loop
	s.wg.Add(1)
	go s.mainLoop()

	log.Println("[Scheduler] Started successfully")
	return nil
}

// Stop gracefully shuts down the scheduler
func (s *Scheduler) Stop() error {
	log.Println("[Scheduler] Shutting down...")

	// Signal shutdown
	s.cancel()
	close(s.shutdown)

	// Close job channel (workers will drain and exit)
	close(s.jobChan)

	// Wait for all goroutines with timeout
	done := make(chan struct{})
	go func() {
		s.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Println("[Scheduler] Shutdown complete")
		return nil
	case <-time.After(30 * time.Second):
		log.Println("[Scheduler] Shutdown timeout - forcing exit")
		return fmt.Errorf("shutdown timeout")
	}
}

// mainLoop is the scheduler's main processing loop
func (s *Scheduler) mainLoop() {
	defer s.wg.Done()

	ticker := time.NewTicker(s.config.CheckInterval)
	defer ticker.Stop()

	log.Printf("[Scheduler] Main loop started (check every %v)\n", s.config.CheckInterval)

	for {
		select {
		case <-s.shutdown:
			log.Println("[Scheduler] Main loop shutting down")
			return

		case <-ticker.C:
			s.processQueue()
		}
	}
}

// processQueue checks for due jobs and dispatches them to workers
func (s *Scheduler) processQueue() {
	start := time.Now()

	// Get jobs due within the lookahead window, capped per tick (SCALE-001 S4)
	// so an aligned fleet can't dump the whole queue into one tick. Overflow
	// stays queued and is picked up next tick.
	cutoff := time.Now().UTC().Add(s.config.LookaheadWindow)
	dueJobs := s.queue.PopBefore(cutoff, s.config.MaxDispatchPerTick)
	if s.config.MaxDispatchPerTick > 0 && len(dueJobs) == s.config.MaxDispatchPerTick && s.queue.Len() > 0 {
		log.Printf("[Scheduler] Dispatch cap hit: %d jobs this tick, %d still queued",
			len(dueJobs), s.queue.Len())
	}

	if len(dueJobs) == 0 {
		// No jobs due, just update stats
		s.mu.Lock()
		s.stats.QueueSize = s.queue.Len()
		s.mu.Unlock()
		return
	}

	log.Printf("[Scheduler] Processing %d jobs due before %s\n",
		len(dueJobs), cutoff.Format("15:04:05"))

	// Add jitter to each job and dispatch to workers
	dispatched := 0
	for _, job := range dueJobs {
		// Add random jitter (0 to MaxJitter)
		jitter := time.Duration(rand.Intn(int(s.config.MaxJitter.Seconds()))) * time.Second
		job.NextRunAt = job.NextRunAt.Add(jitter)

		// Dispatch to worker pool (non-blocking)
		select {
		case s.jobChan <- job:
			dispatched++
		default:
			// Worker pool full, re-queue job
			log.Printf("[Scheduler] Worker pool full, re-queueing %s\n", job.String())
			s.queue.Push(job)

			s.mu.Lock()
			s.stats.JobsSkipped++
			s.mu.Unlock()
		}
	}

	// Update stats
	duration := time.Since(start)
	s.mu.Lock()
	s.stats.JobsProcessed += int64(dispatched)
	s.stats.LastProcessedAt = time.Now().UTC()
	s.stats.QueueSize = s.queue.Len()
	s.stats.WorkerPoolUtilized = len(s.jobChan)
	s.stats.AverageProcessingMS = duration.Milliseconds()
	s.mu.Unlock()

	log.Printf("[Scheduler] Dispatched %d jobs in %v (queue: %d remaining)\n",
		dispatched, duration, s.queue.Len())
}

// refillRateLimiter continuously refills the rate limiter token bucket
func (s *Scheduler) refillRateLimiter() {
	ticker := time.NewTicker(time.Second / time.Duration(s.config.RateLimitPerSecond))
	defer ticker.Stop()

	for {
		select {
		case <-s.shutdown:
			return
		case <-ticker.C:
			// Try to add token (non-blocking)
			select {
			case s.rateLimiter <- struct{}{}:
			default:
				// Bucket full, skip
			}
		}
	}
}

// RemoveSubsystemJob removes a scheduled job for a given agent + subsystem from
// the priority queue. Used by DisableSubsystem so disabling a subsystem stops
// further commands immediately, without waiting for a scheduler reload.
func (s *Scheduler) RemoveSubsystemJob(agentID uuid.UUID, subsystem string) bool {
	return s.queue.Remove(agentID, subsystem)
}

// GetStats returns current scheduler statistics (thread-safe)
func (s *Scheduler) GetStats() Stats {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.stats
}

// GetQueueStats returns current queue statistics
func (s *Scheduler) GetQueueStats() QueueStats {
	return s.queue.GetStats()
}

// worker processes jobs from the job channel
type worker struct {
	id        int
	scheduler *Scheduler
}

func (w *worker) run() {
	defer w.scheduler.wg.Done()

	log.Printf("[Worker %d] Started\n", w.id)

	for job := range w.scheduler.jobChan {
		if err := w.processJob(job); err != nil {
			log.Printf("[Worker %d] Failed to process %s: %v\n", w.id, job.String(), err)

			w.scheduler.mu.Lock()
			w.scheduler.stats.CommandsFailed++
			w.scheduler.mu.Unlock()
		} else {
			w.scheduler.mu.Lock()
			w.scheduler.stats.CommandsCreated++
			w.scheduler.mu.Unlock()
		}

		// Re-queue job for next execution
		job.NextRunAt = time.Now().UTC().Add(time.Duration(job.IntervalMinutes) * time.Minute)
		w.scheduler.queue.Push(job)

		// Persist last_run_at / next_run_at so the UI shows a current "Next Run" time
		if dbErr := w.scheduler.subsystemQueries.UpdateLastRun(job.AgentID, job.Subsystem); dbErr != nil {
			log.Printf("[WARN] [Worker %d] [scheduler] update_last_run_failed agent=%s subsystem=%s err=%v",
				w.id, job.AgentHostname, job.Subsystem, dbErr)
		}
	}

	log.Printf("[Worker %d] Stopped\n", w.id)
}

func (w *worker) processJob(job *SubsystemJob) error {
	// Apply rate limiting if configured
	if w.scheduler.rateLimiter != nil {
		select {
		case <-w.scheduler.rateLimiter:
			// Token acquired
		case <-w.scheduler.shutdown:
			return fmt.Errorf("shutdown during rate limit wait")
		}
	}

	// Check backpressure: skip if agent has too many pending commands
	pendingCount, err := w.scheduler.commandQueries.CountPendingCommandsForAgent(job.AgentID)
	if err != nil {
		return fmt.Errorf("failed to check pending commands: %w", err)
	}

	if pendingCount >= w.scheduler.config.BackpressureThreshold {
		log.Printf("[Worker %d] Backpressure: agent %s has %d pending commands, skipping %s\n",
			w.id, job.AgentHostname, pendingCount, job.Subsystem)

		w.scheduler.mu.Lock()
		w.scheduler.stats.BackpressureSkips++
		w.scheduler.mu.Unlock()

		return nil // Not an error, just skipped
	}

	// BUG-015 FIX: Get platform-specific scan command type
	commandType, err := w.getScanCommandType(job)
	if err != nil {
		return fmt.Errorf("failed to determine scan command type: %w", err)
	}

	// ARC-002: skip platform-specific scan commands when the agent has
	// advertised its available scanners (via ARC-001) and this scanner
	// isn't in that set. Safe-by-default: agents that haven't reported
	// scanners yet (legacy / pre-upgrade) fall through and behave as before.
	if scanner := scannerForCommandType(commandType); scanner != "" {
		if w.shouldSkipForUnavailableScanner(job.AgentID, scanner) {
			log.Printf("[INFO] [Worker %d] [scheduler] skip_unavailable_scanner agent=%s scanner=%s type=%s",
				w.id, job.AgentHostname, scanner, commandType)
			w.scheduler.mu.Lock()
			w.scheduler.stats.JobsSkipped++
			w.scheduler.mu.Unlock()
			return nil
		}
	}

	// FIX: Check for duplicate pending command before creating
	existingCmd, err := w.scheduler.commandQueries.GetPendingCommandByType(job.AgentID, commandType)
	if err == nil && existingCmd != nil {
		// Duplicate pending command exists, skip creating new one
		log.Printf("[INFO] [Worker %d] [scheduler] duplicate_command_skip agent=%s type=%s existing_id=%s",
			w.id, job.AgentHostname, commandType, existingCmd.ID)
		return nil
	}
	cmd := &models.AgentCommand{
		ID:          uuid.Must(uuid.NewV4()),
		AgentID:     job.AgentID,
		CommandType: commandType,
		Params:      models.JSONB{},
		Status:      models.CommandStatusPending,
		Source:      models.CommandSourceSystem,
		CreatedAt:   time.Now().UTC(),
	}

	if w.scheduler.signingService == nil || !w.scheduler.signingService.IsEnabled() {
		return fmt.Errorf("signing service not available - command rejected")
	}

	signature, err := w.scheduler.signingService.SignCommand(cmd)
	if err != nil {
		return fmt.Errorf("failed to sign command: %w", err)
	}
	cmd.Signature = signature

	if err := w.scheduler.commandQueries.CreateCommand(cmd); err != nil {
		return fmt.Errorf("failed to create command: %w", err)
	}

	log.Printf("[INFO] [Worker %d] [scheduler] command_created agent=%s type=%s id=%s",
		w.id, job.AgentHostname, commandType, cmd.ID)

	return nil
}

// getScanCommandType returns platform-specific scan command type based on agent OS
func (w *worker) getScanCommandType(job *SubsystemJob) (string, error) {
	// Get agent details to determine platform
	agent, err := w.scheduler.agentQueries.GetAgentByID(job.AgentID)
	if err != nil {
		return "", fmt.Errorf("failed to get agent %s: %w", job.AgentID, err)
	}

	// Map subsystem to platform-specific command type
	switch job.Subsystem {
	case "updates":
		return w.getUpdateScanCommandType(agent.OSType)
	case "apt":
		return "scan_apt", nil
	case "dnf":
		return "scan_dnf", nil
	case "docker":
		return "scan_docker", nil
	case "storage":
		return "scan_storage", nil
	case "system":
		return "scan_system", nil
	case "windows":
		return "scan_windows", nil
	case "winget":
		return "scan_winget", nil
	default:
		// Default to generic scan command
		return fmt.Sprintf("scan_%s", job.Subsystem), nil
	}
}

// scannerForCommandType maps a platform-specific scan_* command type to the
// scanner identifier the agent reports via ARC-001 capability advertisement.
// Returns "" for command types that aren't tied to a specific scanner
// (storage, system, the legacy "updates" virtual subsystem, unknown distros).
func scannerForCommandType(commandType string) string {
	switch commandType {
	case "scan_apt":
		return "apt"
	case "scan_dnf":
		return "dnf"
	case "scan_winget":
		return "winget"
	case "scan_windows":
		return "windows"
	case "scan_docker":
		return "docker"
	default:
		return ""
	}
}

// shouldSkipForUnavailableScanner returns true when the agent has reported a
// concrete list of available scanners (ARC-001 metadata) and the requested
// scanner isn't in that list. Returns false when the metadata is missing or
// malformed — that means we don't yet have authoritative info, so we fall
// through to the legacy behavior rather than silently dropping commands.
func (w *worker) shouldSkipForUnavailableScanner(agentID uuid.UUID, scanner string) bool {
	agent, err := w.scheduler.agentQueries.GetAgentByID(agentID)
	if err != nil || agent == nil || agent.Metadata == nil {
		return false
	}
	raw, ok := agent.Metadata["available_scanners"]
	if !ok {
		return false
	}
	list, ok := raw.([]interface{})
	if !ok {
		return false
	}
	for _, item := range list {
		if s, ok := item.(string); ok && s == scanner {
			return false
		}
	}
	return true
}

// getUpdateScanCommandType returns the appropriate update scan command for the OS
func (w *worker) getUpdateScanCommandType(osType string) (string, error) {
	switch osType {
	case "debian", "ubuntu", "linuxmint", "pop", "elementary":
		return "scan_apt", nil
	case "fedora", "rhel", "centos", "rocky", "almalinux", "oracle":
		return "scan_dnf", nil
	case "arch", "manjaro", "endeavouros":
		return "scan_pacman", nil
	case "opensuse", "suse":
		return "scan_zypper", nil
	case "windows":
		return "scan_windows", nil
	default:
		// Fallback to generic scan_updates for unknown Linux distros
		if osType == "linux" {
			return "scan_updates", nil
		}
		return "scan_updates", nil
	}
}
