#!/usr/bin/env bash
#
# RedFlag dependency vulnerability gate + supply-chain posture generator.
#
# One audited script, two callers: ci.yml runs it as a gate on every push;
# release.yml runs it with --posture-out to ALSO emit the attested posture that
# gets embedded into the server binary and signed into the release manifest.
#
#   scripts/dep-scan.sh                       # gate only (CI / local)
#   scripts/dep-scan.sh --posture-out PATH    # gate + write attested posture JSON
#
# Gating:
#   Go    govulncheck, reachability-gated via .govulncheck-allow  (server + agent)
#   Web   npm audit --omit=dev at high   (production tree; dev tree advisory only)
#   Rust  cargo audit                    (helper)
# Any un-accepted finding -> exit 1.
#
# Assumes govulncheck, cargo-audit, npm, go on PATH. The workflow installs them;
# locally, `go install golang.org/x/vuln/cmd/govulncheck@latest` and
# `cargo install cargo-audit` once.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
export PATH="$PATH:$(go env GOPATH 2>/dev/null)/bin:$HOME/.cargo/bin"

POSTURE_OUT=""
while [ $# -gt 0 ]; do
  case "$1" in
    --posture-out) POSTURE_OUT="$2"; shift 2 ;;
    *) echo "[ERROR] [dep-scan] unknown arg: $1" >&2; exit 2 ;;
  esac
done

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT
# BLOCK = a real un-accepted vulnerability was found (the gate did its job).
# INFRA = a scanner could not run/parse (offline advisory DB, missing tool).
# These exit differently (1 vs 2) so the Dockerfile can fail the build on a real
# block but degrade to an honest unattested posture when the operator is offline.
BLOCK=0
INFRA=0

# --- Go: reachability-gated via the documented allowlist ---------------------
for mod in server agent; do
  ( cd "$mod" && govulncheck -format=json ./... ) > "$TMP/gv-$mod.json" 2>"$TMP/gv-$mod.err"
  gv_rc=$?
  # govulncheck exit contract: 0 = completed clean, 3 = completed with vulns.
  # Anything else (1/2/137/timeout) means the scan did not finish. govulncheck
  # streams JSON findings as it scans, so a crash mid-scan leaves a stream of
  # complete-but-truncated objects that the gate's lenient raw_decode loop parses
  # without raising -- yielding a falsely clean verdict (real fail-open:
  # attested:true with an undercount). Do NOT feed an incomplete stream to the
  # gate; treat a non-{0,3} exit as INFRA so the posture degrades honestly to
  # unattested instead. (CI-003; ETHOS #1 errors-are-history, #3 assume-failure)
  if [ "$gv_rc" != "0" ] && [ "$gv_rc" != "3" ]; then
    echo "[ERROR] [dep-scan] [$mod] govulncheck exited $gv_rc (scan did not complete) -- stderr:"
    cat "$TMP/gv-$mod.err" >&2
    INFRA=1
    continue
  fi
  python3 scripts/govulncheck-gate.py --allow .govulncheck-allow --label "$mod" \
        --summary-out "$TMP/go-$mod.json" < "$TMP/gv-$mod.json"
  rc=$?
  if [ "$rc" = "1" ]; then BLOCK=1
  elif [ "$rc" != "0" ]; then
    echo "[ERROR] [dep-scan] [$mod] govulncheck-gate could not parse the stream"; INFRA=1
  fi
done

# --- Web: production tree gates; dev tree is advisory (never shipped) ---------
if [ ! -d web/node_modules ]; then
  # npm ci must succeed before npm audit's verdict can be trusted: a half-installed
  # tree can report 0 vulnerabilities and yield a falsely clean posture. Capture
  # stderr (ETHOS: errors are history, never 2>/dev/null) so the real cause shows.
  if ! ( cd web && npm ci --ignore-scripts ) >"$TMP/npm-ci.log" 2>&1; then
    echo "[ERROR] [dep-scan] [web] npm ci failed — production-tree audit is not trustworthy:"
    cat "$TMP/npm-ci.log" >&2
    INFRA=1
  fi
fi
( cd web && npm audit --omit=dev --json ) > "$TMP/npm-prod.json" 2>"$TMP/npm-prod.err" || true
NPM_BLOCK=$(python3 -c "import json,sys
try:
    m=json.load(open('$TMP/npm-prod.json'))['metadata']['vulnerabilities']
    print(m.get('high',0)+m.get('critical',0))
except Exception: print(-1)")
if [ "$NPM_BLOCK" -lt 0 ]; then
  echo "[ERROR] [dep-scan] [web] npm audit (production) did not run (offline?) — stderr:"
  cat "$TMP/npm-prod.err" >&2
  INFRA=1
elif [ "$NPM_BLOCK" -gt 0 ]; then
  echo "[ERROR] [dep-scan] [web] $NPM_BLOCK high/critical vulnerability(ies) in the PRODUCTION tree -- build blocked"
  ( cd web && npm audit --omit=dev --audit-level=high ) || true
  BLOCK=1
else
  echo "[INFO] [dep-scan] [web] production tree clean (0 high/critical)"
fi
# dev tree advisory — print a bounded summary, never block. Surface the advisory
# list (first lines) instead of silencing it outright (ETHOS).
if ! ( cd web && npm audit --audit-level=high ) >"$TMP/npm-dev.out" 2>"$TMP/npm-dev.err"; then
  echo "[WARN] [dep-scan] [web] dev-tree advisories exist (build-time only, not shipped):"
  head -n 8 "$TMP/npm-dev.out" >&2
fi

# --- Rust helper: gate outright ----------------------------------------------
( cd helper && cargo audit --json ) > "$TMP/cargo.json" 2>"$TMP/cargo.err" || true
CARGO_BLOCK=$(python3 -c "import json
try:
    d=json.load(open('$TMP/cargo.json')); print(d['vulnerabilities']['count'])
except Exception: print(-1)")
if [ "$CARGO_BLOCK" -lt 0 ]; then
  echo "[ERROR] [dep-scan] [helper] cargo audit did not run (offline?) — stderr:"
  cat "$TMP/cargo.err" >&2
  INFRA=1
elif [ "$CARGO_BLOCK" -gt 0 ]; then
  echo "[ERROR] [dep-scan] [helper] $CARGO_BLOCK vulnerability(ies) -- build blocked"; BLOCK=1
else
  echo "[INFO] [dep-scan] [helper] clean (0 advisories)"
fi

# --- Posture assembly (release path) -----------------------------------------
if [ -n "$POSTURE_OUT" ]; then
  GO_VER="$(go version 2>/dev/null | awk '{print $3}')"
  RUSTC_VER="$(rustc --version 2>/dev/null | awk '{print $2}')"
  CARGO_VER="$(cargo --version 2>/dev/null | awk '{print $2}')"
  NODE_VER="$(node --version 2>/dev/null)"
  NPM_VER="$(npm --version 2>/dev/null)"
  DOCKER_VER="$(docker version --format '{{.Server.Version}}' 2>/dev/null || echo unknown)"
  # A posture is only attested when every scanner actually ran. If any scan
  # could not complete, we emit an honest unattested posture rather than claim
  # a clean bill we did not earn.
  ATTESTED=$([ "$INFRA" -eq 0 ] && echo true || echo false)
  export GO_VER RUSTC_VER CARGO_VER NODE_VER NPM_VER DOCKER_VER NPM_BLOCK CARGO_BLOCK ATTESTED
  python3 - "$TMP" "$POSTURE_OUT" <<'PY'
import json, os, sys, time
tmp, out = sys.argv[1], sys.argv[2]

def load(name):
    try:
        return json.load(open(os.path.join(tmp, name)))
    except Exception:
        return {"accepted": 0, "blocked": 0, "exceptions": []}

gs, ga = load("go-server.json"), load("go-agent.json")
# Exceptions are the same advisory set across modules — dedupe by id.
exc = {}
for s in (gs, ga):
    for e in s.get("exceptions", []):
        exc[e["id"]] = e["reason"]
go_blocked = gs.get("blocked", 0) + ga.get("blocked", 0)
npm_block = int(os.environ.get("NPM_BLOCK", "0") or 0)
cargo_block = int(os.environ.get("CARGO_BLOCK", "0") or 0)

def status(accepted, blocked):
    return "blocked" if blocked > 0 else ("accepted" if accepted > 0 else "clean")

posture = {
    "attested": os.environ.get("ATTESTED", "false") == "true",
    "generated_at": int(time.time()),
    "substrate": {
        "go": os.environ.get("GO_VER", ""),
        "rustc": os.environ.get("RUSTC_VER", ""),
        "cargo": os.environ.get("CARGO_VER", ""),
        "node": os.environ.get("NODE_VER", ""),
        "npm": os.environ.get("NPM_VER", ""),
        "docker": os.environ.get("DOCKER_VER", ""),
    },
    "scans": [
        {"ecosystem": "go", "tool": "govulncheck", "status": status(len(exc), go_blocked),
         "accepted": len(exc), "blocked": go_blocked},
        {"ecosystem": "npm", "tool": "npm-audit", "status": status(0, max(npm_block, 0)),
         "accepted": 0, "blocked": max(npm_block, 0)},
        {"ecosystem": "cargo", "tool": "cargo-audit", "status": status(0, max(cargo_block, 0)),
         "accepted": 0, "blocked": max(cargo_block, 0)},
    ],
    "exceptions": [{"id": k, "reason": v} for k, v in sorted(exc.items())],
}
with open(out, "w") as fh:
    json.dump(posture, fh, indent=2)
    fh.write("\n")
print(f"[INFO] [dep-scan] [posture] wrote {out} "
      f"(attested={posture['attested']}, "
      f"{len(posture['exceptions'])} accepted exception(s))")
PY
fi

# Exit semantics: a real block is a hard failure everywhere (exit 1). An
# infrastructure-only failure exits 2 so the Dockerfile can degrade to an
# unattested posture offline, while CI (which treats any non-zero as failure)
# still fails loudly.
if [ "$BLOCK" -ne 0 ]; then
  echo "[ERROR] [dep-scan] gate failed — un-accepted dependency vulnerability(ies) present"
  exit 1
fi
if [ "$INFRA" -ne 0 ]; then
  echo "[ERROR] [dep-scan] could not complete — a scanner did not run (offline advisory DB?)"
  exit 2
fi
echo "[INFO] [dep-scan] gate passed"
