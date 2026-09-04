package services

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/httpx"
)

// PackageAgeResult is what GetPackagePublishDate returns. PublishedAt is the
// upstream maintainer's release timestamp for the specific version. Source
// identifies which registry produced it.
//
// Fail-open semantics match CheckOSVVulnerabilities: nil result means we
// couldn't determine the age (registry down, ecosystem unsupported, version
// not found). Callers decide what to do with "unknown" — the gating policy
// in the approve handler treats unknown as "allow" so that infrastructure
// issues never block legitimate work.
type PackageAgeResult struct {
	PublishedAt time.Time `json:"published_at"`
	Source      string    `json:"source"`
}

var packageRegistryHTTPClient = httpx.NewClient(10 * time.Second)

// GetPackagePublishDate queries the upstream registry for a specific package
// version's release timestamp. Currently supports npm (registry.npmjs.org)
// and PyPI (pypi.org). Returns nil on any failure — callers must handle that.
//
// This is intentionally separate from OSV.dev: OSV is "what's wrong with this
// version", publish-date is "how old is it." Conflating them means an OSV
// outage would also blind us to recency, which would weaken the Shai-Hulud
// defense (worm waves rely on fresh-release windows).
func GetPackagePublishDate(pkgName, ecosystem, version string) *PackageAgeResult {
	switch ecosystem {
	case "npm":
		return fetchNpmPublishDate(pkgName, version)
	case "PyPI":
		return fetchPyPIPublishDate(pkgName, version)
	default:
		return nil
	}
}

// fetchNpmPublishDate hits https://registry.npmjs.org/<name>/<version>.
// Response shape: { "time": { "<version>": "RFC3339 timestamp" }, ... }
// We request the version-specific URL but the response still includes the
// full time map; pull the requested version.
func fetchNpmPublishDate(pkgName, version string) *PackageAgeResult {
	// The package name may contain a scope ("@scope/name") which must stay
	// path-encoded but not URL-escaped at the slash. PathEscape on each
	// segment, then join.
	endpoint := fmt.Sprintf("https://registry.npmjs.org/%s", url.PathEscape(pkgName))
	resp, err := packageRegistryHTTPClient.Get(endpoint)
	if err != nil {
		log.Printf("[WARNING] [supply_chain] npm_fetch_failed pkg=%s error=%v", pkgName, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[WARNING] [supply_chain] npm_status_non_ok pkg=%s status=%d", pkgName, resp.StatusCode)
		return nil
	}

	var body struct {
		Time map[string]string `json:"time"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		log.Printf("[WARNING] [supply_chain] npm_decode_failed pkg=%s error=%v", pkgName, err)
		return nil
	}
	tsStr, ok := body.Time[version]
	if !ok {
		log.Printf("[WARNING] [supply_chain] npm_version_not_in_time_map pkg=%s version=%s", pkgName, version)
		return nil
	}
	ts, err := time.Parse(time.RFC3339, tsStr)
	if err != nil {
		log.Printf("[WARNING] [supply_chain] npm_parse_time_failed pkg=%s version=%s ts=%q error=%v", pkgName, version, tsStr, err)
		return nil
	}
	return &PackageAgeResult{PublishedAt: ts, Source: "registry.npmjs.org"}
}

// fetchPyPIPublishDate hits https://pypi.org/pypi/<name>/<version>/json.
// Response includes a urls[] array with upload_time_iso_8601 per dist file;
// take the earliest as the canonical publish moment for the version.
func fetchPyPIPublishDate(pkgName, version string) *PackageAgeResult {
	endpoint := fmt.Sprintf("https://pypi.org/pypi/%s/%s/json", url.PathEscape(pkgName), url.PathEscape(version))
	resp, err := packageRegistryHTTPClient.Get(endpoint)
	if err != nil {
		log.Printf("[WARNING] [supply_chain] pypi_fetch_failed pkg=%s error=%v", pkgName, err)
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		log.Printf("[WARNING] [supply_chain] pypi_status_non_ok pkg=%s status=%d", pkgName, resp.StatusCode)
		return nil
	}

	var body struct {
		URLs []struct {
			UploadTimeISO string `json:"upload_time_iso_8601"`
		} `json:"urls"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		log.Printf("[WARNING] [supply_chain] pypi_decode_failed pkg=%s error=%v", pkgName, err)
		return nil
	}
	if len(body.URLs) == 0 {
		log.Printf("[WARNING] [supply_chain] pypi_no_urls_for_version pkg=%s version=%s", pkgName, version)
		return nil
	}
	var earliest time.Time
	for _, u := range body.URLs {
		ts, err := time.Parse(time.RFC3339Nano, u.UploadTimeISO)
		if err != nil {
			continue
		}
		if earliest.IsZero() || ts.Before(earliest) {
			earliest = ts
		}
	}
	if earliest.IsZero() {
		return nil
	}
	return &PackageAgeResult{PublishedAt: earliest, Source: "pypi.org"}
}

// PackageAgeGateDecision is the verdict the approve handler uses to decide
// whether to block, warn, or pass an approval.
type PackageAgeGateDecision struct {
	PublishedAt    time.Time
	AgeHours       float64
	MinAgeHours    float64
	Enforcement    string // "off" | "warn" | "block"
	ShouldBlock    bool
	BlockedUnknown bool // true when the block was caused by fail-closed-on-unknown, not by freshness
	WarnMessage    string
	Source         string
	Unknown        bool // true when we couldn't determine the publish date
}

// PackageAgeGatePolicy is the resolved configuration the gate evaluates against.
// It is computed once per request (env / config / DB / default) and passed into
// EvaluatePackageAgeGate so the decision logic stays pure and testable.
type PackageAgeGatePolicy struct {
	MinAgeHours float64
	Enforcement string // "off" | "warn" | "block"

	// BlockUnknownAge turns the unknown-age path fail-*closed* instead of the
	// default fail-open. It only bites when Enforcement == "block" AND the
	// ecosystem actually has a recency source (EcosystemAged) — so a registry
	// we *expected* to answer but couldn't is treated as suspicious, while a
	// distro package that legitimately has no publish date is never blocked.
	BlockUnknownAge bool

	// EcosystemAged is true for ecosystems where GetPackagePublishDate can
	// produce a real answer (npm, PyPI). For these, a nil result is anomalous
	// rather than merely "unsupported", which is what makes fail-closed safe.
	EcosystemAged bool
}

// EcosystemSupportsPackageAge reports whether GetPackagePublishDate has a
// recency source for the given OSV ecosystem. Only these ecosystems are
// eligible for fail-closed-on-unknown — for everything else a nil age means
// "no source", not "source went dark", and must never block.
func EcosystemSupportsPackageAge(ecosystem string) bool {
	switch ecosystem {
	case "npm", "PyPI":
		return true
	}
	return false
}

// PackageAgeGateConfig reads the supply_chain gate config. For v0.2.0.0 this
// pulls from env (REDFLAG_SUPPLY_CHAIN_*) with defaults baked in; the
// SecuritySettingsService DB-backed override path can layer in later — see
// security_settings_service.go getDefaultSettings "supply_chain".
//
// Defaults are intentional: 24h soak window, "warn" enforcement. The window
// is the documented threshold below which Shai-Hulud-class worms statistically
// announce themselves; warn-by-default preserves operator sovereignty.
func PackageAgeGateConfig() (minAgeHours float64, enforcement string) {
	minAgeHours = 24.0
	enforcement = "warn"

	if v := os.Getenv("REDFLAG_SUPPLY_CHAIN_MIN_PACKAGE_AGE_HOURS"); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil && f >= 0 {
			minAgeHours = f
		}
	}
	if v := os.Getenv("REDFLAG_SUPPLY_CHAIN_GATE_ENFORCEMENT"); v != "" {
		switch strings.ToLower(v) {
		case "off", "warn", "block":
			enforcement = strings.ToLower(v)
		}
	}
	return
}

// EvaluatePackageAgeGate computes the gate decision for a single package
// version against the resolved policy. Pure logic, no HTTP — the fetch happens
// once at the call site and the result is passed in.
func EvaluatePackageAgeGate(age *PackageAgeResult, policy PackageAgeGatePolicy) PackageAgeGateDecision {
	dec := PackageAgeGateDecision{
		MinAgeHours: policy.MinAgeHours,
		Enforcement: policy.Enforcement,
	}
	if age == nil {
		dec.Unknown = true
		// Fail-closed only when explicitly opted in, under block enforcement,
		// and for an ecosystem we expected to answer. Otherwise unknown age is
		// allowed — infrastructure hiccups must not gate legitimate work, and a
		// distro package with no registry recency source is normal, not suspect.
		if policy.Enforcement == "block" && policy.BlockUnknownAge && policy.EcosystemAged {
			dec.ShouldBlock = true
			dec.BlockedUnknown = true
			dec.WarnMessage = "could not determine the publish date of a registry-backed package; failing closed (block_unknown_age) — a dark recency source is itself a Shai-Hulud-class signal"
		}
		return dec
	}
	dec.PublishedAt = age.PublishedAt
	dec.Source = age.Source
	dec.AgeHours = time.Since(age.PublishedAt).Hours()

	if dec.AgeHours >= policy.MinAgeHours {
		return dec
	}

	// Below the threshold — surface a clear message either way.
	dec.WarnMessage = fmt.Sprintf(
		"package published %.1fh ago (threshold %.1fh) — Shai-Hulud-class supply chain attacks propagate inside this window",
		dec.AgeHours, policy.MinAgeHours,
	)
	if policy.Enforcement == "block" {
		dec.ShouldBlock = true
	}
	return dec
}
