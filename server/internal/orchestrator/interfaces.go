// Package orchestrator advances the package-update lifecycle server-side. It
// auto-approves packages by operator policy and recovers packages stuck in
// active states, holding no state of its own — every decision is read from the
// database on each sweep, so a server restart loses nothing and the next tick
// reconciles. The DB-level guarded UPDATE from LIFECYCLE-001 makes every
// transition idempotent, so a redundant advance is a no-op.
//
// The dependencies below are declared as consumer interfaces so the package is
// unit-testable with fakes and imports neither the queries nor the handlers
// layer. The concrete wiring lives in cmd/server/main.go.
package orchestrator

import (
	"time"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
)

// Store is the persistence surface the orchestrator drives. Satisfied by
// *queries.UpdateQueries.
type Store interface {
	GetUpdateByID(id uuid.UUID) (*models.UpdateState, error)
	GetPackagesInStatus(status models.PackageStatus) ([]models.UpdateState, error)
	TransitionByID(id uuid.UUID, to models.PackageStatus, historyMeta models.JSONB) error
	BumpRetryCounter(id uuid.UUID, key string) (int, error)
}

// DryRunEnqueuer creates and signs the dry_run_update command that moves an
// approved package into checking_dependencies. Satisfied by *handlers.UpdateHandler.
type DryRunEnqueuer interface {
	EnqueueDryRun(update *models.UpdateState) error
}

// DependencyConfirmer performs the capability-path confirm step headlessly: it
// transitions a package waiting in pending_dependencies into installing and
// mints its capability token — the equivalent of an operator's
// ConfirmDependencies dashboard click. Satisfied by *handlers.UpdateHandler.
//
// It returns handled=false when the package is not capability-gated (legacy
// command path: docker/winget/Windows). Those are left for the operator, since
// the server cannot verify a legacy dependency was satisfied. An error means a
// genuine failure (the confirmer has already marked the package failed); the
// orchestrator only logs it. The caller (orchestrator) has already checked the
// maintenance window and auto-approval policy before invoking.
type DependencyConfirmer interface {
	ConfirmDependenciesAuto(update *models.UpdateState) (handled bool, err error)
}

// MaintenanceWindow reports whether installs are currently permitted. The
// orchestrator consults it before auto-confirming a dependency closure: dry-run
// is read-only and safe anytime, but advancing to installing must respect the
// operator's window (design Q1). Satisfied by *queries.MaintenanceWindowQueries.
type MaintenanceWindow interface {
	IsWithinMaintenanceWindow(now time.Time) (bool, error)
}

// Settings supplies live, operator-tunable policy and timing values. Satisfied
// by *services.SecuritySettingsService. A nil/missing value falls back to the
// supplied default, so a misconfigured deployment degrades to the safe default
// (auto-approval off) rather than failing.
type Settings interface {
	GetPolicyString(key, fallback string) string
	GetPolicyBool(key string, fallback bool) bool
	GetOperationalInt(key string, fallback int) int
}

// EventLogger persists server-side system events (orchestrator timeouts,
// workflow state changes) to the system_events table. Satisfied by
// *services.SystemEventLogger.
type EventLogger interface {
	LogEvent(agentID *uuid.UUID, eventType, eventSubtype, severity, component, message string, metadata map[string]interface{})
}

// Clock is injected so timeout logic is deterministic under test.
type Clock interface {
	Now() time.Time
}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }
