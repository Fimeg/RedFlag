package localapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

const localHTTPBaseURL = "http://redflag.local"

// ClientOptions configures local IPC client access. Defaults mirror the agent
// listener and remain platform-specific.
type ClientOptions struct {
	UnixSocketPath  string
	WindowsPipeName string
	Timeout         time.Duration
}

// Snapshot is the compact local state needed by status probes and Desktop.
type Snapshot struct {
	Identity IdentityResponse `json:"identity"`
	Status   StatusResponse   `json:"status"`
}

// FetchSnapshot reads the local agent API over the platform IPC transport.
func FetchSnapshot(ctx context.Context, opts ClientOptions) (*Snapshot, error) {
	httpClient := newLocalHTTPClient(opts)

	identity, err := fetchJSON[IdentityResponse](ctx, httpClient, "/v1/identity")
	if err != nil {
		return nil, fmt.Errorf("fetch identity: %w", err)
	}
	status, err := fetchJSON[StatusResponse](ctx, httpClient, "/v1/status")
	if err != nil {
		return nil, fmt.Errorf("fetch status: %w", err)
	}

	return &Snapshot{
		Identity: identity,
		Status:   status,
	}, nil
}

func newLocalHTTPClient(opts ClientOptions) *http.Client {
	timeout := opts.Timeout
	if timeout == 0 {
		timeout = 5 * time.Second
	}
	return &http.Client{
		Timeout: timeout,
		Transport: &http.Transport{
			DialContext:       dialLocalContext(opts),
			DisableKeepAlives: true,
			Proxy:             nil,
		},
	}
}

func fetchJSON[T any](ctx context.Context, httpClient *http.Client, path string) (T, error) {
	var value T

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, localHTTPBaseURL+path, nil)
	if err != nil {
		return value, err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return value, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return value, fmt.Errorf("local API returned %s", resp.Status)
	}
	if err := json.NewDecoder(resp.Body).Decode(&value); err != nil {
		return value, err
	}
	return value, nil
}

func dialLocalContext(opts ClientOptions) func(context.Context, string, string) (net.Conn, error) {
	return platformDialLocalContext(opts)
}
