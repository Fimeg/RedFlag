package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/circuitbreaker"
	"github.com/Fimeg/RedFlag/server/internal/database/queries"
)

// repologyBreaker wraps repology.org fetches (SCALE-001 S8). Repology is
// best-effort alias enrichment; when it's down or slow the breaker opens and
// Refresh fails fast instead of every reconcile cycle blocking on it. A 404
// (project not found) does not count against the breaker — that's a per-slug
// data condition, not a sign repology is unhealthy.
var repologyBreaker = circuitbreaker.New("repology", circuitbreaker.Config{
	FailureThreshold: 5,
	FailureWindow:    60 * time.Second,
	OpenDuration:     30 * time.Second,
	HalfOpenAttempts: 2,
})

// RepologyBreakerStats exposes the repology breaker state for /health/tasks (OBS-001).
func RepologyBreakerStats() circuitbreaker.Stats { return repologyBreaker.GetStats() }

// RepologyCache wraps the Repology /packages endpoint to build a per-ecosystem
// alias map. The reconciler uses these aliases to match agent-reported package
// names to tracked_software entries.
type RepologyCache struct {
	queries *queries.RepologyQueries
	ttl     time.Duration
	mu      sync.Mutex
}

func NewRepologyCache(q *queries.RepologyQueries) *RepologyCache {
	return &RepologyCache{
		queries: q,
		ttl:     24 * time.Hour,
	}
}

// repoPrefix maps a Repology repo prefix to a canonical ecosystem name.
// Ordered longest-first to ensure specific prefixes win over shorter ones
// (e.g. "linuxmint" before "linux", "chocolatey" before "choco").
type repoPrefix struct {
	prefix string
	eco    string
}

var repoToEcosystem = []repoPrefix{
	{"chocolatey", "winget"},
	{"linuxmint", "apt"},
	{"opensuse", "zypper"},
	{"raspbian", "apt"},
	{"rubygems", "rubygems"},
	{"macports", "brew"},
	{"freebsd", "pkg"},
	{"flatpak", "flatpak"},
	{"openbsd", "pkg"},
	{"homebrew", "brew"},
	{"manjaro", "pacman"},
	{"netbsd", "pkg"},
	{"gentoo", "portage"},
	{"fedora", "dnf"},
	{"centos", "dnf"},
	{"debian", "apt"},
	{"ubuntu", "apt"},
	{"alpine", "apk"},
	{"crates", "cargo"},
	{"pypi", "pypi"},
	{"rocky", "dnf"},
	{"guix", "guix"},
	{"snap", "snap"},
	{"void", "xbps"},
	{"arch", "pacman"},
	{"alma", "dnf"},
	{"rhel", "dnf"},
	{"sles", "zypper"},
	{"scoop", "winget"},
	{"npm", "npm"},
	{"nix", "nix"},
	{"pop", "apt"},
}

// normalizeRepoToEcosystem maps a raw Repology repo string to a canonical ecosystem.
func normalizeRepoToEcosystem(repo string) string {
	lower := strings.ToLower(repo)
	for _, p := range repoToEcosystem {
		if strings.HasPrefix(lower, p.prefix) {
			return p.eco
		}
	}
	log.Printf("[DEBUG] [upstream] [repology] unknown_repo repo=%s", repo)
	return ""
}

// repologyPackageEntry is the shape of one element in the /packages response.
type repologyPackageEntry struct {
	Repo    string `json:"repo"`
	Name    string `json:"name"`
	Status  string `json:"status"`
	Version string `json:"version"`
}

// Refresh fetches the Repology /packages endpoint for a slug and upserts
// the ecosystem-grouped alias rows into the cache.
//
// Lock analysis: rc.mu was previously held across the entire function. The
// RepologyCache struct has no in-memory mutable state — queries and ttl are
// both set at construction and never changed. GetAliases / GetAliasesByEcosystem
// go straight to the database without touching mu. Therefore rc.mu guards
// nothing that Refresh touches: the HTTP call, JSON decode, and DB upsert all
// operate on purely local variables or the database. Holding the mutex across
// those operations serialised concurrent Refresh calls for *different* slugs
// against each other for no benefit. rc.mu is retained in the struct for
// forward-compatibility should in-memory state be added later, but Refresh
// no longer acquires it.
//
// Rate-limiting: the 500ms sleep is a politeness pause toward repology.org.
// It is preserved, but moved outside any mutex so it does not block other
// goroutines from reading cached state while we wait.
func (rc *RepologyCache) Refresh(ctx context.Context, slug string) error {
	endpoint := fmt.Sprintf("https://repology.org/api/v1/project/%s/packages", url.PathEscape(strings.ToLower(slug)))

	// Breaker-wrapped network fetch (SCALE-001 S8). DB upserts + the politeness
	// sleep stay outside the breaker. A 404 is a per-slug data condition, not a
	// repology-health failure, so it returns the not-found error to the caller
	// without tripping the breaker.
	var entries []repologyPackageEntry
	var notFound bool
	callErr := repologyBreaker.Call(func() error {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("User-Agent", "RedFlag/0.2 (+https://github.com/Fimeg/RedFlag)")
		req.Header.Set("Accept", "application/json")

		resp, err := httpClient.Do(req)
		if err != nil {
			return fmt.Errorf("fetch: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode == http.StatusNotFound {
			notFound = true
			return nil // repology is up; the project just doesn't exist
		}
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("status %d", resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
			return fmt.Errorf("decode: %w", err)
		}
		return nil
	})
	if callErr != nil {
		return fmt.Errorf("repology_cache: %w", callErr)
	}
	if notFound {
		return fmt.Errorf("repology_cache: project %q not found", slug)
	}

	ecoPkgs := make(map[string]map[string]bool)
	for _, e := range entries {
		eco := normalizeRepoToEcosystem(e.Repo)
		if eco == "" {
			continue
		}
		if _, ok := ecoPkgs[eco]; !ok {
			ecoPkgs[eco] = make(map[string]bool)
		}
		ecoPkgs[eco][e.Name] = true
	}

	for eco, nameSet := range ecoPkgs {
		names := make([]string, 0, len(nameSet))
		for n := range nameSet {
			names = append(names, n)
		}
		if err := rc.queries.UpsertRepologyAlias(slug, eco, names, nil); err != nil {
			log.Printf("[WARN] [upstream] [repology_cache] upsert_failed slug=%s eco=%s err=%v", slug, eco, err)
		}
	}

	log.Printf("[INFO] [upstream] [repology_cache] refreshed slug=%s ecosystems=%d entries=%d", slug, len(ecoPkgs), len(entries))
	// Politeness rate-limit toward repology.org: 500ms between requests per
	// caller. Kept outside any mutex so concurrent callers for different slugs
	// do not block each other during the wait.
	time.Sleep(500 * time.Millisecond)
	return nil
}

// GetAliases returns all cached package names for a slug across all ecosystems.
func (rc *RepologyCache) GetAliases(slug string) ([]string, error) {
	aliases, err := rc.queries.GetAliases(slug)
	if err != nil {
		return nil, err
	}
	var result []string
	for _, a := range aliases {
		for _, n := range a.PkgNames {
			result = append(result, n)
		}
	}
	return result, nil
}

// GetAliasesByEcosystem returns cached aliases for a specific (slug, ecosystem).
func (rc *RepologyCache) GetAliasesByEcosystem(slug, ecosystem string) ([]string, error) {
	return rc.queries.GetAliasesByEcosystem(slug, ecosystem)
}
