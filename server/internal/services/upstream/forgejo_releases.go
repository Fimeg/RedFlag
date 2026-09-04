package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// ForgejoReleases adapter — queries a Forgejo/Gitea/Codeberg instance for the
// newest versioned release of a repository.
//
// It walks /api/v1/repos/{owner}/{repo}/releases pages and picks the highest
// v-tag by RedFlag's pragmatic version comparator rather than trusting /releases/latest, which on
// Forgejo/Gitea/Codeberg EXCLUDES prereleases. During an alpha run where every
// tag publishes as a prerelease (RedFlag itself, < v0.3.0), /releases/latest
// would freeze or return nothing — so we list and choose. Drafts, non-v* tags,
// and the rolling `nightly` tag are filtered out; nightly is republished every
// night and would otherwise win on recency. Selection mirrors profile-engine's
// latest_release walk.
//
// Prereleases: when includePrerelease is false (the plain Fetch path), releases
// flagged prerelease are skipped. When true (the syncer calls FetchRelease for
// rows with track_prereleases), they're considered alongside stable ones.
//
// source_ref shapes accepted:
//   - "host/owner/repo"          e.g. "codeberg.org/Fimeg/RedFlag"
//   - "https://host/owner/repo"  scheme honored; defaults to https
//   - "owner/repo"               2-part legacy form (source="gitea" rows): the
//     host falls back to REDFLAG_GITEA_HOST so existing entries keep working.
//
// Auth: REDFLAG_GITEA_TOKEN (Bearer) is optional — public repos need none,
// private/self-hosted instances supply a token. Codeberg accepts one too.
type ForgejoReleases struct {
	name         string
	fallbackHost string
	token        string
}

// NewForgejoReleases builds the adapter under its canonical "forgejo" name.
func NewForgejoReleases() *ForgejoReleases {
	return &ForgejoReleases{
		name:         "forgejo",
		fallbackHost: strings.TrimRight(os.Getenv("REDFLAG_GITEA_HOST"), "/"),
		token:        os.Getenv("REDFLAG_GITEA_TOKEN"),
	}
}

// NewGiteaReleases keeps the legacy constructor available while routing it to
// the Forgejo-compatible implementation. Forgejo is a Gitea fork, and the
// release API shape RedFlag uses here is shared.
func NewGiteaReleases() *ForgejoReleases {
	return NewForgejoReleases().WithName("gitea")
}

// WithName returns the same adapter configuration under an alias name. The
// registry keys by Name(), so registering the forgejo adapter twice — once as
// "forgejo", once as "gitea" — keeps legacy source="gitea" rows resolving to
// this (now prerelease-aware, list-walking) implementation.
func (f *ForgejoReleases) WithName(name string) *ForgejoReleases {
	clone := *f
	clone.name = name
	return &clone
}

func (f ForgejoReleases) Name() string { return f.name }

type forgejoReleaseEnvelope struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
}

// Fetch is the stable-only path (includePrerelease=false), satisfying
// ReleaseSource. The syncer uses FetchRelease directly for prerelease-aware rows.
func (f ForgejoReleases) Fetch(ctx context.Context, ref string) (*Release, error) {
	return f.FetchRelease(ctx, ref, false)
}

func (f ForgejoReleases) FetchRelease(ctx context.Context, ref string, includePrerelease bool) (*Release, error) {
	host, owner, repo, err := f.resolveRef(ref)
	if err != nil {
		return nil, fmt.Errorf("forgejo: %w", err)
	}

	envs, err := f.fetchReleasePages(ctx, host, owner, repo)
	if err != nil {
		return nil, err
	}

	// Pick the highest v-tag. Drafts and non-v* / nightly tags are dropped;
	// prereleases are dropped unless includePrerelease.
	var best *forgejoReleaseEnvelope
	for i := range envs {
		e := &envs[i]
		if e.Draft {
			continue
		}
		if e.Prerelease && !includePrerelease {
			continue
		}
		if !isVersionTag(e.TagName) {
			continue
		}
		if best == nil || CompareVersions(e.TagName, best.TagName) > 0 {
			best = e
		}
	}
	if best == nil {
		return nil, fmt.Errorf("forgejo: no versioned release for %s/%s (prereleases %s)", owner, repo, includeWord(includePrerelease))
	}

	rel := &Release{Version: best.TagName, SourceURL: best.HTMLURL}
	if best.PublishedAt != "" {
		if t, err := time.Parse(time.RFC3339, best.PublishedAt); err == nil {
			rel.PublishedAt = &t
		}
	}
	return rel, nil
}

func (f ForgejoReleases) fetchReleasePages(ctx context.Context, host, owner, repo string) ([]forgejoReleaseEnvelope, error) {
	const (
		pageLimit = 30
		maxPages  = 10
	)
	var all []forgejoReleaseEnvelope
	for page := 1; page <= maxPages; page++ {
		endpoint := fmt.Sprintf("%s/api/v1/repos/%s/%s/releases?limit=%d&page=%d", host, owner, repo, pageLimit, page)

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("forgejo: build request: %w", err)
		}
		req.Header.Set("User-Agent", "RedFlag/0.2 (+https://codeberg.org/Fimeg/RedFlag)")
		req.Header.Set("Accept", "application/json")
		if f.token != "" {
			req.Header.Set("Authorization", "Bearer "+f.token)
		}

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("forgejo: fetch %s/%s: %w", owner, repo, err)
		}

		switch resp.StatusCode {
		case http.StatusOK:
		case http.StatusNotFound:
			resp.Body.Close()
			return nil, fmt.Errorf("forgejo: %s/%s has no releases (or repo private/missing)", owner, repo)
		case http.StatusUnauthorized, http.StatusForbidden:
			resp.Body.Close()
			return nil, fmt.Errorf("forgejo: auth required for %s/%s (set REDFLAG_GITEA_TOKEN)", owner, repo)
		default:
			resp.Body.Close()
			return nil, fmt.Errorf("forgejo: status %d for %s/%s", resp.StatusCode, owner, repo)
		}

		var envs []forgejoReleaseEnvelope
		if err := json.NewDecoder(resp.Body).Decode(&envs); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("forgejo: decode: %w", err)
		}
		resp.Body.Close()
		all = append(all, envs...)
		if len(envs) < pageLimit {
			break
		}
	}
	return all, nil
}

func includeWord(b bool) string {
	if b {
		return "included"
	}
	return "excluded"
}

// resolveRef splits a source_ref into (host, owner, repo). Accepts a 3-part
// "host/owner/repo" (optionally scheme-prefixed) or a 2-part "owner/repo" that
// falls back to REDFLAG_GITEA_HOST. The returned host carries a scheme (https
// when none was supplied) and no trailing slash.
func (f ForgejoReleases) resolveRef(ref string) (host, owner, repo string, err error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", "", "", fmt.Errorf("empty source_ref (want host/owner/repo or owner/repo)")
	}

	scheme := "https"
	if i := strings.Index(ref, "://"); i >= 0 {
		scheme = ref[:i]
		ref = ref[i+3:]
	}
	ref = strings.Trim(ref, "/")

	parts := strings.Split(ref, "/")
	switch len(parts) {
	case 2:
		// Legacy "owner/repo" — host from env.
		if f.fallbackHost == "" {
			return "", "", "", fmt.Errorf("source_ref %q is owner/repo but REDFLAG_GITEA_HOST is not set", ref)
		}
		host = f.fallbackHost
		owner, repo = parts[0], parts[1]
	case 3:
		host = scheme + "://" + parts[0]
		owner, repo = parts[1], parts[2]
	default:
		return "", "", "", fmt.Errorf("source_ref %q is not host/owner/repo or owner/repo", ref)
	}
	if owner == "" || repo == "" {
		return "", "", "", fmt.Errorf("source_ref %q has empty owner or repo", ref)
	}
	// Validate the env-derived fallback host parses (it may lack a scheme).
	if !strings.Contains(host, "://") {
		host = "https://" + host
	}
	if _, perr := url.Parse(host); perr != nil {
		return "", "", "", fmt.Errorf("source_ref %q: bad host: %w", ref, perr)
	}
	return strings.TrimRight(host, "/"), owner, repo, nil
}

// isVersionTag reports whether tag looks like a v-prefixed numeric version
// (v1, v0.2.8.4, ...). Mirrors profile-engine's `^v\d` filter, which also drops
// the rolling `nightly` tag.
func isVersionTag(tag string) bool {
	if len(tag) < 2 || tag[0] != 'v' {
		return false
	}
	return tag[1] >= '0' && tag[1] <= '9'
}
