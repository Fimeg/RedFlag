package supplychain

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestOSVEcosystem(t *testing.T) {
	tests := []struct {
		pkgType string
		want    string
	}{
		{"npm", "npm"},
		{"pypi", "PyPI"},
		{"pip", "PyPI"},
		{"apt", "Debian"},
		{"dnf", "AlmaLinux"},
		{"unknown", "unknown"},
	}
	for _, tt := range tests {
		if got := OSVEcosystem(tt.pkgType); got != tt.want {
			t.Errorf("OSVEcosystem(%q) = %q, want %q", tt.pkgType, got, tt.want)
		}
	}
}

func TestCheckClosureOSVUnreachable(t *testing.T) {
	// Mock HTTP client that returns 503.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer ts.Close()

	orig := osvHTTPClient
	osvHTTPClient = ts.Client()
	defer func() { osvHTTPClient = orig }()

	// We can't easily redirect the hardcoded URL, but we can test with a
	// real unreachable host by pointing the client at the test server and
	// using a context cancel to simulate transport failure.
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // immediately cancelled — simulates unreachable

	status, vulns := CheckClosureOSV(ctx, "npm", []PkgVersion{
		{Name: "left-pad", Version: "1.3.0"},
	})
	if status != OSVStatusUnreachable {
		t.Errorf("expected unreachable, got %q", status)
	}
	if vulns != 0 {
		t.Errorf("expected 0 vulns on unreachable, got %d", vulns)
	}
}

func TestCheckClosureOSVEmpty(t *testing.T) {
	// Empty closure should return clear without making any HTTP call.
	status, vulns := CheckClosureOSV(context.Background(), "npm", nil)
	if status != OSVStatusClear {
		t.Errorf("expected clear for empty closure, got %q", status)
	}
	if vulns != 0 {
		t.Errorf("expected 0 vulns for empty closure, got %d", vulns)
	}
}

func TestCheckClosureOSVCleanResponse(t *testing.T) {
	// Mock OSV batch endpoint returning no vulnerabilities.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req osvBatchRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("failed to decode request: %v", err)
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		resp := osvBatchResponse{
			Results: make([]struct {
				Vulns []struct {
					ID string `json:"id"`
				} `json:"vulns"`
			}, len(req.Queries)),
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	// Override the package-level client and URL via a custom transport.
	orig := osvHTTPClient
	osvHTTPClient = &http.Client{
		Transport: rewriteTransport{ts.URL},
	}
	defer func() { osvHTTPClient = orig }()

	status, vulns := CheckClosureOSV(context.Background(), "npm", []PkgVersion{
		{Name: "left-pad", Version: "1.3.0"},
		{Name: "is-odd", Version: "2.0.0"},
	})
	if status != OSVStatusClear {
		t.Errorf("expected clear, got %q", status)
	}
	if vulns != 0 {
		t.Errorf("expected 0 vulns, got %d", vulns)
	}
}

func TestCheckClosureOSVVulnerableResponse(t *testing.T) {
	// Mock OSV batch endpoint returning vulnerabilities.
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := osvBatchResponse{
			Results: []struct {
				Vulns []struct {
					ID string `json:"id"`
				} `json:"vulns"`
			}{
				{Vulns: []struct {
					ID string `json:"id"`
				}{{ID: "CVE-2024-12345"}, {ID: "CVE-2024-99999"}}},
				{Vulns: nil}, // clean
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
	}))
	defer ts.Close()

	orig := osvHTTPClient
	osvHTTPClient = &http.Client{
		Transport: rewriteTransport{ts.URL},
	}
	defer func() { osvHTTPClient = orig }()

	status, vulns := CheckClosureOSV(context.Background(), "npm", []PkgVersion{
		{Name: "vulnerable-pkg", Version: "1.0.0"},
		{Name: "clean-pkg", Version: "2.0.0"},
	})
	if status != OSVStatusVulnerable {
		t.Errorf("expected vulnerable, got %q", status)
	}
	if vulns != 2 {
		t.Errorf("expected 2 vulns, got %d", vulns)
	}
}

// rewriteTransport rewrites all requests to a test server URL, preserving the
// path but changing the host. This lets us test the hardcoded osv.dev URL.
type rewriteTransport struct {
	testServerURL string
}

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	// Rewrite to test server.
	req.URL.Scheme = "http"
	req.URL.Host = rt.testServerURL[len("http://"):]
	return http.DefaultTransport.RoundTrip(req)
}
