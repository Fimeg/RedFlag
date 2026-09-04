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

// GitlabReleases adapter — queries the GitLab API for the latest release
// of a project, via the path-based project identifier (URL-encoded).
//
// source_ref shape: "group/project" or "group/subgroup/project" (deeper
// namespaces are common on self-hosted GitLab). We URL-encode the path
// and call /api/v4/projects/{encoded}/releases/permalink/latest, which
// returns the most recent release without needing the numeric project ID.
//
// Host defaults to https://gitlab.com; override with REDFLAG_GITLAB_HOST
// for self-hosted (e.g. "https://gitlab.example.com").
//
// Auth via REDFLAG_GITLAB_TOKEN env var (personal access token, read_api
// scope sufficient). Public projects work without a token; private
// projects return 404 unauthenticated.
type GitlabReleases struct {
	host  string
	token string
}

func NewGitlabReleases() *GitlabReleases {
	host := strings.TrimRight(os.Getenv("REDFLAG_GITLAB_HOST"), "/")
	if host == "" {
		host = "https://gitlab.com"
	}
	return &GitlabReleases{host: host, token: os.Getenv("REDFLAG_GITLAB_TOKEN")}
}

func (GitlabReleases) Name() string { return "gitlab" }

type gitlabReleaseEnvelope struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	ReleasedAt  string `json:"released_at"`
	Links       struct {
		Self string `json:"self"`
	} `json:"_links"`
}

func (g GitlabReleases) Fetch(ctx context.Context, ref string) (*Release, error) {
	ref = strings.Trim(strings.TrimSpace(ref), "/")
	if ref == "" {
		return nil, fmt.Errorf("gitlab: empty source_ref (want group/project)")
	}
	if !strings.Contains(ref, "/") {
		return nil, fmt.Errorf("gitlab: source_ref %q is not a group/project path", ref)
	}
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/releases/permalink/latest", g.host, url.PathEscape(ref))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("gitlab: build request: %w", err)
	}
	req.Header.Set("User-Agent", "RedFlag/0.2 (+https://github.com/Fimeg/RedFlag)")
	req.Header.Set("Accept", "application/json")
	if g.token != "" {
		// GitLab accepts both PRIVATE-TOKEN and Authorization: Bearer.
		req.Header.Set("PRIVATE-TOKEN", g.token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("gitlab: fetch %s: %w", ref, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("gitlab: project %q not found or has no releases (private projects need REDFLAG_GITLAB_TOKEN)", ref)
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("gitlab: auth required for %q (set REDFLAG_GITLAB_TOKEN)", ref)
	default:
		return nil, fmt.Errorf("gitlab: status %d for %q", resp.StatusCode, ref)
	}

	var env gitlabReleaseEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("gitlab: decode: %w", err)
	}
	if env.TagName == "" {
		return nil, fmt.Errorf("gitlab: empty tag_name for %q", ref)
	}

	rel := &Release{Version: env.TagName, SourceURL: env.Links.Self}
	if env.ReleasedAt != "" {
		if t, err := time.Parse(time.RFC3339, env.ReleasedAt); err == nil {
			rel.PublishedAt = &t
		}
	}
	return rel, nil
}
