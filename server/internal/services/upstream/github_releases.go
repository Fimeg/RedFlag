package upstream

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

// GithubReleases adapter — queries the GitHub REST API for the latest
// published release of a repository.
//
// source_ref shape: "owner/repo" (e.g. "kubernetes/kubernetes",
// "vercel/next.js"). Anything else is rejected up front so a typo
// surfaces immediately instead of as a 404 from GitHub.
//
// Auth: REDFLAG_GITHUB_TOKEN is optional. Without a token, the
// unauthenticated rate limit is 60 req/hr per source IP, shared across
// the whole upstream syncer. With a personal-access or fine-grained
// token, the cap is 5000/hr. Operators tracking more than ~50 repos
// should set the token.
type GithubReleases struct {
	token string
}

func NewGithubReleases() *GithubReleases {
	return &GithubReleases{token: os.Getenv("REDFLAG_GITHUB_TOKEN")}
}

func (GithubReleases) Name() string { return "github" }

type githubReleaseEnvelope struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	PublishedAt string `json:"published_at"`
	HTMLURL     string `json:"html_url"`
	Draft       bool   `json:"draft"`
	Prerelease  bool   `json:"prerelease"`
}

func (g GithubReleases) Fetch(ctx context.Context, ref string) (*Release, error) {
	owner, repo, err := splitOwnerRepo(ref)
	if err != nil {
		return nil, fmt.Errorf("github: %w", err)
	}
	endpoint := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("github: build request: %w", err)
	}
	req.Header.Set("User-Agent", "RedFlag/0.2 (+https://github.com/Fimeg/RedFlag)")
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github: fetch %s/%s: %w", owner, repo, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// fallthrough below
	case http.StatusNotFound:
		return nil, fmt.Errorf("github: %s/%s has no releases (or repo is private/missing)", owner, repo)
	case http.StatusForbidden:
		if resp.Header.Get("X-RateLimit-Remaining") == "0" {
			reset := resp.Header.Get("X-RateLimit-Reset")
			if g.token == "" {
				return nil, fmt.Errorf("github: rate-limited (unauthenticated, 60/hr); reset epoch=%s; set REDFLAG_GITHUB_TOKEN", reset)
			}
			return nil, fmt.Errorf("github: rate-limited; reset epoch=%s", reset)
		}
		return nil, fmt.Errorf("github: 403 forbidden for %s/%s", owner, repo)
	default:
		return nil, fmt.Errorf("github: status %d for %s/%s", resp.StatusCode, owner, repo)
	}

	var env githubReleaseEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("github: decode: %w", err)
	}
	if env.TagName == "" {
		return nil, fmt.Errorf("github: empty tag_name for %s/%s", owner, repo)
	}

	rel := &Release{Version: env.TagName, SourceURL: env.HTMLURL}
	if env.PublishedAt != "" {
		if t, err := time.Parse(time.RFC3339, env.PublishedAt); err == nil {
			rel.PublishedAt = &t
		}
	}
	return rel, nil
}

// splitOwnerRepo validates the "owner/repo" form used by GitHub, Gitea,
// and Bitbucket adapters. Empties, missing slash, or extra slashes are
// rejected here so the HTTP fetch never sees garbage.
func splitOwnerRepo(ref string) (owner, repo string, err error) {
	ref = strings.Trim(strings.TrimSpace(ref), "/")
	if ref == "" {
		return "", "", errors.New("empty source_ref (want owner/repo)")
	}
	parts := strings.Split(ref, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("source_ref %q is not in owner/repo form", ref)
	}
	return parts[0], parts[1], nil
}
