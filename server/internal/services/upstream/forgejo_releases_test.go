package upstream

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestForgejoReleasesStableOnlySkipsPrereleases(t *testing.T) {
	withForgejoHTTPClient(t, map[string][]forgejoReleaseEnvelope{
		"1": {
			{TagName: "v0.2.8.4", HTMLURL: "https://codeberg.org/Fimeg/RedFlag/releases/tag/v0.2.8.4", Prerelease: true},
			{TagName: "v0.2.7", HTMLURL: "https://codeberg.org/Fimeg/RedFlag/releases/tag/v0.2.7"},
		},
	})

	rel, err := NewForgejoReleases().Fetch(context.Background(), "forgejo.test/Fimeg/RedFlag")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if rel.Version != "v0.2.7" {
		t.Fatalf("Fetch() version = %q, want stable v0.2.7", rel.Version)
	}
}

func TestForgejoReleasesCanIncludePrereleases(t *testing.T) {
	withForgejoHTTPClient(t, map[string][]forgejoReleaseEnvelope{
		"1": {
			{TagName: "nightly", HTMLURL: "https://codeberg.org/Fimeg/RedFlag/releases/tag/nightly", Prerelease: true},
			{TagName: "v9.0.0", HTMLURL: "https://codeberg.org/Fimeg/RedFlag/releases/tag/v9.0.0", Draft: true},
			{TagName: "v0.2.8.4", HTMLURL: "https://codeberg.org/Fimeg/RedFlag/releases/tag/v0.2.8.4", Prerelease: true},
			{TagName: "v0.2.8.10", HTMLURL: "https://codeberg.org/Fimeg/RedFlag/releases/tag/v0.2.8.10", Prerelease: true},
		},
	})

	_, err := NewForgejoReleases().Fetch(context.Background(), "forgejo.test/Fimeg/RedFlag")
	if err == nil || !strings.Contains(err.Error(), "prereleases excluded") {
		t.Fatalf("Fetch() error = %v, want prerelease-excluded error", err)
	}

	rel, err := NewForgejoReleases().FetchRelease(context.Background(), "forgejo.test/Fimeg/RedFlag", true)
	if err != nil {
		t.Fatalf("FetchRelease(includePrerelease=true) returned error: %v", err)
	}
	if rel.Version != "v0.2.8.10" {
		t.Fatalf("FetchRelease(includePrerelease=true) version = %q, want v0.2.8.10", rel.Version)
	}
}

func TestForgejoReleasesFinalStableBeatsSameBasePrerelease(t *testing.T) {
	withForgejoHTTPClient(t, map[string][]forgejoReleaseEnvelope{
		"1": {
			{TagName: "v0.3.0-alpha.4", HTMLURL: "https://codeberg.org/Fimeg/RedFlag/releases/tag/v0.3.0-alpha.4", Prerelease: true},
			{TagName: "v0.3.0", HTMLURL: "https://codeberg.org/Fimeg/RedFlag/releases/tag/v0.3.0"},
		},
	})

	rel, err := NewForgejoReleases().FetchRelease(context.Background(), "forgejo.test/Fimeg/RedFlag", true)
	if err != nil {
		t.Fatalf("FetchRelease(includePrerelease=true) returned error: %v", err)
	}
	if rel.Version != "v0.3.0" {
		t.Fatalf("FetchRelease(includePrerelease=true) version = %q, want final stable v0.3.0", rel.Version)
	}
}

func TestForgejoReleasesReadsBeyondFirstPage(t *testing.T) {
	firstPage := make([]forgejoReleaseEnvelope, 30)
	for i := range firstPage {
		firstPage[i] = forgejoReleaseEnvelope{TagName: "nightly", HTMLURL: "https://codeberg.org/Fimeg/RedFlag/releases/tag/nightly"}
	}
	withForgejoHTTPClient(t, map[string][]forgejoReleaseEnvelope{
		"1": firstPage,
		"2": {
			{TagName: "v1.2.3", HTMLURL: "https://codeberg.org/Fimeg/RedFlag/releases/tag/v1.2.3"},
		},
	})

	rel, err := NewForgejoReleases().Fetch(context.Background(), "forgejo.test/Fimeg/RedFlag")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if rel.Version != "v1.2.3" {
		t.Fatalf("Fetch() version = %q, want v1.2.3 from page 2", rel.Version)
	}
}

func TestGiteaAliasUsesFallbackHost(t *testing.T) {
	withForgejoHTTPClient(t, map[string][]forgejoReleaseEnvelope{
		"1": {
			{TagName: "v1.0.0", HTMLURL: "https://gitea.example/releases/tag/v1.0.0"},
		},
	})
	t.Setenv("REDFLAG_GITEA_HOST", "https://gitea.example")

	rel, err := NewGiteaReleases().Fetch(context.Background(), "Fimeg/RedFlag")
	if err != nil {
		t.Fatalf("Fetch returned error: %v", err)
	}
	if rel.Version != "v1.0.0" {
		t.Fatalf("Fetch() version = %q, want v1.0.0", rel.Version)
	}
}

func withForgejoHTTPClient(t *testing.T, pages map[string][]forgejoReleaseEnvelope) {
	t.Helper()
	oldClient := httpClient
	httpClient = &http.Client{Transport: forgejoRoundTripper{t: t, pages: pages}}
	t.Cleanup(func() { httpClient = oldClient })
}

type forgejoRoundTripper struct {
	t     *testing.T
	pages map[string][]forgejoReleaseEnvelope
}

func (f forgejoRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	f.t.Helper()
	if req.URL.Path != "/api/v1/repos/Fimeg/RedFlag/releases" {
		f.t.Fatalf("path = %q, want /api/v1/repos/Fimeg/RedFlag/releases", req.URL.Path)
	}
	if req.URL.Query().Get("limit") != "30" {
		f.t.Fatalf("limit = %q, want 30", req.URL.Query().Get("limit"))
	}
	page := req.URL.Query().Get("page")
	releases := f.pages[page]
	body, err := json.Marshal(releases)
	if err != nil {
		f.t.Fatalf("marshal releases: %v", err)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(strings.NewReader(string(body))),
		Request:    req,
	}, nil
}
