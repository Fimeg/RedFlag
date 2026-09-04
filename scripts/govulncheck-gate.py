#!/usr/bin/env python3
"""
govulncheck reachability gate.

govulncheck has no native ignore mechanism. RedFlag links github.com/docker/docker
as a client and inherits two daemon-side Moby CVEs that have no fixed version, so a
bare `govulncheck ./...` can never go green. This gate reads govulncheck JSON, keeps
only the *reachable* (called) vulns, subtracts a documented allowlist, and fails the
build on anything left over.

  govulncheck -format=json ./... | govulncheck-gate.py --allow ../.govulncheck-allow

Exit codes:
  0  no un-allowlisted reachable vulns
  1  one or more reachable vulns not in the allowlist  -> CI fails
  2  usage / parse error

A vuln is "reachable" when govulncheck emits a finding whose trace has a frame with
a populated `function` (symbol-level), per govulncheck's own semantics. Findings that
stop at module/package level (imported but not called) are reported as advisory only.
"""
import argparse
import json
import re
import sys

GO_ID = re.compile(r"^GO-\d{4}-\d+$")


def load_allow(path):
    """Return {id: reason} from the allowlist file (blank/`#` lines ignored)."""
    allow = {}
    if not path:
        return allow
    with open(path, encoding="utf-8") as fh:
        for line in fh:
            line = line.strip()
            if not line or line.startswith("#"):
                continue
            parts = line.split(None, 1)
            ident = parts[0]
            if not GO_ID.match(ident):
                print(f"[WARN] [dep-scan] [govulncheck-gate] allowlist line ignored "
                      f"(not a GO-id): {line}", file=sys.stderr)
                continue
            allow[ident] = parts[1].strip() if len(parts) > 1 else "(no reason given)"
    return allow


def parse_findings(text):
    """
    govulncheck -format=json emits a stream of concatenated JSON objects.
    Return {osv_id: reachable_bool} merged across all findings for that id.
    """
    reachable = {}
    osv_titles = {}
    decoder = json.JSONDecoder()
    idx, n = 0, len(text)
    while idx < n:
        while idx < n and text[idx] in " \t\r\n":
            idx += 1
        if idx >= n:
            break
        obj, end = decoder.raw_decode(text, idx)
        idx = end
        if "osv" in obj:
            osv = obj["osv"]
            osv_titles[osv.get("id", "")] = (osv.get("summary") or "").strip()
        if "finding" in obj:
            f = obj["finding"]
            osv_id = f.get("osv")
            if not osv_id:
                continue
            frames = f.get("trace") or []
            called = any(fr.get("function") for fr in frames)
            reachable[osv_id] = reachable.get(osv_id, False) or called
    return reachable, osv_titles


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--allow", default=None, help="path to .govulncheck-allow")
    ap.add_argument("--label", default="", help="module label for log lines")
    ap.add_argument("--summary-out", default=None,
                    help="write a JSON verdict {accepted,blocked,exceptions} here "
                         "for posture assembly")
    args = ap.parse_args()

    raw = sys.stdin.read()
    if not raw.strip():
        print("[ERROR] [dep-scan] [govulncheck-gate] no JSON on stdin", file=sys.stderr)
        return 2
    try:
        reachable, titles = parse_findings(raw)
    except (json.JSONDecodeError, ValueError) as exc:
        print(f"[ERROR] [dep-scan] [govulncheck-gate] bad govulncheck JSON: {exc}",
              file=sys.stderr)
        return 2

    allow = load_allow(args.allow)
    label = f"[{args.label}] " if args.label else ""

    called = sorted(i for i, r in reachable.items() if r)
    blocked = [i for i in called if i not in allow]
    accepted = [i for i in called if i in allow]
    stale = [i for i in allow if i not in called]

    for i in accepted:
        print(f"[INFO] [dep-scan] [govulncheck-gate] {label}accepted {i}: "
              f"{titles.get(i, '')} -- {allow[i]}")
    for i in stale:
        print(f"[WARN] [dep-scan] [govulncheck-gate] {label}stale exception {i} "
              f"no longer reachable -- remove it from .govulncheck-allow")
    for i in blocked:
        print(f"[ERROR] [dep-scan] [govulncheck-gate] {label}reachable & NOT "
              f"allowlisted: {i}: {titles.get(i, '')}")

    if args.summary_out:
        with open(args.summary_out, "w", encoding="utf-8") as fh:
            json.dump({
                "accepted": len(accepted),
                "blocked": len(blocked),
                "exceptions": [{"id": i, "reason": allow[i]} for i in accepted],
            }, fh)

    if blocked:
        print(f"[ERROR] [dep-scan] [govulncheck-gate] {label}{len(blocked)} "
              f"un-accepted reachable vulnerability(ies) -- build blocked")
        return 1
    print(f"[INFO] [dep-scan] [govulncheck-gate] {label}clean "
          f"({len(accepted)} accepted, 0 blocked)")
    return 0


if __name__ == "__main__":
    sys.exit(main())
