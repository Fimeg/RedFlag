package orchestrator

import (
	"errors"
	"testing"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
)

// --- fakes -----------------------------------------------------------------

type transitionCall struct {
	id   uuid.UUID
	to   models.PackageStatus
	meta models.JSONB
}

type fakeStore struct {
	byStatus    map[models.PackageStatus][]models.UpdateState
	byID        map[uuid.UUID]*models.UpdateState
	counters    map[uuid.UUID]int
	transitions []transitionCall
	failBump    bool
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		byStatus: map[models.PackageStatus][]models.UpdateState{},
		byID:     map[uuid.UUID]*models.UpdateState{},
		counters: map[uuid.UUID]int{},
	}
}

func (f *fakeStore) add(p models.UpdateState) {
	f.byStatus[p.Status] = append(f.byStatus[p.Status], p)
	cp := p
	f.byID[p.ID] = &cp
}

func (f *fakeStore) GetUpdateByID(id uuid.UUID) (*models.UpdateState, error) {
	if p, ok := f.byID[id]; ok {
		return p, nil
	}
	return nil, errors.New("not found")
}

func (f *fakeStore) GetPackagesInStatus(status models.PackageStatus) ([]models.UpdateState, error) {
	return f.byStatus[status], nil
}

func (f *fakeStore) TransitionByID(id uuid.UUID, to models.PackageStatus, meta models.JSONB) error {
	f.transitions = append(f.transitions, transitionCall{id: id, to: to, meta: meta})
	if p, ok := f.byID[id]; ok {
		p.Status = to
	}
	return nil
}

func (f *fakeStore) BumpRetryCounter(id uuid.UUID, key string) (int, error) {
	if f.failBump {
		return 0, errors.New("bump failed")
	}
	f.counters[id]++
	return f.counters[id], nil
}

func (f *fakeStore) transitionsTo(to models.PackageStatus) int {
	n := 0
	for _, t := range f.transitions {
		if t.to == to {
			n++
		}
	}
	return n
}

type fakeEnqueuer struct {
	calls int
	err   error
}

func (f *fakeEnqueuer) EnqueueDryRun(update *models.UpdateState) error {
	f.calls++
	return f.err
}

type fakeConfirmer struct {
	calls   int
	handled bool
	err     error
	seen    []uuid.UUID
}

func (f *fakeConfirmer) ConfirmDependenciesAuto(update *models.UpdateState) (bool, error) {
	f.calls++
	f.seen = append(f.seen, update.ID)
	return f.handled, f.err
}

type fakeWindow struct {
	open bool
	err  error
}

func (f fakeWindow) IsWithinMaintenanceWindow(time.Time) (bool, error) { return f.open, f.err }

type fakeSettings struct {
	policyStr  map[string]string
	policyBool map[string]bool
	opInt      map[string]int
}

func (f fakeSettings) GetPolicyString(k, d string) string {
	if v, ok := f.policyStr[k]; ok {
		return v
	}
	return d
}
func (f fakeSettings) GetPolicyBool(k string, d bool) bool {
	if v, ok := f.policyBool[k]; ok {
		return v
	}
	return d
}
func (f fakeSettings) GetOperationalInt(k string, d int) int {
	if v, ok := f.opInt[k]; ok {
		return v
	}
	return d
}

func newOrch(store Store, enq DryRunEnqueuer, set Settings, now time.Time) *Orchestrator {
	return &Orchestrator{store: store, enqueuer: enq, settings: set, clock: fixedClock{now}, interval: time.Minute, stopChan: make(chan struct{})}
}

func newConfirmOrch(store Store, conf DependencyConfirmer, win MaintenanceWindow, set Settings, now time.Time) *Orchestrator {
	return &Orchestrator{store: store, confirmer: conf, window: win, settings: set, clock: fixedClock{now}, interval: time.Minute, stopChan: make(chan struct{})}
}

type fixedClock struct{ t time.Time }

func (c fixedClock) Now() time.Time { return c.t }

func pkg(status models.PackageStatus, severity string, age time.Duration, now time.Time) models.UpdateState {
	return models.UpdateState{
		ID:            uuid.Must(uuid.NewV4()),
		AgentID:       uuid.Must(uuid.NewV4()),
		PackageType:   "dnf",
		PackageName:   "demo",
		Severity:      severity,
		Status:        status,
		LastUpdatedAt: now.Add(-age),
	}
}

// --- policy ----------------------------------------------------------------

func TestAutoApproveCeiling(t *testing.T) {
	cases := map[string]int{
		"":         0,
		"off":      0,
		"none":     0,
		"disabled": 0,
		"garbage":  0, // unrecognized must fail safe to disabled
		"low":      1,
		"high":     4,
		"critical": 5,
		"CRITICAL": 5,
	}
	for in, want := range cases {
		if got := autoApproveCeiling(in); got != want {
			t.Errorf("autoApproveCeiling(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestShouldAutoApprove(t *testing.T) {
	now := time.Now().UTC()
	base := pkg(models.StatusPending, "moderate", 0, now)

	if !shouldAutoApprove(base, "high") {
		t.Error("moderate package should auto-approve under high ceiling")
	}
	if shouldAutoApprove(base, "low") {
		t.Error("moderate package must not auto-approve under low ceiling")
	}
	if shouldAutoApprove(base, "off") {
		t.Error("nothing auto-approves when policy is off")
	}

	withVulns := base
	withVulns.Metadata = models.JSONB{"supply_chain_vulns": []interface{}{map[string]interface{}{"id": "CVE-1"}}}
	if shouldAutoApprove(withVulns, "critical") {
		t.Error("package with vulns must never auto-approve regardless of ceiling")
	}

	unknownSev := pkg(models.StatusPending, "spicy", 0, now)
	if shouldAutoApprove(unknownSev, "critical") {
		t.Error("unknown severity must not auto-approve")
	}
}

// --- auto-approve sweep ----------------------------------------------------

func TestSweepAutoApprove_ApprovesEligibleAndEnqueues(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore()
	store.add(pkg(models.StatusPending, "low", 0, now))      // eligible
	store.add(pkg(models.StatusPending, "critical", 0, now)) // too severe for "high"
	enq := &fakeEnqueuer{}
	set := fakeSettings{policyStr: map[string]string{"auto_approve_max_severity": "high"}}

	o := newOrch(store, enq, set, now)
	o.sweepAutoApprove()

	if got := store.transitionsTo(models.StatusApproved); got != 1 {
		t.Errorf("approved transitions = %d, want 1", got)
	}
	if enq.calls != 1 {
		t.Errorf("dry-run enqueues = %d, want 1", enq.calls)
	}
}

func TestSweepAutoApprove_DisabledByDefault(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore()
	store.add(pkg(models.StatusPending, "low", 0, now))
	enq := &fakeEnqueuer{}
	set := fakeSettings{} // no policy set → default "off"

	o := newOrch(store, enq, set, now)
	o.sweepAutoApprove()

	if len(store.transitions) != 0 || enq.calls != 0 {
		t.Errorf("auto-approve must be a no-op by default: transitions=%d enqueues=%d", len(store.transitions), enq.calls)
	}
}

func TestSweepAutoApprove_RespectsAllowDryRunsFalse(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore()
	store.add(pkg(models.StatusPending, "low", 0, now))
	enq := &fakeEnqueuer{}
	set := fakeSettings{
		policyStr:  map[string]string{"auto_approve_max_severity": "high"},
		policyBool: map[string]bool{"allow_dry_runs": false},
	}

	o := newOrch(store, enq, set, now)
	o.sweepAutoApprove()

	if len(store.transitions) != 0 || enq.calls != 0 {
		t.Errorf("auto-approve must not run when dry-runs are disabled: transitions=%d enqueues=%d", len(store.transitions), enq.calls)
	}
}

// --- auto-confirm sweep ----------------------------------------------------

func confirmSettings() fakeSettings {
	return fakeSettings{policyStr: map[string]string{"auto_approve_max_severity": "high"}}
}

// clearedPkg is a pending_dependencies package whose closure has been OSV-checked
// and came back clean — the precondition for auto-confirmation.
func clearedPkg(severity string, now time.Time) models.UpdateState {
	p := pkg(models.StatusPendingDependencies, severity, 0, now)
	p.Metadata = models.JSONB{
		"closure_checked_at": now.Format(time.RFC3339),
		"closure_vulns":      "[]",
	}
	return p
}

func TestSweepAutoConfirm_ConfirmsEligibleInsideWindow(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore()
	store.add(clearedPkg("low", now))      // eligible: cleared closure, under ceiling
	store.add(clearedPkg("critical", now)) // too severe for "high"
	conf := &fakeConfirmer{handled: true}

	o := newConfirmOrch(store, conf, fakeWindow{open: true}, confirmSettings(), now)
	o.sweepAutoConfirm(now)

	if conf.calls != 1 {
		t.Errorf("confirm calls = %d, want 1 (only the eligible package)", conf.calls)
	}
}

func TestSweepAutoConfirm_HoldsUncheckedClosure(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore()
	// Eligible by severity/window, but the closure was never OSV-checked (no
	// closure_checked_at) — fail-closed, must not auto-mint.
	store.add(pkg(models.StatusPendingDependencies, "low", 0, now))
	conf := &fakeConfirmer{handled: true}

	o := newConfirmOrch(store, conf, fakeWindow{open: true}, confirmSettings(), now)
	o.sweepAutoConfirm(now)

	if conf.calls != 0 {
		t.Errorf("must not auto-confirm an unchecked closure, got %d calls", conf.calls)
	}
}

func TestSweepAutoConfirm_BlocksVulnerableClosure(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore()
	p := clearedPkg("low", now)
	p.Metadata["closure_vulns"] = `[{"name":"libxml2","version":"2.9.0","vulns":[{"id":"CVE-9"}]}]`
	store.add(p)
	conf := &fakeConfirmer{handled: true}

	o := newConfirmOrch(store, conf, fakeWindow{open: true}, confirmSettings(), now)
	o.sweepAutoConfirm(now)

	if conf.calls != 0 {
		t.Errorf("must not auto-confirm a closure with vulns, got %d calls", conf.calls)
	}
}

func TestSweepAutoConfirm_SkipsOutsideWindow(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore()
	store.add(pkg(models.StatusPendingDependencies, "low", 0, now))
	conf := &fakeConfirmer{handled: true}

	o := newConfirmOrch(store, conf, fakeWindow{open: false}, confirmSettings(), now)
	o.sweepAutoConfirm(now)

	if conf.calls != 0 {
		t.Errorf("must not confirm outside the maintenance window, got %d calls", conf.calls)
	}
}

func TestSweepAutoConfirm_FailClosedOnWindowError(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore()
	store.add(pkg(models.StatusPendingDependencies, "low", 0, now))
	conf := &fakeConfirmer{handled: true}

	o := newConfirmOrch(store, conf, fakeWindow{err: errors.New("db down")}, confirmSettings(), now)
	o.sweepAutoConfirm(now)

	if conf.calls != 0 {
		t.Errorf("must not confirm when the window check errors, got %d calls", conf.calls)
	}
}

func TestSweepAutoConfirm_DisabledByDefault(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore()
	store.add(pkg(models.StatusPendingDependencies, "low", 0, now))
	conf := &fakeConfirmer{handled: true}

	o := newConfirmOrch(store, conf, fakeWindow{open: true}, fakeSettings{}, now) // policy off
	o.sweepAutoConfirm(now)

	if conf.calls != 0 {
		t.Errorf("auto-confirm must be a no-op when auto-approval is off, got %d calls", conf.calls)
	}
}

func TestSweepAutoConfirm_NoConfirmerIsNoOp(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore()
	store.add(pkg(models.StatusPendingDependencies, "low", 0, now))

	// confirmer + window nil (unwired, e.g. minter disabled)
	o := &Orchestrator{store: store, settings: confirmSettings(), clock: fixedClock{now}, interval: time.Minute, stopChan: make(chan struct{})}
	o.sweepAutoConfirm(now) // must not panic
}

// --- stuck-state recovery --------------------------------------------------

func TestSweepCheckingDependencies_RequeueThenFail(t *testing.T) {
	now := time.Now().UTC()
	stuck := pkg(models.StatusCheckingDependencies, "high", 45*time.Minute, now) // past 30m threshold
	store := newFakeStore()
	store.add(stuck)
	enq := &fakeEnqueuer{}
	set := fakeSettings{} // defaults: 30m threshold, 2 retries

	o := newOrch(store, enq, set, now)

	// Attempts 1 and 2 re-enqueue the dry-run; no failure yet.
	o.sweepCheckingDependencies(now)
	o.sweepCheckingDependencies(now)
	if enq.calls != 2 {
		t.Fatalf("expected 2 re-enqueues, got %d", enq.calls)
	}
	if store.transitionsTo(models.StatusFailed) != 0 {
		t.Fatalf("must not fail before retry budget is spent")
	}

	// Attempt 3 exceeds the budget → fail.
	o.sweepCheckingDependencies(now)
	if enq.calls != 2 {
		t.Errorf("must not re-enqueue after budget spent, got %d calls", enq.calls)
	}
	if store.transitionsTo(models.StatusFailed) != 1 {
		t.Errorf("expected package failed after budget, got %d failed transitions", store.transitionsTo(models.StatusFailed))
	}
}

func TestSweepCheckingDependencies_SkipsFreshPackages(t *testing.T) {
	now := time.Now().UTC()
	fresh := pkg(models.StatusCheckingDependencies, "high", 5*time.Minute, now) // under 30m
	store := newFakeStore()
	store.add(fresh)
	enq := &fakeEnqueuer{}
	o := newOrch(store, enq, fakeSettings{}, now)

	o.sweepCheckingDependencies(now)
	if enq.calls != 0 || len(store.transitions) != 0 {
		t.Errorf("fresh package must be untouched: enqueues=%d transitions=%d", enq.calls, len(store.transitions))
	}
}

func TestSweepInstalling_FailsStuck(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStore()
	store.add(pkg(models.StatusInstalling, "high", 90*time.Minute, now)) // past 60m
	store.add(pkg(models.StatusInstalling, "high", 10*time.Minute, now)) // fresh
	o := newOrch(store, &fakeEnqueuer{}, fakeSettings{}, now)

	o.sweepInstalling(now)
	if got := store.transitionsTo(models.StatusFailed); got != 1 {
		t.Errorf("expected exactly the stuck install to fail, got %d", got)
	}
}
