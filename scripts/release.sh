#!/usr/bin/env bash
# Guided release for RedFlag. Checks everything, asks before everything.
#
#   scripts/release.sh              # walks you through, suggests versions
#   scripts/release.sh 0.2.8.0      # same, with the target version given
#
# What it verifies before anything is tagged or pushed:
#   branch == public, tree clean, local == remote, version lockstep across
#   versions.go / docker-compose.yml / Cargo.toml / CHANGELOG, tag is new and
#   sorts above every existing tag, Gitea is reachable, Actions is enabled,
#   a runner with the right label is registered, the public-forge publisher
#   secret exists, and the dedicated SSH release key can sign and verify.
#   Every mutation is shown first and confirmed.

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
# The LAN Gitea remote is named "origin" on current checkouts, "gitea-local"
# on older ones. Take whichever exists; override with RELEASE_REMOTE=<name>.
REMOTE="${RELEASE_REMOTE:-$(git -C "$ROOT" remote | grep -qx gitea-local && echo gitea-local || echo origin)}"
# The API address is installation-local. Keep it in the environment or this
# checkout's config; neither belongs in reachable public history.
GITEA="${GITEA_URL:-$(git -C "$ROOT" config --get redflag.giteaUrl || true)}"

bold()  { printf '\033[1m%s\033[0m\n' "$*"; }
ok()    { printf '  \033[32m✓\033[0m %s\n' "$*"; }
warn()  { printf '  \033[33m!\033[0m %s\n' "$*"; }
fail()  { printf '  \033[31m✗\033[0m %s\n' "$*"; }

ask() {  # ask "question" -> returns 0 on yes
  local reply
  read -rp "  → $1 [y/N] " reply
  [[ "$reply" =~ ^[yY]$ ]]
}

abort() { echo; echo "Aborted. Nothing was pushed."; exit 1; }

[ -n "$GITEA" ] || {
  fail "No Gitea API URL (set GITEA_URL or git config redflag.giteaUrl)"
  exit 1
}

# ---------------------------------------------------------------- step 0: gitea
bold "[0/6] Gitea connection"

REMOTE_URL=$(git -C "$ROOT" remote get-url "$REMOTE")
# Git transport and API authentication are separate. The current remote is
# SSH, while Actions lives on the LAN HTTP endpoint. Ask git's credential
# helper for the API token instead of assuming a secret is embedded in a URL.
credential_protocol=${GITEA%%://*}
credential_host=${GITEA#*://}
credential_host=${credential_host%%/*}
credential=
TOKEN=${GITEA_TOKEN:-}
if [ -z "$TOKEN" ]; then
  credential=$(printf 'protocol=%s\nhost=%s\n\n' \
    "$credential_protocol" "$credential_host" \
    | GIT_TERMINAL_PROMPT=0 git credential fill 2>/dev/null) || credential=
  TOKEN=$(printf '%s\n' "$credential" | sed -n 's/^password=//p' | head -1)
fi
if [ -z "$TOKEN" ]; then
  TOKEN=$(printf '%s\n' "$credential" \
    | sed -n 's/^username=\([0-9a-f]\{40\}\)$/\1/p' | head -1)
fi
[ -n "$TOKEN" ] || { fail "No API token for $GITEA (set GITEA_TOKEN or configure git credential)"; exit 1; }

case "$REMOTE_URL" in
  http://*|https://*|ssh://*) remote_path=${REMOTE_URL#*://}; remote_path=${remote_path#*/} ;;
  *:*) remote_path=${REMOTE_URL#*:} ;;
  *) remote_path=$REMOTE_URL ;;
esac
OWNER_REPO=${remote_path%.git}
OWNER_REPO=${OWNER_REPO#/}
API="$GITEA/api/v1"

if ! GITEA_VER=$(curl -sf --max-time 5 -H "Authorization: token $TOKEN" "$API/version" | grep -oP '"version":\s*"\K[^"]+'); then
  fail "Gitea unreachable at $GITEA — is the box up?"
  exit 1
fi
ok "Gitea $GITEA_VER at $GITEA ($OWNER_REPO)"

if [ "$(curl -s -H "Authorization: token $TOKEN" "$API/repos/$OWNER_REPO" | grep -oP '"has_actions":\s*\K(true|false)')" != "true" ]; then
  fail "Actions is disabled on $OWNER_REPO — enable it in repo Settings → Units."
  exit 1
fi
ok "Actions enabled on the repo"

# ------------------------------------------------------------- step 1: runners
bold "[1/6] Runner + secret preflight"

RUNNERS_JSON=$(curl -s -H "Authorization: token $TOKEN" "$API/admin/actions/runners" 2>/dev/null || echo "")
RUNNER_COUNT=$(echo "$RUNNERS_JSON" | grep -oP '"total_count":\s*\K[0-9]+' || echo 0)
if [ "${RUNNER_COUNT:-0}" -eq 0 ]; then
  fail "No Actions runner is registered on this Gitea instance."
  echo "    A tag push will queue the release workflow forever — nothing will run it."
  echo "    To register one (on any docker-capable box that can reach $GITEA):"
  echo "      1. Get a registration token: $GITEA/-/admin/actions/runners"
  echo "      2. docker run -d --name act_runner --restart always \\"
  echo "           -v /var/run/docker.sock:/var/run/docker.sock \\"
  echo "           -e GITEA_INSTANCE_URL=$GITEA \\"
  echo "           -e GITEA_RUNNER_REGISTRATION_TOKEN=<token> \\"
  echo "           docker.io/gitea/act_runner:latest"
  echo "    The default runner config carries the 'ubuntu-latest' label the workflows need."
  ask "Continue anyway (tag will sit queued until a runner exists)?" || abort
else
  ok "$RUNNER_COUNT runner(s) registered"
  # The workflows declare runs-on labels; make sure at least one runner carries each.
  NEEDED=$(grep -rhoP 'runs-on:\s*\K\S+' "$ROOT/.gitea/workflows/" | sort -u)
  for label in $NEEDED; do
    if echo "$RUNNERS_JSON" | grep -q "\"$label\""; then
      ok "runner label '$label' available"
    else
      warn "no runner advertises label '$label' — jobs declaring it will never start"
    fi
  done
fi

if curl -s -H "Authorization: token $TOKEN" "$API/repos/$OWNER_REPO/actions/secrets" | grep -q '"PUBLIC_FORGE_TOKEN"'; then
  ok "repo secret PUBLIC_FORGE_TOKEN exists"
else
  fail "repo secret PUBLIC_FORGE_TOKEN is missing — public tag and release publication will fail."
  echo "    Install the dedicated public-forge publisher token in repo Actions secrets."
  exit 1
fi

if [ "$(git -C "$ROOT" config --get gpg.format)" != "ssh" ]; then
  fail "git gpg.format must be ssh for RedFlag release tags"
  exit 1
fi
SIGNING_KEY=$(git -C "$ROOT" config --get user.signingkey || true)
[ -n "$SIGNING_KEY" ] && [ -r "$SIGNING_KEY" ] || { fail "configured release signing key is missing or unreadable"; exit 1; }
[ -r "$ROOT/.gitea/allowed_signers" ] || { fail "release allowed-signers file is missing"; exit 1; }
ok "dedicated SSH release signing key is available"

# ------------------------------------------------------------ step 2: git state
bold "[2/6] Git state"

BRANCH=$(git -C "$ROOT" branch --show-current)
if [ "$BRANCH" != "public" ]; then
  fail "On branch '$BRANCH' — releases come from public."
  abort
fi
ok "on public"

git -C "$ROOT" fetch "$REMOTE" public --tags --quiet
AHEAD=$(git -C "$ROOT" rev-list --count "$REMOTE/public..public")
BEHIND=$(git -C "$ROOT" rev-list --count "public..$REMOTE/public")
[ "$BEHIND" -gt 0 ] && { fail "public is $BEHIND commit(s) behind $REMOTE/public — pull/rebase first."; abort; }
[ "$AHEAD" -gt 0 ] && warn "public is $AHEAD commit(s) ahead of $REMOTE/public (they push with the tag)"
[ "$AHEAD" -eq 0 ] && ok "public matches $REMOTE/public"

if ! git -C "$ROOT" diff --quiet || ! git -C "$ROOT" diff --cached --quiet; then
  warn "uncommitted changes:"
  git -C "$ROOT" status --short | sed 's/^/      /' | head -20
  UNTRACKED=$(git -C "$ROOT" status --short | grep -c '^??' || true)
  [ "$UNTRACKED" -gt 0 ] && warn "$UNTRACKED untracked file(s) above will NOT be in the release unless added"
  if ask "Commit everything (git add -A) before tagging?"; then
    read -rp "  → Commit message: " MSG
    [ -z "$MSG" ] && { fail "empty commit message"; abort; }
    git -C "$ROOT" add -A
    git -C "$ROOT" commit -m "$MSG"
    ok "committed: $MSG"
  else
    ask "Tag and release WITHOUT the uncommitted changes?" || abort
  fi
else
  ok "working tree clean"
fi

# -------------------------------------------------------------- step 3: version
bold "[3/6] Version lockstep"

V_SERVER=$(grep -P '^\s*AgentVersion\s*=' "$ROOT/server/internal/version/versions.go" | grep -oP '"\K[^"]+')
V_CONFIG=$(grep -P '^\s*ConfigVersion\s*=' "$ROOT/server/internal/version/versions.go" | grep -oP '"\K[^"]+')
V_COMPOSE=$(grep -oP '(?<=BUILD_VERSION:-)[0-9]+(\.[0-9]+){3}' "$ROOT/docker-compose.yml")
V_CARGO=$(grep -m1 '^version' "$ROOT/helper/Cargo.toml" | cut -d'"' -f2)
V_TAG=$(git -C "$ROOT" tag --list 'v*' --sort=-v:refname | head -1 | sed 's/^v//')

echo "      versions.go (Agent):   $V_SERVER"
echo "      versions.go (Config):  $V_CONFIG"
echo "      docker-compose.yml:    $V_COMPOSE"
echo "      helper/Cargo.toml:     $V_CARGO"
echo "      highest existing tag:  ${V_TAG:-none}"

NEW_VERSION="${1:-}"
if [ -z "$NEW_VERSION" ]; then
  if [ "$V_SERVER" = "$V_COMPOSE" ] && [ "$V_SERVER" != "${V_TAG:-}" ]; then
    echo
    echo "  Tree is already bumped to $V_SERVER (no tag for it yet)."
    ask "Release v$V_SERVER?" && NEW_VERSION="$V_SERVER"
  fi
  if [ -z "$NEW_VERSION" ]; then
    IFS='.' read -r a b c d <<< "$V_COMPOSE"
    echo
    echo "  Suggested bumps from $V_COMPOSE:"
    echo "      patch:  $a.$b.$c.$((d+1))"
    echo "      minor:  $a.$b.$((c+1)).0"
    echo "      major:  $a.$((b+1)).0.0"
    read -rp "  → New version: " NEW_VERSION
  fi
fi

[[ "$NEW_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]] || { fail "version must be N.N.N.N (got: $NEW_VERSION)"; abort; }

# Bump the tree if it isn't already at the target.
if [ "$V_SERVER" != "$NEW_VERSION" ] || [ "$V_COMPOSE" != "$NEW_VERSION" ]; then
  echo
  warn "tree is not at $NEW_VERSION — running bump-version.sh"
  "$ROOT/scripts/bump-version.sh" "$NEW_VERSION"
  if ask "Commit the bump as 'v$NEW_VERSION'?"; then
    git -C "$ROOT" add -A
    git -C "$ROOT" commit -m "v$NEW_VERSION"
    ok "committed v$NEW_VERSION"
  else
    fail "bump is uncommitted — the tag would point at a tree without it."
    abort
  fi
fi

# Same checks the CI gate runs — catch it here, not after the push.
LOCKSTEP_FAIL=0
for pair in "AgentVersion=$(grep -P '^\s*AgentVersion\s*=' "$ROOT/server/internal/version/versions.go" | grep -oP '"\K[^"]+')" \
            "ConfigVersion=$(grep -P '^\s*ConfigVersion\s*=' "$ROOT/server/internal/version/versions.go" | grep -oP '"\K[^"]+')" \
            "docker-compose=$(grep -oP '(?<=BUILD_VERSION:-)[0-9]+(\.[0-9]+){3}' "$ROOT/docker-compose.yml")"; do
  name="${pair%%=*}"; val="${pair#*=}"
  if [ "$val" != "$NEW_VERSION" ]; then fail "$name is $val, expected $NEW_VERSION"; LOCKSTEP_FAIL=1; fi
done
CARGO_NOW=$(grep -m1 '^version' "$ROOT/helper/Cargo.toml" | cut -d'"' -f2)
[ "$CARGO_NOW" != "$(echo "$NEW_VERSION" | cut -d. -f1-3)" ] && { fail "Cargo.toml is $CARGO_NOW, expected $(echo "$NEW_VERSION" | cut -d. -f1-3)"; LOCKSTEP_FAIL=1; }
[ "$LOCKSTEP_FAIL" -eq 1 ] && abort
ok "all version sources agree on $NEW_VERSION"

if git -C "$ROOT" rev-parse "v$NEW_VERSION" >/dev/null 2>&1; then
  fail "tag v$NEW_VERSION already exists"
  abort
fi
HIGHEST=$(printf 'v%s\nv%s\n' "${V_TAG:-0.0.0.0}" "$NEW_VERSION" | sort -V | tail -1)
[ "$HIGHEST" != "v$NEW_VERSION" ] && { fail "v$NEW_VERSION does not sort above v$V_TAG — versions move forward only"; abort; }
ok "v$NEW_VERSION is new and sorts above v${V_TAG:-none}"

# ------------------------------------------------------------ step 4: changelog
bold "[4/6] CHANGELOG"

if grep -q "$NEW_VERSION" "$ROOT/CHANGELOG.md" 2>/dev/null; then
  ok "CHANGELOG.md has an entry for $NEW_VERSION"
else
  fail "no CHANGELOG.md entry for $NEW_VERSION — the CI gate will reject the tag."
  echo "    Add the entry, commit it, and rerun. (This script stops here on purpose:"
  echo "    a release without release notes is the 'what's new?' gap.)"
  abort
fi

# ----------------------------------------------------------- step 5: tag + push
bold "[5/6] Tag and push"

echo "  About to run:"
echo "      git tag -s -m 'v$NEW_VERSION' v$NEW_VERSION"
echo "      git push $REMOTE public v$NEW_VERSION"
echo "  The tag push triggers the release workflow: gate → build → verify → publish."
ask "Proceed?" || abort

git -C "$ROOT" tag -s -m "v$NEW_VERSION" "v$NEW_VERSION"
git -C "$ROOT" config gpg.ssh.allowedSignersFile .gitea/allowed_signers
git -C "$ROOT" tag -v "v$NEW_VERSION"
# Push only this release's tag. --tags would ship every local tag and chokes
# on this checkout's divergent legacy tags (v0.2.8.2 had to be cut by hand).
git -C "$ROOT" push "$REMOTE" public "v$NEW_VERSION"
ok "pushed public + v$NEW_VERSION to $REMOTE"
RELEASE_SHA=$(git -C "$ROOT" rev-parse "v$NEW_VERSION^{commit}")

# ------------------------------------------------------------- step 6: watch CI
bold "[6/6] Watching the workflow"

echo "  Polling for the release run (90s max) ..."
for i in $(seq 1 15); do
  sleep 6
  if ! RUNS=$(curl -fsS -H "Authorization: token $TOKEN" "$API/repos/$OWNER_REPO/actions/runs?limit=20"); then
    fail "Actions API became unreachable while watching v$NEW_VERSION"
    exit 2
  fi
  RUN=$(printf '%s' "$RUNS" | python3 -c '
import json, sys
tag, sha = sys.argv[1:]
data = json.load(sys.stdin)
runs = data.get("workflow_runs", data) if isinstance(data, dict) else data
for run in runs:
    if run.get("head_sha") == sha and (run.get("head_branch") == tag or run.get("name") == "release"):
        print(run.get("id", ""), run.get("conclusion") or run.get("status") or "unknown")
        break
' "v$NEW_VERSION" "$RELEASE_SHA")
  read -r RUN_ID STATUS <<< "$RUN"
  if [ -n "$STATUS" ]; then
    echo "      run ${RUN_ID:-unknown} for v$NEW_VERSION → $STATUS"
    case "$STATUS" in
      success) ok "release pipeline finished"; break ;;
      failure) fail "pipeline failed — see $GITEA/$OWNER_REPO/actions"; exit 1 ;;
      cancelled|canceled) fail "pipeline was cancelled"; exit 1 ;;
    esac
  else
    echo "      no run for v$NEW_VERSION picked up yet (waiting on a runner?)"
  fi
done

echo
echo "Done. Watch it live: $GITEA/$OWNER_REPO/actions"
echo "Release lands at:    $GITEA/$OWNER_REPO/releases"
