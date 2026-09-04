#!/bin/sh
# fetch-desktop-windows.sh — obtain the prebuilt, signed Windows Desktop
# binary from the newest published release and verify it against that release's
# manifest hash.
#
# Why this exists: the Go agent cross-compiles to Windows without a Windows
# SDK. Native Qt/QML Desktop does not: it needs a matching Qt and MSVC toolchain.
# That build belongs on a native release runner, never inside every source-built
# server image. Until that runner publishes an artifact, this script returns an
# honest optional absence.
#
# Trust: TLS to the forge + the manifest sha256 gate this download, the same
# trust any release download carries. The server re-signs the binary with its
# own Ed25519 key at startup before serving it to the fleet — that is the
# load-bearing signature agents verify. This step gets a known-good exe into the
# serving path; it is not the fleet trust root.
#
# The desktop component is OPTIONAL (manifest required:false). Every failure that
# is not active tampering degrades to "no Windows Desktop" rather than breaking the
# build: offline, no release yet, no desktop asset in the release, no manifest
# entry. A hash MISMATCH is the one hard stop — that is tampering, not absence.
set -eu

ARCH="${1:-amd64}"
OUT_DIR="${2:-/out}"
REPO_API="${DESKTOP_RELEASE_REPO_API:-https://forge.caseytunturi.com/api/v1/repos/Fimeg/RedFlag}"

log()  { printf '%s\n' "[INFO] [build] [desktop-fetch] $*" >&2; }
warn() { printf '%s\n' "[WARN] [build] [desktop-fetch] $*" >&2; }
fail() { printf '%s\n' "[ERROR] [build] [desktop-fetch] $*" >&2; exit 1; }

mkdir -p "$OUT_DIR"
work="$(mktemp -d)"
trap 'rm -rf "$work"' EXIT

# Newest release INCLUDING prereleases. /releases/latest skips prereleases, and
# every tag below v0.3.0 publishes as a prerelease during the alpha — using it
# would always come back empty. The list endpoint sorts newest-first.
log "querying newest release from $REPO_API"
if ! curl -sfL "$REPO_API/releases?limit=1&draft=false" -o "$work/releases.json"; then
  warn "release API unreachable (offline or forge down) — skipping Windows Desktop"
  exit 0
fi

if [ "$(jq 'length' "$work/releases.json" 2>/dev/null || echo 0)" = "0" ]; then
  log "no published releases at $REPO_API yet — Windows Desktop ships once a native build exists"
  exit 0
fi

manifest_url="$(jq -r '.[0].assets[]? | select(.name == "manifest.json") | .browser_download_url' "$work/releases.json" 2>/dev/null | head -1)"
tag="$(jq -r '.[0].tag_name // empty' "$work/releases.json" 2>/dev/null)"
if [ -z "$manifest_url" ] || [ "$manifest_url" = "null" ]; then
  warn "newest release ($tag) has no manifest.json asset — skipping Windows Desktop"
  exit 0
fi
log "newest release: $tag"

if ! curl -sfL "$manifest_url" -o "$work/manifest.json"; then
  warn "manifest download failed — skipping Windows Desktop"
  exit 0
fi

# Pull the expected hash + filename for desktop-windows/<arch> out of the manifest.
expected_sha="$(jq -r --arg a "$ARCH" \
  '.artifacts[]? | select(.platform == "desktop-windows" and .architecture == $a) | .sha256' \
  "$work/manifest.json" 2>/dev/null | head -1)"
exe_name="$(jq -r --arg a "$ARCH" \
  '.artifacts[]? | select(.platform == "desktop-windows" and .architecture == $a) | .filename' \
  "$work/manifest.json" 2>/dev/null | head -1)"

if [ -z "$expected_sha" ] || [ "$expected_sha" = "null" ]; then
  warn "release $tag carries no desktop-windows/$ARCH binary — Windows Desktop unavailable from this release"
  exit 0
fi

# The exe ships inside the platform zip (release.yml attaches zips, not loose
# exes). Derive the zip asset name from the release version.
ver="${tag#v}"
zip_name="redflag-${ver}-windows-${ARCH}.zip"
zip_url="$(jq -r --arg z "$zip_name" \
  '.[0].assets[]? | select(.name == $z) | .browser_download_url' \
  "$work/releases.json" 2>/dev/null | head -1)"
if [ -z "$zip_url" ] || [ "$zip_url" = "null" ]; then
  warn "release $tag has a manifest entry but no $zip_name asset — skipping Windows Desktop"
  exit 0
fi

log "downloading $zip_name"
if ! curl -sfL "$zip_url" -o "$work/win.zip"; then
  warn "zip download failed — skipping Windows Desktop"
  exit 0
fi

if ! unzip -o -q "$work/win.zip" -d "$work/unz"; then
  warn "zip extraction failed — skipping Windows Desktop"
  exit 0
fi

src="$work/unz/$exe_name"
if [ ! -f "$src" ]; then
  warn "$exe_name not found inside $zip_name — skipping Windows Desktop"
  exit 0
fi

actual_sha="$(sha256sum "$src" | awk '{print $1}')"
if [ "$actual_sha" != "$expected_sha" ]; then
  # Tampering — refuse the build. This is the one non-recoverable case.
  fail "hash mismatch on $exe_name: manifest=$expected_sha actual=$actual_sha"
fi

cp "$src" "$OUT_DIR/redflag-desktop.exe"
log "verified Windows Desktop $tag (desktop-windows/$ARCH) -> $OUT_DIR/redflag-desktop.exe"
