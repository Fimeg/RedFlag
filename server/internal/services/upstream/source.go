// Package upstream tracks canonical upstream releases for software the
// operator has chosen to monitor. The category name in the wild is
// "upstream release monitoring" — see Anitya (release-monitoring.org),
// Repology, nvchecker, and endoflife.date.
//
// Each upstream registry is wrapped behind ReleaseSource so the syncer can
// dispatch by source name without knowing the registry's wire format.
// Fail-open semantics mirror OSV and package-age: if a fetch fails we
// surface the error on tracked_software.last_error but do NOT mutate
// latest_version (stale-is-better-than-wrong).
package upstream

import (
	"context"
	"sync"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/httpx"
)

// Release is the normalized shape every adapter returns. Source-specific
// shape lives inside each adapter and is collapsed here.
type Release struct {
	Version     string
	PublishedAt *time.Time
	EOLAt       *time.Time // nil unless the source supplies one (endoflife.date)
	SourceURL   string
}

// ReleaseSource is the pluggable contract every registry adapter implements.
// Name() must be stable — it's the value stored in tracked_software.source
// and used to look up the adapter at sync time.
type ReleaseSource interface {
	Name() string
	Fetch(ctx context.Context, ref string) (*Release, error)
}

// PrereleaseAwareSource is an optional capability some adapters implement when
// their registry distinguishes prereleases from stable releases. The syncer
// type-asserts for it and, for rows that opted into track_prereleases, calls
// FetchRelease(ctx, ref, true) so prereleases are considered when picking the
// latest version. Adapters that don't implement it keep the plain Fetch path
// (stable-only); for an adapter that does, Fetch must equal
// FetchRelease(ctx, ref, false). The base ReleaseSource.Fetch signature stays
// fixed so the other adapters need no changes.
type PrereleaseAwareSource interface {
	ReleaseSource
	FetchRelease(ctx context.Context, ref string, includePrerelease bool) (*Release, error)
}

// Registry holds the live adapters. Adapters self-register at construction
// time so main.go composes the set explicitly.
type Registry struct {
	mu      sync.RWMutex
	sources map[string]ReleaseSource
}

func NewRegistry() *Registry {
	return &Registry{sources: make(map[string]ReleaseSource)}
}

func (r *Registry) Register(s ReleaseSource) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.sources[s.Name()] = s
}

func (r *Registry) Get(name string) (ReleaseSource, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	s, ok := r.sources[name]
	return s, ok
}

func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.sources))
	for n := range r.sources {
		names = append(names, n)
	}
	return names
}

// httpClient is shared across adapters so connection pooling holds and one
// adapter can't starve the others on a slow registry.
var httpClient = httpx.NewClient(15 * time.Second)
