package supplychain

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// Standalone-mode OSV check (FEAT-003). In fleet mode OSV runs server-side at
// detection; a standalone host has no server, so the unprivileged agent queries
// OSV.dev directly before requesting a mint. Best-effort with an honest verdict
// (Casey, 2026-06-10): vulnerable is a full stop, unreachable is surfaced as
// "unverified" and mints only over an explicit operator override reason. The
// verdict strings below are the gate_evidence contract with the helper's mint
// mode — keep them aligned with helper/src/main.rs.
const (
	OSVStatusClear       = "clear"
	OSVStatusVulnerable  = "vulnerable"
	OSVStatusUnreachable = "unreachable"
	OSVStatusUnsupported = "unsupported"
)

// osvBatchLimit mirrors the server's batch sizing for /v1/querybatch.
const osvBatchLimit = 100

var osvHTTPClient = &http.Client{Timeout: 15 * time.Second}

// OSV resilience (SEC-029, ETHOS #3 "assume failure; circuit-break fragile
// scanners"). A single OSV.dev blip used to force the operator into an override
// reason or an abandoned install. We now retry transient transport/5xx/429 with
// exponential backoff, and a process-wide breaker fast-fails after a run of
// failures so a sustained outage stops hammering the endpoint. The gate stays
// fail-CLOSED throughout: an exhausted retry or an open breaker surfaces as
// OSVStatusUnreachable, never a silent clear.
var osvBackoff = []time.Duration{500 * time.Millisecond, 2 * time.Second, 8 * time.Second}

const (
	osvBreakerTrip     = 5 // consecutive failed batches before the breaker opens
	osvBreakerCooldown = 60 * time.Second
)

var osvBreaker struct {
	mu          sync.Mutex
	consecutive int
	openUntil   time.Time
}

func osvBreakerOpen() bool {
	osvBreaker.mu.Lock()
	defer osvBreaker.mu.Unlock()
	return time.Now().Before(osvBreaker.openUntil)
}

func osvBreakerRecord(success bool) {
	osvBreaker.mu.Lock()
	defer osvBreaker.mu.Unlock()
	if success {
		osvBreaker.consecutive = 0
		osvBreaker.openUntil = time.Time{}
		return
	}
	osvBreaker.consecutive++
	if osvBreaker.consecutive >= osvBreakerTrip {
		osvBreaker.openUntil = time.Now().Add(osvBreakerCooldown)
		log.Printf("[WARNING] [agent] [supplychain] osv_breaker_open consecutive=%d cooldown=%s",
			osvBreaker.consecutive, osvBreakerCooldown)
	}
}

// osvReadBatch decodes a querybatch response. retryable is true only for
// transient server-side conditions (5xx, 429) so the caller backs off; a hard
// 4xx or a decode failure is not retryable.
func osvReadBatch(resp *http.Response) (batchResp *osvBatchResponse, retryable bool, err error) {
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		retryable = resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests
		return nil, retryable, fmt.Errorf("osv status %d", resp.StatusCode)
	}
	var decoded osvBatchResponse
	if decodeErr := json.NewDecoder(resp.Body).Decode(&decoded); decodeErr != nil {
		return nil, false, decodeErr
	}
	return &decoded, false, nil
}

// osvQueryBatch posts one querybatch payload with retry + breaker, returning the
// decoded response or an error the caller surfaces as unreachable.
func osvQueryBatch(ctx context.Context, body []byte) (*osvBatchResponse, error) {
	if osvBreakerOpen() {
		return nil, fmt.Errorf("osv breaker open")
	}

	var lastErr error
	for attempt := 0; attempt <= len(osvBackoff); attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(osvBackoff[attempt-1]):
			}
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
			"https://api.osv.dev/v1/querybatch", bytes.NewReader(body))
		if err != nil {
			return nil, err // deterministic construction error; not retryable
		}
		httpReq.Header.Set("Content-Type", "application/json")

		resp, err := osvHTTPClient.Do(httpReq)
		if err != nil {
			lastErr = err
			log.Printf("[WARNING] [agent] [supplychain] osv_attempt_failed attempt=%d/%d error=%v",
				attempt+1, len(osvBackoff)+1, err)
			continue
		}

		batchResp, retryable, readErr := osvReadBatch(resp)
		if readErr == nil {
			osvBreakerRecord(true)
			return batchResp, nil
		}
		lastErr = readErr
		if !retryable {
			osvBreakerRecord(false)
			return nil, readErr
		}
		log.Printf("[WARNING] [agent] [supplychain] osv_attempt_failed attempt=%d/%d error=%v",
			attempt+1, len(osvBackoff)+1, readErr)
	}

	osvBreakerRecord(false)
	return nil, lastErr
}

// PkgVersion is the minimal identity OSV needs.
type PkgVersion struct {
	Name    string
	Version string
}

// OSVEcosystem maps RedFlag package types to OSV.dev ecosystem names. Mirrors
// services.EcosystemFromPackageType on the server — keep in sync.
func OSVEcosystem(pkgType string) string {
	switch pkgType {
	case "npm":
		return "npm"
	case "pypi", "pip":
		return "PyPI"
	case "apt":
		return "Debian"
	case "dnf":
		return "AlmaLinux"
	}
	return pkgType
}

type osvQuery struct {
	Package struct {
		Name      string `json:"name"`
		Ecosystem string `json:"ecosystem"`
	} `json:"package"`
	Version string `json:"version"`
}

type osvBatchRequest struct {
	Queries []osvQuery `json:"queries"`
}

type osvBatchResponse struct {
	Results []struct {
		Vulns []struct {
			ID string `json:"id"`
		} `json:"vulns"`
	} `json:"results"`
}

// CheckClosureOSV queries OSV.dev for every package in the closure. Returns the
// gate-evidence verdict and the total vulnerability count. Any transport or
// decode failure returns OSVStatusUnreachable — never a silent clear.
func CheckClosureOSV(ctx context.Context, pkgType string, pkgs []PkgVersion) (string, int) {
	ecosystem := OSVEcosystem(pkgType)
	vulnCount := 0

	for start := 0; start < len(pkgs); start += osvBatchLimit {
		end := start + osvBatchLimit
		if end > len(pkgs) {
			end = len(pkgs)
		}
		batch := pkgs[start:end]

		req := osvBatchRequest{Queries: make([]osvQuery, len(batch))}
		for i, p := range batch {
			req.Queries[i].Package.Name = p.Name
			req.Queries[i].Package.Ecosystem = ecosystem
			req.Queries[i].Version = p.Version
		}
		body, err := json.Marshal(req)
		if err != nil {
			log.Printf("[ERROR] [agent] [supplychain] osv_marshal_failed error=%v", err)
			return OSVStatusUnreachable, 0
		}

		batchResp, err := osvQueryBatch(ctx, body)
		if err != nil {
			log.Printf("[WARNING] [agent] [supplychain] osv_unreachable error=%v", err)
			return OSVStatusUnreachable, 0
		}
		for i, r := range batchResp.Results {
			if len(r.Vulns) > 0 {
				vulnCount += len(r.Vulns)
				log.Printf("[SECURITY] [agent] [supplychain] osv_vulns_found pkg=%s version=%s ecosystem=%s count=%d",
					batch[i].Name, batch[i].Version, ecosystem, len(r.Vulns))
			}
		}
	}

	if vulnCount > 0 {
		return OSVStatusVulnerable, vulnCount
	}
	log.Printf("[INFO] [agent] [supplychain] osv_closure_clear pkg_type=%s packages=%d", pkgType, len(pkgs))
	return OSVStatusClear, 0
}
