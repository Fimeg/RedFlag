#!/usr/bin/env bash
# Resolves and updates pinned GitHub Action SHAs in Gitea workflow files.
#
# Reads lines matching:   uses: org/repo@<40-hex-sha> # <ref>
# Queries GitHub API for the current SHA at <ref> (tag or branch).
# Updates the file in-place if the SHA has changed.
#
# Usage: scripts/update-action-pins.sh [--check]
#   --check  Report stale pins and exit non-zero if any found (CI mode).
#
# Depends on: curl, python3

set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
WORKFLOWS_DIR="$ROOT/.gitea/workflows"
CHECK_ONLY=0
CHANGED=0

if [ "${1:-}" = "--check" ]; then
  CHECK_ONLY=1
fi

resolve_sha() {
  local repo="$1"
  local ref="$2"
  local result type sha

  # Try as tag.
  result=$(curl -sf "https://api.github.com/repos/$repo/git/ref/tags/$ref" 2>/dev/null || echo "")
  type=$(echo "$result" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('object',{}).get('type',''))" 2>/dev/null || echo "")

  if [ "$type" = "commit" ]; then
    echo "$result" | python3 -c "import sys,json; print(json.load(sys.stdin)['object']['sha'])"
    return 0
  elif [ "$type" = "tag" ]; then
    # Annotated tag object — dereference to the commit it wraps.
    sha=$(echo "$result" | python3 -c "import sys,json; print(json.load(sys.stdin)['object']['sha'])")
    curl -sf "https://api.github.com/repos/$repo/git/tags/$sha" 2>/dev/null \
      | python3 -c "import sys,json; print(json.load(sys.stdin)['object']['sha'])"
    return 0
  fi

  # Fall through: try as branch.
  result=$(curl -sf "https://api.github.com/repos/$repo/git/ref/heads/$ref" 2>/dev/null || echo "")
  type=$(echo "$result" | python3 -c "import sys,json; d=json.load(sys.stdin); print(d.get('object',{}).get('type',''))" 2>/dev/null || echo "")

  if [ "$type" = "commit" ]; then
    echo "$result" | python3 -c "import sys,json; print(json.load(sys.stdin)['object']['sha'])"
    return 0
  fi

  echo ""
  return 1
}

process_workflow() {
  local file="$1"

  while IFS= read -r line; do
    # Match:  uses: org/repo@<40-hex> # <ref>
    if [[ "$line" =~ uses:[[:space:]]+([a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+)@([0-9a-f]{40})[[:space:]]+#[[:space:]]+([a-zA-Z0-9_./-]+) ]]; then
      local repo="${BASH_REMATCH[1]}"
      local current="${BASH_REMATCH[2]}"
      local ref="${BASH_REMATCH[3]}"
      local new

      new=$(resolve_sha "$repo" "$ref") || {
        echo "  WARN  $repo@$ref  could not resolve"
        continue
      }

      if [ -z "$new" ]; then
        echo "  WARN  $repo@$ref  empty response from API"
        continue
      fi

      if [ "$current" != "$new" ]; then
        if [ "$CHECK_ONLY" -eq 1 ]; then
          echo "  STALE $repo  current=${current:0:12}  latest=${new:0:12}  (# $ref)"
        else
          sed -i "s|$repo@$current|$repo@$new|g" "$file"
          echo "  BUMP  $repo  ${current:0:12} -> ${new:0:12}  (# $ref)"
        fi
        CHANGED=1
      else
        echo "  OK    $repo@${current:0:12}  (# $ref)"
      fi
    fi
  done < "$file"
}

echo "=== GitHub Action SHA pins ==="
if [ "$CHECK_ONLY" -eq 1 ]; then
  echo "(check mode — no files modified)"
fi
echo ""

for workflow in "$WORKFLOWS_DIR"/*.yml; do
  echo "$(basename "$workflow")"
  process_workflow "$workflow"
  echo ""
done

if [ "$CHANGED" -eq 1 ] && [ "$CHECK_ONLY" -eq 1 ]; then
  echo "Stale pins found. Run scripts/update-action-pins.sh to update."
  exit 1
elif [ "$CHANGED" -eq 1 ]; then
  echo "Pins updated. Stage and commit the workflow files."
else
  echo "All pins are current."
fi
