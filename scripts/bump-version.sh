#!/usr/bin/env bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"

# --- Current version detection ---
# docker-compose.yml BUILD_VERSION is the read point; this script keeps
# versions.go, docker-compose.yml, and Cargo.toml in lockstep, and the
# release gate verifies all three against the tag.
CURRENT=$(grep -oP '(?<=BUILD_VERSION:-)[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+' "$ROOT/docker-compose.yml" 2>/dev/null || echo "unknown")
CURRENT_TAG=$(git -C "$ROOT" describe --tags --abbrev=0 2>/dev/null | sed 's/^v//' || echo "none")

echo "=== RedFlag Version Bump ==="
echo ""
echo "  Current (docker-compose):  $CURRENT"
echo "  Current (latest git tag):  $CURRENT_TAG"
echo ""

# --- Dirty tree check ---
if ! git -C "$ROOT" diff --quiet 2>/dev/null; then
  DIRTY=$(git -C "$ROOT" diff --stat --no-color 2>/dev/null | tail -1)
  echo "WARNING: Working tree has uncommitted changes:"
  echo "  $DIRTY"
  echo ""
  read -rp "Continue anyway? [y/N] " confirm
  if [[ ! "$confirm" =~ ^[yY]$ ]]; then
    echo "Aborted."
    exit 1
  fi
fi

# --- Argument handling ---
if [ $# -eq 1 ]; then
  NEW_VERSION="$1"
else
  echo "Usage: $0 <new-version>"
  echo "Example: $0 0.2.8.0"
  echo ""
  echo "Bump types for current $CURRENT:"
  # Parse octets
  IFS='.' read -r a b c d <<< "$CURRENT"
  echo "  patch:  $a.$b.$c.$((d+1))"
  echo "  minor:  $a.$b.$((c+1)).0"
  echo "  major:  $a.$((b+1)).0.0"
  exit 1
fi

# --- Format validation ---
if ! [[ "$NEW_VERSION" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Error: version must be N.N.N.N (got: $NEW_VERSION)"
  exit 1
fi

# --- Duplicate check ---
if [ "$NEW_VERSION" = "$CURRENT" ]; then
  echo "Error: new version ($NEW_VERSION) is the same as current ($CURRENT)."
  exit 1
fi

# --- CHANGELOG check ---
if ! grep -q "$NEW_VERSION" "$ROOT/CHANGELOG.md" 2>/dev/null; then
  echo "WARNING: No entry for $NEW_VERSION in CHANGELOG.md"
  echo ""
  read -rp "Continue without changelog entry? [y/N] " confirm
  if [[ ! "$confirm" =~ ^[yY]$ ]]; then
    echo "Aborted. Add an entry to CHANGELOG.md first."
    exit 1
  fi
fi

# --- Confirmation ---
echo ""
echo "Bumping: $CURRENT -> $NEW_VERSION"
echo ""
echo "Files to modify:"
echo "  [1] server/internal/version/versions.go  AgentVersion + ConfigVersion"
echo "  [2] docker-compose.yml                   BUILD_VERSION"
echo "  [3] helper/Cargo.toml                    version"
echo ""
read -rp "Proceed? [y/N] " confirm
if [[ ! "$confirm" =~ ^[yY]$ ]]; then
  echo "Aborted."
  exit 1
fi

echo ""

# 1. server/internal/version/versions.go — AgentVersion + ConfigVersion (not MinAgentVersion)
FILE="$ROOT/server/internal/version/versions.go"
sed -i -E "s/(^[[:space:]]+AgentVersion[[:space:]]*=[[:space:]]*)\"[a-zA-Z0-9.]+\"/\1\"$NEW_VERSION\"/" "$FILE"
sed -i -E "s/(^[[:space:]]+ConfigVersion[[:space:]]*=[[:space:]]*)\"[a-zA-Z0-9.]+\"/\1\"$NEW_VERSION\"/" "$FILE"
echo "  [1] server/internal/version/versions.go  AgentVersion  -> $NEW_VERSION"
echo "  [2] server/internal/version/versions.go  ConfigVersion -> $NEW_VERSION"

# 2. docker-compose.yml — BUILD_VERSION default
FILE="$ROOT/docker-compose.yml"
sed -i -E "s/(BUILD_VERSION: \\\$\{BUILD_VERSION:-)[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+\}/\1$NEW_VERSION}/" "$FILE"
echo "  [3] docker-compose.yml                   BUILD_VERSION -> $NEW_VERSION"

# 3. helper/Cargo.toml — version (semver: 3 parts only, drop 4th octet)
CARGO_VERSION=$(echo "$NEW_VERSION" | cut -d. -f1-3)
FILE="$ROOT/helper/Cargo.toml"
sed -i -E "s/(version[[:space:]]*=[[:space:]]*)\"[0-9]+\.[0-9]+\.[0-9]+(\.[0-9]+)?\"/\1\"$CARGO_VERSION\"/" "$FILE"
echo "  [4] helper/Cargo.toml                    version       -> $CARGO_VERSION (from $NEW_VERSION)"

# 4. desktop/Cargo.toml — version (semver: 3 parts only)
FILE="$ROOT/desktop/Cargo.toml"
if [ -f "$FILE" ]; then
  sed -i -E "s/(version[[:space:]]*=[[:space:]]*)\"[0-9]+\.[0-9]+\.[0-9]+(\.[0-9]+)?\"/\1\"$CARGO_VERSION\"/" "$FILE"
  echo "  [5] desktop/Cargo.toml                   version       -> $CARGO_VERSION (from $NEW_VERSION)"
fi

# Desktop is a Rust/Qt crate. Its Cargo version carries three semver fields;
# release tags and the Go components retain RedFlag's fourth build field.

echo ""

# Refresh action SHA pins so the workflow files ship with current hashes.
echo "--- Updating GitHub Action SHA pins ---"
"$ROOT/scripts/update-action-pins.sh" || true
echo ""

echo "Done. Next steps:"
echo "  1. Update CHANGELOG.md if you haven't already"
echo "  2. git add -A && git commit -m \"v$NEW_VERSION\""
echo "  3. git tag v$NEW_VERSION"
echo "  4. git push gitea-local public --tags"
echo ""
echo "Verify with:"
echo "  grep -n '$NEW_VERSION' server/internal/version/versions.go docker-compose.yml helper/Cargo.toml"
