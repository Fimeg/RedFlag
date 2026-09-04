package localapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Fimeg/RedFlag/agent/internal/cache"
)

func approveHandler(t *testing.T, approve func(body []byte) (interface{}, error)) http.Handler {
	t.Helper()
	return newHandler(Options{Config: testConfig(t), LoadCache: func() (*cache.LocalCache, error) {
		return &cache.LocalCache{}, nil
	}, ApproveUpdate: approve})
}

func postApprove(t *testing.T, handler http.Handler, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/v1/actions/approve-update", strings.NewReader(body)))
	return rec
}

func TestApproveUpdateSuccess(t *testing.T) {
	var gotBody string
	handler := approveHandler(t, func(body []byte) (interface{}, error) {
		gotBody = string(body)
		return map[string]interface{}{"request_id": "req-9", "osv_status": "clear"}, nil
	})

	rec := postApprove(t, handler, `{"package_type":"dnf","package_name":"hyprutils"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(gotBody, "hyprutils") {
		t.Fatalf("callback body = %q, want raw request body", gotBody)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp["request_id"] != "req-9" {
		t.Fatalf("request_id = %v, want req-9", resp["request_id"])
	}
}

func TestApproveUpdateConflictIs409(t *testing.T) {
	handler := approveHandler(t, func([]byte) (interface{}, error) {
		return nil, fmt.Errorf("%w: gate refused", ErrApprovalConflict)
	})
	rec := postApprove(t, handler, `{}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "gate refused") {
		t.Fatalf("body = %s, want gate refusal detail", rec.Body.String())
	}
}

func TestApproveUpdateNoAuthorityIs503(t *testing.T) {
	handler := approveHandler(t, func([]byte) (interface{}, error) {
		return nil, fmt.Errorf("%w: no local authority", ErrApprovalUnavailable)
	})
	rec := postApprove(t, handler, `{}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestApproveUpdateNilCallbackIs503(t *testing.T) {
	handler := approveHandler(t, nil)
	rec := postApprove(t, handler, `{}`)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestApproveUpdateInternalErrorIs500(t *testing.T) {
	handler := approveHandler(t, func([]byte) (interface{}, error) {
		return nil, errors.New("dry run exploded")
	})
	rec := postApprove(t, handler, `{}`)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestApproveUpdateRejectsGet(t *testing.T) {
	handler := approveHandler(t, func([]byte) (interface{}, error) { return nil, nil })
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/v1/actions/approve-update", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want 405", rec.Code)
	}
}
