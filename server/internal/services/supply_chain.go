package services

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/Fimeg/RedFlag/server/internal/circuitbreaker"
	"github.com/Fimeg/RedFlag/server/internal/httpx"
	"github.com/gofrs/uuid/v5"
)

// OSVQueryRequest is sent to the OSV.dev API.
type OSVQueryRequest struct {
	Package OSVPackage `json:"package"`
	Version string     `json:"version"`
}

// OSVPackage identifies a package in a specific ecosystem.
type OSVPackage struct {
	Name      string `json:"name"`
	Ecosystem string `json:"ecosystem"`
}

// OSVQueryResponse is the response from the OSV.dev API.
type OSVQueryResponse struct {
	Vulns []OSVVuln `json:"vulns"`
}

// OSVVuln represents a single vulnerability from OSV.dev. We decode the subset
// of the OSV schema we surface in the UI: identity, qualitative severity
// (database_specific.severity, used by GHSA for npm/PyPI), CVSS vectors, the
// first fixed version (from affected ranges), and the publish date.
type OSVVuln struct {
	ID               string                 `json:"id"`
	Summary          string                 `json:"summary"`
	Aliases          []string               `json:"aliases"`
	Published        string                 `json:"published"`
	Severity         []OSVSeverity          `json:"severity"`
	Affected         []OSVAffected          `json:"affected"`
	DatabaseSpecific map[string]interface{} `json:"database_specific"`
}

// OSVSeverity is one severity record (typically a CVSS vector string).
type OSVSeverity struct {
	Type  string `json:"type"`
	Score string `json:"score"`
}

// OSVAffected carries the affected version ranges for a package.
type OSVAffected struct {
	Ranges []OSVRange `json:"ranges"`
}

// OSVRange is an introduced/fixed event sequence over a version space.
type OSVRange struct {
	Type   string     `json:"type"`
	Events []OSVEvent `json:"events"`
}

// OSVEvent is a single boundary in a range (introduced or fixed).
type OSVEvent struct {
	Introduced   string `json:"introduced"`
	Fixed        string `json:"fixed"`
	LastAffected string `json:"last_affected"`
	Limit        string `json:"limit"`
}

// SupplyChainCheckResult is returned by the vulnerability check.
type SupplyChainCheckResult struct {
	Vulnerabilities []VulnerabilityInfo `json:"vulnerabilities"`
	CheckedAt       time.Time           `json:"checked_at"`
}

// VulnerabilityInfo is a display-oriented vulnerability record. Stored as JSON
// in current_package_state.metadata.supply_chain_vulns and rendered by the UI.
// Severity is qualitative (CRITICAL/HIGH/MODERATE/LOW) when OSV provides it;
// CVSSVector is the raw v3/v4 vector for operators who want the dimensions.
type VulnerabilityInfo struct {
	ID             string   `json:"id"`
	Summary        string   `json:"summary"`
	Aliases        []string `json:"aliases"`
	Severity       string   `json:"severity,omitempty"`
	CVSSVector     string   `json:"cvss_vector,omitempty"`
	CVSSScore      float64  `json:"cvss_score,omitempty"`
	FixedVersion   string   `json:"fixed_version,omitempty"`
	Published      string   `json:"published,omitempty"`
	AdvisoryType   string   `json:"advisory_type,omitempty"` // "AlmaLinux advisory", "CVE", etc.
	AffectedRanges []string `json:"affected_ranges,omitempty"`
	KnownExploited bool     `json:"known_exploited,omitempty"`
}

// AdvisoryType returns a human-readable label for an advisory ID prefix.
func AdvisoryType(id string) string {
	switch {
	case strings.HasPrefix(id, "ALSA-"):
		return "AlmaLinux advisory"
	case strings.HasPrefix(id, "ALBA-"), strings.HasPrefix(id, "ALEA-"):
		return "AlmaLinux erratum (non-security)"
	case strings.HasPrefix(id, "RHSA-"):
		return "Red Hat advisory"
	case strings.HasPrefix(id, "RHBA-"), strings.HasPrefix(id, "RHEA-"):
		return "Red Hat erratum (non-security)"
	case strings.HasPrefix(id, "USN-"):
		return "Ubuntu advisory"
	case strings.HasPrefix(id, "GHSA-"):
		return "GitHub advisory"
	case strings.HasPrefix(id, "CVE-"):
		return "CVE"
	default:
		return "security advisory"
	}
}

// nonSecurityErrata matches RPM-family errata that carry no security content:
// bugfix (*BA) and enhancement (*EA) advisories from AlmaLinux, Red Hat,
// Rocky, and Oracle. OSV returns them alongside security advisories; counting
// them as threats inflates the dashboard with non-threats.
var nonSecurityErrata = regexp.MustCompile(`^(AL|RH|RL|EL)(BA|EA)-`)

// IsSecurityAdvisory reports whether an OSV record ID names an actual
// security advisory rather than a bugfix/enhancement erratum.
func IsSecurityAdvisory(id string) bool {
	return !nonSecurityErrata.MatchString(id)
}

// filterSecurityVulns drops non-security errata from an OSV result set.
func filterSecurityVulns(vulns []OSVVuln) []OSVVuln {
	kept := vulns[:0:0]
	for _, v := range vulns {
		if IsSecurityAdvisory(v.ID) {
			kept = append(kept, v)
		} else {
			log.Printf("[INFO] [supply_chain] erratum_filtered id=%s (non-security)", v.ID)
		}
	}
	return kept
}

// toVulnerabilityInfo maps a raw OSV record into the display struct, pulling
// qualitative severity, the first CVSS vector, the first fixed version, and the
// publish date out of the OSV schema's various nesting points.
func toVulnerabilityInfo(v OSVVuln) VulnerabilityInfo {
	info := VulnerabilityInfo{
		ID:             v.ID,
		Summary:        v.Summary,
		Aliases:        v.Aliases,
		Published:      v.Published,
		AdvisoryType:   AdvisoryType(v.ID),
		AffectedRanges: affectedRanges(v.Affected),
		KnownExploited: knownExploited(v.DatabaseSpecific),
	}

	// Qualitative severity: GHSA puts it in database_specific.severity.
	if v.DatabaseSpecific != nil {
		if s, ok := v.DatabaseSpecific["severity"].(string); ok && s != "" {
			info.Severity = strings.ToUpper(s)
		}
	}

	// First CVSS vector (prefer v4, else v3, else whatever is present).
	for _, s := range v.Severity {
		if strings.HasPrefix(s.Score, "CVSS:") {
			info.CVSSVector = s.Score
			if strings.HasPrefix(s.Score, "CVSS:4") {
				break
			}
		}
	}
	if score, ok := cvss3BaseScore(info.CVSSVector); ok {
		info.CVSSScore = score
		if info.Severity == "" {
			info.Severity = severityFromCVSSScore(score)
		}
	}

	// First fixed version across affected ranges.
	for _, a := range v.Affected {
		for _, r := range a.Ranges {
			for _, e := range r.Events {
				if e.Fixed != "" {
					info.FixedVersion = e.Fixed
					break
				}
			}
			if info.FixedVersion != "" {
				break
			}
		}
		if info.FixedVersion != "" {
			break
		}
	}

	return info
}

func cvss3BaseScore(vector string) (float64, bool) {
	if !strings.HasPrefix(vector, "CVSS:3.") {
		return 0, false
	}

	metrics := make(map[string]string)
	for _, part := range strings.Split(vector, "/") {
		key, value, ok := strings.Cut(part, ":")
		if ok {
			metrics[key] = value
		}
	}

	avMap := map[string]float64{"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.2}
	acMap := map[string]float64{"L": 0.77, "H": 0.44}
	uiMap := map[string]float64{"N": 0.85, "R": 0.62}
	impactMap := map[string]float64{"H": 0.56, "L": 0.22, "N": 0}

	scope := metrics["S"]
	if scope != "U" && scope != "C" {
		return 0, false
	}
	prMap := map[string]float64{"N": 0.85, "L": 0.62, "H": 0.27}
	if scope == "C" {
		prMap = map[string]float64{"N": 0.85, "L": 0.68, "H": 0.5}
	}

	av, okAV := avMap[metrics["AV"]]
	ac, okAC := acMap[metrics["AC"]]
	pr, okPR := prMap[metrics["PR"]]
	ui, okUI := uiMap[metrics["UI"]]
	c, okC := impactMap[metrics["C"]]
	i, okI := impactMap[metrics["I"]]
	a, okA := impactMap[metrics["A"]]
	if !okAV || !okAC || !okPR || !okUI || !okC || !okI || !okA {
		return 0, false
	}

	exploitability := 8.22 * av * ac * pr * ui
	impactSubScore := 1 - (1-c)*(1-i)*(1-a)
	impact := 6.42 * impactSubScore
	if scope == "C" {
		impact = 7.52*(impactSubScore-0.029) - 3.25*math.Pow(impactSubScore-0.02, 15)
	}
	if impact <= 0 {
		return 0, true
	}

	score := impact + exploitability
	if scope == "C" {
		score = 1.08 * score
	}
	if score > 10 {
		score = 10
	}
	return math.Ceil((score-1e-10)*10) / 10, true
}

func severityFromCVSSScore(score float64) string {
	switch {
	case score >= 9:
		return "CRITICAL"
	case score >= 7:
		return "HIGH"
	case score >= 4:
		return "MEDIUM"
	case score > 0:
		return "LOW"
	default:
		return ""
	}
}

func affectedRanges(affected []OSVAffected) []string {
	var ranges []string
	for _, a := range affected {
		for _, r := range a.Ranges {
			var introduced string
			emitted := false
			for _, e := range r.Events {
				if e.Introduced != "" {
					introduced = e.Introduced
				}
				switch {
				case e.Fixed != "":
					ranges = append(ranges, formatAffectedRange(r.Type, introduced, "<", e.Fixed))
					introduced = ""
					emitted = true
				case e.LastAffected != "":
					ranges = append(ranges, formatAffectedRange(r.Type, introduced, "<=", e.LastAffected))
					emitted = true
				case e.Limit != "":
					ranges = append(ranges, formatAffectedRange(r.Type, introduced, "<", e.Limit))
					emitted = true
				}
			}
			if !emitted && introduced != "" {
				ranges = append(ranges, formatAffectedRange(r.Type, introduced, "", ""))
			}
		}
	}
	return ranges
}

func formatAffectedRange(rangeType, introduced, upperOp, upperVersion string) string {
	lower := "all prior versions"
	if introduced != "" && introduced != "0" {
		lower = ">= " + introduced
	}
	body := lower + " and later"
	if upperOp != "" && upperVersion != "" {
		body = lower + ", " + upperOp + " " + upperVersion
	}
	if rangeType != "" {
		return rangeType + ": " + body
	}
	return body
}

func knownExploited(databaseSpecific map[string]interface{}) bool {
	keys := []string{
		"known_exploited",
		"knownExploited",
		"known_exploited_vulnerability",
		"knownExploitedVulnerability",
		"cisa_kev",
		"cisaKev",
		"cisa_known_exploited",
		"cisaKnownExploited",
		"cisaExploitAdd",
		"cisaActionDue",
		"cisaRequiredAction",
		"cisaVulnerabilityName",
		"kev",
	}
	for _, key := range keys {
		if truthy(databaseSpecific[key]) {
			return true
		}
	}
	return false
}

func truthy(v interface{}) bool {
	switch value := v.(type) {
	case bool:
		return value
	case string:
		normalized := strings.ToLower(strings.TrimSpace(value))
		return normalized != "" &&
			normalized != "false" &&
			normalized != "no" &&
			normalized != "none" &&
			normalized != "unknown" &&
			normalized != "0"
	case float64:
		return value > 0
	case int:
		return value > 0
	case []interface{}:
		return len(value) > 0
	case map[string]interface{}:
		return len(value) > 0
	default:
		return false
	}
}

var osvHTTPClient = httpx.NewClient(30 * time.Second)

// osvBreaker wraps OSV.dev calls (SCALE-001 S8). Both the batch and single-query
// paths hit api.osv.dev, so they share one breaker: when OSV is down or slow,
// the breaker opens and subsequent checks fail fast instead of each timing out.
// Callers fail OPEN on an open breaker (record the check as unrun) — sovereignty:
// an unreachable advisory feed never blocks a patch, same as today's transport
// errors.
var osvBreaker = circuitbreaker.New("osv", circuitbreaker.Config{
	FailureThreshold: 5,
	FailureWindow:    60 * time.Second,
	OpenDuration:     30 * time.Second,
	HalfOpenAttempts: 2,
})

// OSVBreakerStats exposes the OSV breaker state for /health/tasks (OBS-001).
func OSVBreakerStats() circuitbreaker.Stats { return osvBreaker.GetStats() }

// OSVRecheckInterval is how long a stored supply-chain result is treated as
// fresh. The scan path skips packages checked more recently than this so a
// frequent scan cadence does not re-hit OSV.dev for every package every cycle;
// the periodic recheck still picks up newly published advisories.
const OSVRecheckInterval = 6 * time.Hour

// osvBatchSize is the max queries per /v1/querybatch POST. OSV.dev accepts up
// to 1000; 100 keeps each request under 2s and the response payload manageable.
const osvBatchSize = 100

// osvBatchesInFlight caps the number of concurrent batch requests to OSV.dev
// across the entire process. At 10,000 agents × ~300 packages each, we could
// have millions of checks queued — this semaphore ensures we don't hammer the
// API regardless of how many callers invoke RunOSVChecks simultaneously.
const osvBatchesInFlight = 4

// osvBatchSem is the process-wide semaphore for concurrent OSV.dev batch
// requests. Shared across all RunOSVChecks calls so that N agents reporting
// simultaneously total at most osvBatchesInFlight requests in flight.
var osvBatchSem = make(chan struct{}, osvBatchesInFlight)

// OSVCheckRequest is one package to check against OSV.dev.
// Namespace routes results to different metadata keys:
//   - "" or "remediation" → supply_chain_checked_at / supply_chain_checked_version / supply_chain_vulns
//   - "installed"         → installed_checked_at / installed_checked_version / installed_vulns
type OSVCheckRequest struct {
	AgentID   uuid.UUID
	PkgType   string
	PkgName   string
	Version   string
	Namespace string // "" = remediation (default)
}

// OSVStoreFunc persists a supply-chain result (or a recorded failure) for one
// package. Implemented by the caller against current_package_state so this
// package keeps no database dependency.
type OSVStoreFunc func(agentID uuid.UUID, pkgType, pkgName string, meta map[string]interface{}) error

// osvBatchQuery is one entry in the /v1/querybatch request.
type osvBatchQuery struct {
	Package OSVPackage `json:"package"`
	Version string     `json:"version"`
}

// osvBatchRequest is the POST body for /v1/querybatch.
type osvBatchRequest struct {
	Queries []osvBatchQuery `json:"queries"`
}

// osvBatchResult is one result from /v1/querybatch.
type osvBatchResult struct {
	Vulns []OSVVuln `json:"vulns"`
}

// osvBatchResponse is the response from /v1/querybatch.
type osvBatchResponse struct {
	Results []osvBatchResult `json:"results"`
}

// RunOSVChecks queries OSV.dev for each request using the batch endpoint with
// bounded, process-wide concurrency, and persists every outcome through store.
// A clean result records checked_at and checked_version, and clears stale vulns;
// a hit also records vulns. A query failure records check_error WITHOUT a
// checked_at timestamp, so the package stays a candidate for the next run rather
// than being silently dropped (ETHOS: errors are history, assume failure).
// Blocks until done.
func RunOSVChecks(reqs []OSVCheckRequest, store OSVStoreFunc) {
	if len(reqs) == 0 {
		return
	}

	var wg sync.WaitGroup
	for i := 0; i < len(reqs); i += osvBatchSize {
		end := i + osvBatchSize
		if end > len(reqs) {
			end = len(reqs)
		}
		batch := reqs[i:end]

		osvBatchSem <- struct{}{}
		wg.Add(1)
		go func(b []OSVCheckRequest) {
			defer wg.Done()
			defer func() { <-osvBatchSem }()
			osvBatchRun(b, store)
		}(batch)
	}
	wg.Wait()
}

// osvBatchRun sends one batch to /v1/querybatch and persists all results.
func osvBatchRun(reqs []OSVCheckRequest, store OSVStoreFunc) {
	queries := make([]osvBatchQuery, len(reqs))
	for i, r := range reqs {
		queries[i] = osvBatchQuery{
			Package: OSVPackage{
				Name:      r.PkgName,
				Ecosystem: EcosystemFromPackageType(r.PkgType),
			},
			Version: r.Version,
		}
	}

	body, err := json.Marshal(osvBatchRequest{Queries: queries})
	if err != nil {
		log.Printf("[WARNING] [supply_chain] batch_marshal_failed count=%d error=%v", len(reqs), err)
		recordBatchFailure(reqs, store)
		return
	}

	// Breaker-wrapped (SCALE-001 S8): a down/slow OSV trips the breaker so the
	// rest of this batch — and the single-query path — fail fast rather than each
	// blocking on a 30s timeout. An open breaker takes the same fail-open path as
	// a transport error below.
	var batchResp osvBatchResponse
	callErr := osvBreaker.Call(func() error {
		resp, err := osvHTTPClient.Post("https://api.osv.dev/v1/querybatch", "application/json", bytes.NewReader(body))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("osv batch status %d", resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
			return fmt.Errorf("decode: %w", err)
		}
		return nil
	})
	if callErr != nil {
		log.Printf("[WARNING] [supply_chain] batch_query_failed count=%d error=%v", len(reqs), callErr)
		recordBatchFailure(reqs, store)
		return
	}

	if len(batchResp.Results) != len(reqs) {
		log.Printf("[WARNING] [supply_chain] batch_count_mismatch sent=%d got=%d", len(reqs), len(batchResp.Results))
		recordBatchFailure(reqs, store)
		return
	}

	now := time.Now().UTC()
	for i, r := range reqs {
		result := batchResp.Results[i]
		checkedKey, versionKey, vulnsKey, errorKey := osvMetaKeys(r.Namespace)
		meta := map[string]interface{}{
			checkedKey: now.Format(time.RFC3339),
			versionKey: r.Version,
			vulnsKey:   nil,
			errorKey:   nil,
		}

		securityVulns := filterSecurityVulns(result.Vulns)
		if len(securityVulns) > 0 {
			// Enrich ALSA/RHSA/USN advisories with full details from OSV.
			enriched := enrichAdvisoryVulns(securityVulns)
			vulns := make([]VulnerabilityInfo, len(enriched))
			for i, v := range enriched {
				vulns[i] = toVulnerabilityInfo(v)
			}
			vulnJSON, err := json.Marshal(vulns)
			if err != nil {
				log.Printf("[WARNING] [supply_chain] vuln_marshal_failed pkg=%s error=%v", r.PkgName, err)
				continue
			}
			meta[vulnsKey] = string(vulnJSON)
			log.Printf("[SECURITY] [supply_chain] vulns_found namespace=%s pkg=%s type=%s ecosystem=%s count=%d",
				r.Namespace, r.PkgName, r.PkgType, EcosystemFromPackageType(r.PkgType), len(securityVulns))
		} else {
			log.Printf("[INFO] [supply_chain] clean namespace=%s pkg=%s type=%s ecosystem=%s version=%s",
				r.Namespace, r.PkgName, r.PkgType, EcosystemFromPackageType(r.PkgType), r.Version)
		}

		if err := store(r.AgentID, r.PkgType, r.PkgName, meta); err != nil {
			log.Printf("[WARNING] [supply_chain] metadata_store_failed pkg=%s error=%v", r.PkgName, err)
		}
	}
}

// enrichAdvisoryVulns does a second OSV pass for ALSA/RHSA/USN advisories to
// resolve their aliases and summaries. OSV's batch endpoint returns advisory IDs
// but often without full details; the single-vuln endpoint (/v1/vulns/{id})
// returns the complete record including constituent CVEs.
func enrichAdvisoryVulns(vulns []OSVVuln) []OSVVuln {
	enriched := make([]OSVVuln, 0, len(vulns))
	for _, v := range vulns {
		if needsEnrichment(v) {
			if resolved, err := fetchVulnDetail(v.ID); err == nil {
				enriched = append(enriched, resolved)
				continue
			}
			// On failure, keep original — partial data is better than none.
			log.Printf("[WARNING] [supply_chain] enrich_failed id=%s", v.ID)
		}
		enriched = append(enriched, v)
	}
	return enriched
}

// needsEnrichment returns true if a vuln record looks sparse — missing
// summary and aliases — and its ID is a known advisory prefix.
func needsEnrichment(v OSVVuln) bool {
	if v.Summary != "" && len(v.Aliases) > 0 {
		return false
	}
	return strings.HasPrefix(v.ID, "ALSA-") ||
		strings.HasPrefix(v.ID, "RHSA-") ||
		strings.HasPrefix(v.ID, "USN-")
}

// fetchVulnDetail queries OSV.dev for a single vulnerability ID.
func fetchVulnDetail(id string) (OSVVuln, error) {
	url := fmt.Sprintf("https://api.osv.dev/v1/vulns/%s", id)
	resp, err := osvHTTPClient.Get(url)
	if err != nil {
		return OSVVuln{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return OSVVuln{}, fmt.Errorf("osv status %d", resp.StatusCode)
	}
	var v OSVVuln
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return OSVVuln{}, err
	}
	return v, nil
}

// osvMetaKeys returns the metadata key names for a given check namespace.
func osvMetaKeys(namespace string) (checkedAt, checkedVersion, vulns, checkError string) {
	if namespace == "installed" {
		return "installed_checked_at", "installed_checked_version", "installed_vulns", "installed_check_error"
	}
	return "supply_chain_checked_at", "supply_chain_checked_version", "supply_chain_vulns", "supply_chain_check_error"
}

// recordBatchFailure persists a failure record for every request in a batch
// that could not be checked (HTTP error, decode failure, count mismatch).
// No checked_at is set so these packages are retried next cycle.
func recordBatchFailure(reqs []OSVCheckRequest, store OSVStoreFunc) {
	for _, r := range reqs {
		_, _, _, errorKey := osvMetaKeys(r.Namespace)
		meta := map[string]interface{}{
			errorKey:                "osv_batch_failed",
			"supply_chain_error_at": time.Now().UTC().Format(time.RFC3339),
		}
		if err := store(r.AgentID, r.PkgType, r.PkgName, meta); err != nil {
			log.Printf("[WARNING] [supply_chain] failure_record_failed pkg=%s error=%v", r.PkgName, err)
		}
	}
}

// ClosurePkg is one resolved artifact (name + version) in a dependency closure
// to be checked against OSV.dev.
type ClosurePkg struct {
	Name    string
	Version string
}

// ClosureVuln records a closure artifact that has known vulnerabilities.
type ClosureVuln struct {
	Name    string    `json:"name"`
	Version string    `json:"version"`
	Vulns   []OSVVuln `json:"vulns"`
}

// CheckClosureOSV queries OSV.dev for every artifact in a resolved dependency
// closure and returns the subset with known vulnerabilities. This is the
// dependency-level supply-chain gate: the closure is the exact set of artifacts
// the capability token authorizes the network-less executor to install, so each
// transitive artifact is queried — not just the top-level package that was
// checked at discovery.
//
// Unlike CheckOSVVulnerabilities (fail-open, for advisory display), this is
// FAIL-CLOSED: if any batch cannot be queried (HTTP error, bad status, decode
// failure, count mismatch), it returns ok=false. The caller must treat a
// not-ok result as "closure not cleared" and never as clean — auto-confirm must
// not mint a token over a closure OSV could not vet. All entries share the
// update's ecosystem.
func CheckClosureOSV(pkgType string, entries []ClosurePkg) (found []ClosureVuln, ok bool) {
	if len(entries) == 0 {
		return nil, true // empty closure is trivially clean
	}
	ecosystem := EcosystemFromPackageType(pkgType)

	for i := 0; i < len(entries); i += osvBatchSize {
		end := i + osvBatchSize
		if end > len(entries) {
			end = len(entries)
		}
		batch := entries[i:end]
		hits, batchOK := osvClosureBatch(ecosystem, batch)
		if !batchOK {
			return found, false // fail closed: a batch we couldn't check is not "clean"
		}
		found = append(found, hits...)
	}
	return found, true
}

// osvClosureBatch sends one closure batch to /v1/querybatch under the
// process-wide semaphore and returns the entries with vulns. ok=false on any
// query/parse failure.
func osvClosureBatch(ecosystem string, batch []ClosurePkg) (found []ClosureVuln, ok bool) {
	queries := make([]osvBatchQuery, len(batch))
	for i, e := range batch {
		queries[i] = osvBatchQuery{Package: OSVPackage{Name: e.Name, Ecosystem: ecosystem}, Version: e.Version}
	}
	body, err := json.Marshal(osvBatchRequest{Queries: queries})
	if err != nil {
		log.Printf("[WARNING] [supply_chain] closure_batch_marshal_failed count=%d error=%v", len(batch), err)
		return nil, false
	}

	osvBatchSem <- struct{}{}
	defer func() { <-osvBatchSem }()

	resp, err := osvHTTPClient.Post("https://api.osv.dev/v1/querybatch", "application/json", bytes.NewReader(body))
	if err != nil {
		log.Printf("[WARNING] [supply_chain] closure_batch_query_failed count=%d error=%v", len(batch), err)
		return nil, false
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Printf("[WARNING] [supply_chain] closure_batch_query_status=%d count=%d", resp.StatusCode, len(batch))
		return nil, false
	}

	var batchResp osvBatchResponse
	if err := json.NewDecoder(resp.Body).Decode(&batchResp); err != nil {
		log.Printf("[WARNING] [supply_chain] closure_batch_decode_failed count=%d error=%v", len(batch), err)
		return nil, false
	}
	if len(batchResp.Results) != len(batch) {
		log.Printf("[WARNING] [supply_chain] closure_batch_count_mismatch sent=%d got=%d", len(batch), len(batchResp.Results))
		return nil, false
	}

	for i, e := range batch {
		securityVulns := filterSecurityVulns(batchResp.Results[i].Vulns)
		if len(securityVulns) > 0 {
			found = append(found, ClosureVuln{Name: e.Name, Version: e.Version, Vulns: securityVulns})
			log.Printf("[SECURITY] [supply_chain] closure_vuln pkg=%s version=%s ecosystem=%s count=%d",
				e.Name, e.Version, ecosystem, len(securityVulns))
		}
	}
	return found, true
}

// CheckOSVVulnerabilities queries the OSV.dev API for known vulnerabilities
// matching the given package name, ecosystem, and version. Returns nil slice
// with no error on clean results or API failure (fail-open).
func CheckOSVVulnerabilities(pkgName, ecosystem, version string) *SupplyChainCheckResult {
	reqBody := OSVQueryRequest{
		Package: OSVPackage{
			Name:      pkgName,
			Ecosystem: ecosystem,
		},
		Version: version,
	}

	body, err := json.Marshal(reqBody)
	if err != nil {
		log.Printf("[WARNING] [supply_chain] marshal_failed pkg=%s error=%v", pkgName, err)
		return nil
	}

	// Breaker-wrapped (SCALE-001 S8); shares osvBreaker with the batch path. An
	// open breaker returns nil (fail-open) — same as a transport error: the check
	// is recorded as unrun, never blocking a patch (sovereignty).
	var result OSVQueryResponse
	callErr := osvBreaker.Call(func() error {
		resp, err := osvHTTPClient.Post("https://api.osv.dev/v1/query", "application/json", bytes.NewReader(body))
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("osv status %d", resp.StatusCode)
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			return fmt.Errorf("decode: %w", err)
		}
		return nil
	})
	if callErr != nil {
		log.Printf("[WARNING] [supply_chain] query_failed pkg=%s error=%v", pkgName, callErr)
		return nil
	}

	securityVulns := filterSecurityVulns(result.Vulns)
	if len(securityVulns) == 0 {
		return &SupplyChainCheckResult{
			CheckedAt: time.Now().UTC(),
		}
	}

	vulns := make([]VulnerabilityInfo, len(securityVulns))
	for i, v := range securityVulns {
		vulns[i] = toVulnerabilityInfo(v)
	}

	return &SupplyChainCheckResult{
		Vulnerabilities: vulns,
		CheckedAt:       time.Now().UTC(),
	}
}

// NeedsSupplyChainCheck returns true if the given package ecosystem can be
// checked against OSV.dev. Broader than before — runs for all ecosystems that
// have an OSV.dev mapping, even when coverage is sparse. A nil result is honest
// visibility (the check ran, nothing found).
//
// TODO(GATE-006): pacman is wired as a scanner but OSV.dev does not yet cover
// Arch Linux (as of 2026-06). When OSV adds Arch support, add "pacman" here
// and flip EcosystemFromPackageType to return "Arch". Track via OSV.dev
// ecosystem registry — search for "arch" or "archlinux". Until then, pacman
// updates skip the supply-chain check and rely on the capability gate + hash
// verification for install safety.
func NeedsSupplyChainCheck(pkgType string) bool {
	switch pkgType {
	case "npm", "pip", "pypi", "apt", "dnf":
		return true
	}
	return false
}

// CanServerFetchArtifact returns true if the server can download package
// artifacts from the public registry for the given ecosystem. Agent-sourced
// ecosystems (dnf, apt) return false — their artifacts come from the agent's
// own repos, which the server may not be able to reach (custom mirrors,
// air-gapped networks). The mirror tier may add server-side fetching for
// these ecosystems later.
func CanServerFetchArtifact(pkgType string) bool {
	switch pkgType {
	case "npm", "pip", "pypi":
		return true
	}
	return false
}

// NeedsCapabilityGate returns true if the ecosystem routes mutation through
// the capability-token path (consumer.go → redflag-helper). Server-fetched
// ecosystems (npm/pypi) and agent-sourced ecosystems (dnf/apt) both use it
// when the minter is enabled. pacman routes through the gate for privilege
// isolation and artifact hashing even though OSV does not cover Arch.
func NeedsCapabilityGate(pkgType string) bool {
	switch pkgType {
	case "dnf", "apt", "npm", "pip", "pypi", "pacman":
		return true
	}
	return false
}

// EcosystemFromPackageType maps RedFlag package types to OSV.dev ecosystems.
// Best-effort: dnf maps to AlmaLinux (closest supported RHEL-family ecosystem),
// apt maps to Debian. Unmapped types return the raw package type — OSV.dev will
// return empty results for unrecognized ecosystems rather than error. pacman
// returns "Arch" for future OSV coverage; as of 2026-06 OSV.dev does not support
// Arch Linux, so NeedsSupplyChainCheck returns false for pacman.
func EcosystemFromPackageType(pkgType string) string {
	switch pkgType {
	case "npm":
		return "npm"
	case "pip", "pypi":
		return "PyPI"
	case "apt":
		return "Debian"
	case "dnf":
		return "AlmaLinux"
	case "pacman":
		return "Arch"
	}
	return pkgType
}
