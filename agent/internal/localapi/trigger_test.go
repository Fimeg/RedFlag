package localapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Fimeg/RedFlag/agent/internal/cache"
)

func triggerHandler(t *testing.T, trigger func(source string) error) http.Handler {
	t.Helper()
	return newHandler(Options{Config: testConfig(t), LoadCache: func() (*cache.LocalCache, error) {
		return &cache.LocalCache{}, nil
	}, TriggerScan: trigger})
}

func postTrigger(t *testing.T, handler http.Handler) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/actions/trigger-scan", nil))
	return rec
}

func TestTriggerScanAccepted(t *testing.T) {
	var gotSource string
	handler := triggerHandler(t, func(source string) error {
		gotSource = source
		return nil
	})

	rec := postTrigger(t, handler)
	if rec.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d; body=%s", rec.Code, http.StatusAccepted, rec.Body.String())
	}
	if gotSource != "localapi" {
		t.Fatalf("source = %q, want localapi", gotSource)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["accepted"] != true {
		t.Fatalf("accepted = %v, want true", resp["accepted"])
	}
}

func TestTriggerScanInFlightIsConflict(t *testing.T) {
	handler := triggerHandler(t, func(string) error { return ErrScanInFlight })

	rec := postTrigger(t, handler)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusConflict)
	}
}

func TestTriggerScanUnavailableWithoutCallback(t *testing.T) {
	handler := triggerHandler(t, nil)

	rec := postTrigger(t, handler)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
	}
}

func TestTriggerScanFailureIs500(t *testing.T) {
	handler := triggerHandler(t, func(string) error { return errors.New("boom") })

	rec := postTrigger(t, handler)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
}

func TestTriggerScanRejectsGet(t *testing.T) {
	handler := triggerHandler(t, func(string) error { return nil })

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/actions/trigger-scan", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
	}
	if rec.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("Allow = %q, want POST", rec.Header().Get("Allow"))
	}
}
