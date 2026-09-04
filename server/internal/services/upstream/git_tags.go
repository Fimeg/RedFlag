package upstream

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/storage/memory"
)

// GitTags adapter — the most general "any Git remote" source. Performs
// the equivalent of `git ls-remote --tags <url>` against an arbitrary
// Git remote (HTTPS or git://), then picks the highest tag by
// CompareVersions.
//
// source_ref shape: a clone URL — anything go-git can resolve. Examples:
//   - https://git.kernel.org/pub/scm/linux/kernel/git/torvalds/linux.git
//   - https://git.savannah.gnu.org/git/coreutils.git
//   - https://forge.caseytunturi.com/Fimeg/RedFlag.git
//
// Auth: anonymous. For private repos, prefer the dedicated github/gitea/
// gitlab/bitbucket adapters which honor their respective token env vars.
//
// Implementation note: uses go-git's in-memory storage so nothing hits
// disk. ListContext respects the syncer's 20-second per-row timeout via
// the passed-in ctx.
type GitTags struct{}

func NewGitTags() *GitTags { return &GitTags{} }

func (GitTags) Name() string { return "git" }

func (GitTags) Fetch(ctx context.Context, ref string) (*Release, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, fmt.Errorf("git: empty source_ref (want clone URL)")
	}

	remote := git.NewRemote(memory.NewStorage(), &config.RemoteConfig{
		Name: "origin",
		URLs: []string{ref},
	})

	refs, err := remote.ListContext(ctx, &git.ListOptions{})
	if err != nil {
		return nil, fmt.Errorf("git: ls-remote %s: %w", ref, err)
	}

	bestTag := ""
	for _, r := range refs {
		if !r.Name().IsTag() {
			continue
		}
		tag := r.Name().Short()
		// Skip annotated-tag peel refs (the "^{}" suffix) — they
		// represent the dereferenced commit, not a separate tag.
		if strings.HasSuffix(tag, "^{}") {
			continue
		}
		if bestTag == "" || CompareVersions(tag, bestTag) > 0 {
			bestTag = tag
		}
	}

	if bestTag == "" {
		return nil, fmt.Errorf("git: no tags found on %s", ref)
	}

	// We don't have a publish date from ls-remote (only refs and shas)
	// and a follow-up fetch-by-tag would defeat the "no disk, no clone"
	// design point. PublishedAt is left nil — endoflife/github/gitlab
	// adapters supply that field when an operator wants it.
	return &Release{Version: bestTag, SourceURL: ref}, nil
}
