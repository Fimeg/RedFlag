package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strings"
)

// Repology adapter — queries https://repology.org/api/v1/project/{name},
// which returns an array of package observations across every distro /
// upstream channel Repology indexes. We collapse it to "the newest version
// observed across all sources" which is the closest analog to "canonical
// upstream latest." See https://repology.org/api/v1.
//
// source_ref is the Repology project name (lowercase, lower-kebab). For
// example "nginx", "postgresql", "node". Some projects use a normalized
// slug that differs from the package name — operators paste the Repology
// URL slug literally.
type Repology struct{}

func NewRepology() *Repology { return &Repology{} }

func (Repology) Name() string { return "repology" }

type repologyEntry struct {
	Repo        string `json:"repo"`
	Version     string `json:"version"`
	Status      string `json:"status"` // "newest" | "outdated" | "devel" | ...
	OrigVersion string `json:"origversion"`
}

func (r Repology) Fetch(ctx context.Context, ref string) (*Release, error) {
	if ref == "" {
		return nil, fmt.Errorf("repology: empty source_ref")
	}
	endpoint := fmt.Sprintf("https://repology.org/api/v1/project/%s", url.PathEscape(strings.ToLower(ref)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("repology: build request: %w", err)
	}
	req.Header.Set("User-Agent", "RedFlag/0.2 (+https://github.com/Fimeg/RedFlag)")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("repology: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("repology: project %q not found", ref)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("repology: status %d for %q", resp.StatusCode, ref)
	}

	var entries []repologyEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("repology: decode: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("repology: no observations for %q", ref)
	}

	// Repology marks the canonical winner with status "newest". Prefer it.
	// Fall back to the lexicographically-highest "devel" or "unique"; finally
	// to entries[0] (Repology pre-sorts). We don't try to be cleverer than
	// Repology itself — it already did the version comparison work.
	for _, e := range entries {
		if e.Status == "newest" && e.Version != "" {
			return &Release{
				Version:   e.Version,
				SourceURL: fmt.Sprintf("https://repology.org/project/%s/versions", url.PathEscape(strings.ToLower(ref))),
			}, nil
		}
	}
	for _, e := range entries {
		if (e.Status == "devel" || e.Status == "unique") && e.Version != "" {
			log.Printf("[INFO] [upstream] [repology] no 'newest' for %s, using %s (%s)", ref, e.Version, e.Status)
			return &Release{
				Version:   e.Version,
				SourceURL: fmt.Sprintf("https://repology.org/project/%s/versions", url.PathEscape(strings.ToLower(ref))),
			}, nil
		}
	}
	return &Release{
		Version:   entries[0].Version,
		SourceURL: fmt.Sprintf("https://repology.org/project/%s/versions", url.PathEscape(strings.ToLower(ref))),
	}, nil
}
