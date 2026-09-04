package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// EndOfLife adapter — queries https://endoflife.date/api/{product}.json.
// Each entry is a release cycle; the first entry is canonical "current."
// source_ref is the endoflife.date product slug (e.g. "postgresql", "nginx",
// "nodejs", "ubuntu"). See https://endoflife.date/docs/api.
//
// What this adapter contributes over Repology: an EOL date. Most operators
// don't actually care about being one minor behind upstream — they care
// about being on a branch that's about to stop getting security fixes.
type EndOfLife struct{}

func NewEndOfLife() *EndOfLife { return &EndOfLife{} }

func (EndOfLife) Name() string { return "endoflife" }

// Cycle is what the endoflife.date API returns per release line. The "eol"
// and "releaseDate" fields can be either an ISO-8601 date string or a
// boolean (e.g. "eol": false on a still-supported branch). Capture both
// shapes via json.RawMessage and decode lazily.
type cycle struct {
	Cycle         string          `json:"cycle"`
	ReleaseDate   json.RawMessage `json:"releaseDate"`
	EOL           json.RawMessage `json:"eol"`
	Latest        string          `json:"latest"`
	LatestRelease json.RawMessage `json:"latestReleaseDate"`
}

func (EndOfLife) Fetch(ctx context.Context, ref string) (*Release, error) {
	if ref == "" {
		return nil, fmt.Errorf("endoflife: empty source_ref")
	}
	endpoint := fmt.Sprintf("https://endoflife.date/api/%s.json", url.PathEscape(strings.ToLower(ref)))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("endoflife: build request: %w", err)
	}
	req.Header.Set("User-Agent", "RedFlag/0.2 (+https://github.com/Fimeg/RedFlag)")
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("endoflife: fetch: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("endoflife: product %q not found", ref)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("endoflife: status %d for %q", resp.StatusCode, ref)
	}

	var cycles []cycle
	if err := json.NewDecoder(resp.Body).Decode(&cycles); err != nil {
		return nil, fmt.Errorf("endoflife: decode: %w", err)
	}
	if len(cycles) == 0 {
		return nil, fmt.Errorf("endoflife: empty cycle list for %q", ref)
	}

	// First entry is the newest release line. "latest" is the specific
	// version at the head of that line.
	head := cycles[0]
	version := head.Latest
	if version == "" {
		version = head.Cycle
	}

	published := decodeMaybeDate(head.LatestRelease)
	if published == nil {
		published = decodeMaybeDate(head.ReleaseDate)
	}
	eol := decodeMaybeDate(head.EOL)

	return &Release{
		Version:     version,
		PublishedAt: published,
		EOLAt:       eol,
		SourceURL:   fmt.Sprintf("https://endoflife.date/%s", url.PathEscape(strings.ToLower(ref))),
	}, nil
}

// decodeMaybeDate handles endoflife.date's quirk of returning either a
// date string or a boolean for the same field. Boolean false / true (with
// no date attached) collapse to nil — we don't know when, only that there
// is or isn't an EOL.
func decodeMaybeDate(raw json.RawMessage) *time.Time {
	if len(raw) == 0 {
		return nil
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil && s != "" {
		// API uses "2026-11-01" form most often.
		for _, layout := range []string{"2006-01-02", time.RFC3339} {
			if t, err := time.Parse(layout, s); err == nil {
				return &t
			}
		}
	}
	return nil
}
