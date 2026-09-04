package upstream

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
)

// BitbucketTags adapter — Bitbucket Cloud doesn't have a "latest release"
// endpoint the way GitHub/GitLab/Gitea do (Bitbucket Server self-hosted is
// deprecated, so we only target Cloud). We list tags and pick the highest
// by CompareVersions.
//
// source_ref shape: "workspace/repo" (e.g. "atlassian/atlassian-sdk").
//
// Auth via REDFLAG_BITBUCKET_TOKEN env var. Bitbucket Cloud accepts
// "Authorization: Bearer <token>" for app passwords and OAuth tokens.
// Public repos work without a token; rate limit is generous either way.
type BitbucketTags struct {
	token string
}

func NewBitbucketTags() *BitbucketTags {
	return &BitbucketTags{token: os.Getenv("REDFLAG_BITBUCKET_TOKEN")}
}

func (BitbucketTags) Name() string { return "bitbucket" }

type bitbucketTagEnvelope struct {
	Values []struct {
		Name   string `json:"name"`
		Date   string `json:"date"`
		Target struct {
			Date string `json:"date"`
			Hash string `json:"hash"`
		} `json:"target"`
		Links struct {
			HTML struct {
				Href string `json:"href"`
			} `json:"html"`
		} `json:"links"`
	} `json:"values"`
}

func (b BitbucketTags) Fetch(ctx context.Context, ref string) (*Release, error) {
	workspace, repo, err := splitOwnerRepo(ref)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: %w", err)
	}
	// pagelen=100 plus sort=-name as a heuristic to put recent tags on
	// page 1. We still re-sort with CompareVersions because Bitbucket's
	// sort is lex-only.
	endpoint := fmt.Sprintf("https://api.bitbucket.org/2.0/repositories/%s/%s/refs/tags?pagelen=100&sort=-name", workspace, repo)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: build request: %w", err)
	}
	req.Header.Set("User-Agent", "RedFlag/0.2 (+https://github.com/Fimeg/RedFlag)")
	req.Header.Set("Accept", "application/json")
	if b.token != "" {
		req.Header.Set("Authorization", "Bearer "+b.token)
	}

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bitbucket: fetch %s/%s: %w", workspace, repo, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("bitbucket: %s/%s not found", workspace, repo)
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("bitbucket: auth required for %s/%s (set REDFLAG_BITBUCKET_TOKEN)", workspace, repo)
	default:
		return nil, fmt.Errorf("bitbucket: status %d for %s/%s", resp.StatusCode, workspace, repo)
	}

	var env bitbucketTagEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, fmt.Errorf("bitbucket: decode: %w", err)
	}
	if len(env.Values) == 0 {
		return nil, fmt.Errorf("bitbucket: %s/%s has no tags", workspace, repo)
	}

	bestIdx := 0
	for i := 1; i < len(env.Values); i++ {
		if CompareVersions(env.Values[i].Name, env.Values[bestIdx].Name) > 0 {
			bestIdx = i
		}
	}
	best := env.Values[bestIdx]

	rel := &Release{Version: best.Name, SourceURL: best.Links.HTML.Href}
	// Bitbucket exposes the tag date under .date and the target commit's
	// date under .target.date. Prefer the tag date if present.
	for _, candidate := range []string{best.Date, best.Target.Date} {
		if candidate == "" {
			continue
		}
		if t, err := time.Parse(time.RFC3339, candidate); err == nil {
			rel.PublishedAt = &t
			break
		}
	}
	return rel, nil
}
