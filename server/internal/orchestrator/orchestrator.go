package orchestrator

import (
	"log"
	"sync"
	"time"

	"github.com/gofrs/uuid/v5"
)

// Orchestrator advances the package update lifecycle on a server-side timer and
// via synchronous discovery triggers. See the package doc for the design stance.
type Orchestrator struct {
	store     Store
	enqueuer  DryRunEnqueuer
	confirmer DependencyConfirmer // optional; nil disables auto-confirm
	window    MaintenanceWindow   // optional; nil disables auto-confirm
	settings  Settings
	events    EventLogger         // optional; nil disables event emission
	clock     Clock

	interval time.Duration
	ticker   *time.Ticker
	stopChan chan struct{}

	// sweepLock serializes the auto-advance passes (auto-approve and
	// auto-confirm) so the timer tick and a synchronous discovery trigger
	// cannot run them concurrently.
	sweepLock sync.Mutex
}

// New builds an Orchestrator. interval <= 0 selects the 60s default. confirmer,
// window, and events may be nil; if confirmer or window is, auto-confirmation of
// resolved closures is disabled. If events is nil, system_events are not emitted.
func New(store Store, enqueuer DryRunEnqueuer, confirmer DependencyConfirmer, window MaintenanceWindow, settings Settings, events EventLogger, interval time.Duration) *Orchestrator {
	if interval <= 0 {
		interval = 60 * time.Second
	}
	return &Orchestrator{
		store:     store,
		enqueuer:  enqueuer,
		confirmer: confirmer,
		window:    window,
		settings:  settings,
		events:    events,
		clock:     systemClock{},
		interval:  interval,
		stopChan:  make(chan struct{}),
	}
}

// Start launches the timer sweep goroutine.
func (o *Orchestrator) Start() {
	log.Printf("[INFO] [server] [orchestrator] service_started interval=%s", o.interval)
	o.ticker = time.NewTicker(o.interval)
	go func() {
		for {
			select {
			case <-o.ticker.C:
				o.sweep()
			case <-o.stopChan:
				o.ticker.Stop()
				log.Printf("[INFO] [server] [orchestrator] service_stopped")
				return
			}
		}
	}()
}

// Stop halts the timer sweep.
func (o *Orchestrator) Stop() { close(o.stopChan) }

// emitEvent writes a system event via the injected logger, if present. No-op
// when the logger is nil so the orchestrator degrades gracefully without it.
func (o *Orchestrator) emitEvent(agentID *uuid.UUID, eventType, eventSubtype, severity, component, message string, metadata map[string]interface{}) {
	if o.events == nil {
		return
	}
	o.events.LogEvent(agentID, eventType, eventSubtype, severity, component, message, metadata)
}

// sweep runs one full reconciliation pass: auto-approve eligible pending
// packages, auto-confirm resolved closures whose dependencies were reported,
// then recover anything stuck in an active state.
func (o *Orchestrator) sweep() {
	now := o.clock.Now()
	o.runAutoAdvance(now)
	o.sweepCheckingDependencies(now)
	o.sweepInstalling(now)
	o.sweepPendingDependencies(now)
}

// runAutoAdvance runs the forward-advance passes (auto-approve, then
// auto-confirm) under the sweep lock so the timer tick and a synchronous
// discovery trigger cannot pile onto each other. If a pass is already running,
// the caller skips — the next tick will catch up.
func (o *Orchestrator) runAutoAdvance(now time.Time) {
	if !o.sweepLock.TryLock() {
		return
	}
	defer o.sweepLock.Unlock()
	o.sweepAutoApprove()
	o.sweepAutoConfirm(now)
}

// OnPackagesDiscovered is the synchronous trigger the update handler fires after
// a scan report lands, so newly-discovered pending packages are auto-approved
// without waiting for the next timer tick. Non-blocking.
func (o *Orchestrator) OnPackagesDiscovered() {
	go o.runAutoAdvance(o.clock.Now())
}

// OnDependenciesReported is the synchronous trigger the update handler fires
// after an agent reports a dependency closure (the package has just entered
// pending_dependencies). It runs the auto-confirm pass immediately so an
// eligible package mints its capability token without waiting for the next
// timer tick. Non-blocking; the pass re-reads the package from the DB, so a
// status that changed in between is handled idempotently.
func (o *Orchestrator) OnDependenciesReported(updateID uuid.UUID) {
	go func() {
		if !o.sweepLock.TryLock() {
			return
		}
		defer o.sweepLock.Unlock()
		o.sweepAutoConfirm(o.clock.Now())
	}()
}
