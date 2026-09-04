package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/capability"
	"github.com/Fimeg/RedFlag/agent/internal/models"
	"github.com/Fimeg/RedFlag/agent/internal/system"
	"github.com/gofrs/uuid/v5"
)

// Auth sentinel errors. The polling loop branches on these via errors.Is rather
// than matching status text, so a change in error formatting can't silently
// disable token renewal.
//
//   - ErrUnauthorized: the access token (JWT) was rejected — expected at the
//     24h expiry boundary. Recoverable: renew with the refresh token.
//   - ErrRefreshTokenInvalid: the refresh token itself was rejected (expired,
//     revoked, or machine unbound). Terminal — no auto-recovery, the agent
//     must be re-registered. Distinct so operators can tell it apart from a
//     transient renewal failure (network, 502).
var (
	ErrUnauthorized        = errors.New("unauthorized: access token rejected")
	ErrRefreshTokenInvalid = errors.New("unauthorized: refresh token rejected")
	// ErrMachineMismatch: the server rejected us because our machine ID doesn't
	// match the one this agent registered with (403). Terminal — this identity
	// has been moved or copied to another host. Renewing won't help; a human must
	// re-register. Distinct so the loop can alarm instead of silently retrying.
	ErrMachineMismatch = errors.New("forbidden: machine ID mismatch")
)

// Client handles API communication with the server
type Client struct {
	baseURL             string
	token               string
	http                *http.Client
	RapidPollingEnabled bool
	RapidPollingUntil   time.Time
	machineID           string // Cached machine ID for security binding
	refreshToken        string // Most recent refresh token (rotated on each renew, migration 045)
}

// newHTTPClient returns an *http.Client with the given timeout and a transport
// tuned for repeated calls to the RedFlag server. The transport is cloned from
// http.DefaultTransport so Proxy, TLS settings, and all other defaults are
// inherited unchanged; only idle-connection pool limits are raised above the
// DefaultTransport default of MaxIdleConnsPerHost=2, which causes constant
// connection churn under sustained polling.
//
//	MaxIdleConns=100         — global cap; headroom without unbounded growth.
//	MaxIdleConnsPerHost=10   — the agent talks to one server; 10 keeps a warm
//	                           pool across rapid-poll bursts without wasting FDs.
//	IdleConnTimeout=90s      — matches http.DefaultTransport's own default.
func newHTTPClient(timeout time.Duration) *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.MaxIdleConns = 100
	t.MaxIdleConnsPerHost = 10
	t.IdleConnTimeout = 90 * time.Second
	return &http.Client{
		Timeout:   timeout,
		Transport: t,
	}
}

// NewClient creates a new API client
func NewClient(baseURL, token string) *Client {
	// Get machine ID for security binding (v0.1.22+)
	machineID, err := system.GetMachineID()
	if err != nil {
		// Log warning but don't fail — older servers may not require it
		log.Printf("[WARNING] [agent] [client] machine_id_error error=%q", err)
		machineID = "" // Will be handled by server validation
	}

	return &Client{
		baseURL:   baseURL,
		token:     token,
		machineID: machineID,
		http:      newHTTPClient(30 * time.Second),
	}
}

// ReportEvents sends buffered events to the server [TD-003]
// Returns (accepted, rejected, error)
func (c *Client) ReportEvents(agentID uuid.UUID, events []*models.SystemEvent) (int, int, error) {
	if len(events) == 0 {
		return 0, 0, nil
	}

	url := fmt.Sprintf("%s/api/v1/agents/%s/events", c.baseURL, agentID.String())

	body, err := json.Marshal(map[string]interface{}{"events": events})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to marshal events: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to send events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, 0, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var result struct {
		Accepted int      `json:"accepted"`
		Rejected int      `json:"rejected"`
		Errors   []string `json:"errors,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, 0, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Accepted, result.Rejected, nil
}

// AgentSecurityEvent is the wire format for security events sent from agent
// to server. The server handler maps this onto its SecurityEvent model,
// stamping AgentID from the URL parameter.
type AgentSecurityEvent struct {
	Timestamp time.Time              `json:"timestamp"`
	Level     string                 `json:"level"`
	EventType string                 `json:"event_type"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// ReportSecurityEvents sends buffered security events to the server.
// POST /api/v1/agents/:id/security-events
// Returns (accepted, rejected, error).
func (c *Client) ReportSecurityEvents(agentID uuid.UUID, events []AgentSecurityEvent) (int, int, error) {
	if len(events) == 0 {
		return 0, 0, nil
	}

	url := fmt.Sprintf("%s/api/v1/agents/%s/security-events", c.baseURL, agentID.String())

	body, err := json.Marshal(map[string]interface{}{"events": events})
	if err != nil {
		return 0, 0, fmt.Errorf("failed to marshal security events: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return 0, 0, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to send security events: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return 0, 0, fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	var result struct {
		Accepted int      `json:"accepted"`
		Rejected int      `json:"rejected"`
		Errors   []string `json:"errors,omitempty"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, 0, fmt.Errorf("failed to decode response: %w", err)
	}

	return result.Accepted, result.Rejected, nil
}

// ReportInventory sends inventory data to the server.
// POST /api/v1/agents/:id/inventory
func (c *Client) ReportInventory(agentID uuid.UUID, report InventoryReport) error {
	url := fmt.Sprintf("%s/api/v1/agents/%s/inventory", c.baseURL, agentID.String())

	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to marshal inventory report: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send inventory report: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(bodyBytes))
	}

	return nil
}

// addMachineIDHeader adds X-Machine-ID header to authenticated requests (v0.1.22+)
func (c *Client) addMachineIDHeader(req *http.Request) {
	if c.machineID != "" {
		req.Header.Set("X-Machine-ID", c.machineID)
	}
}

// GetToken returns the current JWT token
func (c *Client) GetToken() string {
	return c.token
}

// SetToken updates the JWT token
func (c *Client) SetToken(token string) {
	c.token = token
}

// GetRefreshToken returns the most recent refresh token. Empty unless a renewal
// has rotated one this process lifetime; the caller persists it to config.
func (c *Client) GetRefreshToken() string {
	return c.refreshToken
}

// DownloadAuthenticatedToFile fetches a relative or absolute URL using the agent's
// JWT + machine binding and streams the body into dstPath. maxBytes caps the size to
// guard against a misbehaving server filling the disk. Returns the number of bytes
// written, or an error.
//
// Relative URLs (those beginning with "/") are resolved against the configured server.
// Used by the agent self-update handler — the /api/v1/downloads/updates/:package_id
// route is auth-protected, so an unauthenticated http.Get would 401.
func (c *Client) DownloadAuthenticatedToFile(rawURL, dstPath string, maxBytes int64) (int64, error) {
	target := rawURL
	if strings.HasPrefix(rawURL, "/") {
		base, err := url.Parse(c.baseURL)
		if err != nil {
			return 0, fmt.Errorf("parse base url: %w", err)
		}
		rel, err := url.Parse(rawURL)
		if err != nil {
			return 0, fmt.Errorf("parse download url: %w", err)
		}
		target = base.ResolveReference(rel).String()
	}

	req, err := http.NewRequest("GET", target, nil)
	if err != nil {
		return 0, fmt.Errorf("build download request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return 0, fmt.Errorf("download request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return 0, fmt.Errorf("download failed: status=%d body=%q", resp.StatusCode, string(bodyBytes))
	}

	dst, err := os.Create(dstPath)
	if err != nil {
		return 0, fmt.Errorf("create destination: %w", err)
	}
	defer dst.Close()

	limit := io.LimitReader(resp.Body, maxBytes)
	written, err := io.Copy(dst, limit)
	if err != nil {
		return written, fmt.Errorf("write download: %w", err)
	}

	// Detect truncation: if we hit exactly maxBytes, the source may have been longer.
	// Probe by reading one more byte.
	probe := make([]byte, 1)
	if n, _ := resp.Body.Read(probe); n > 0 {
		return written, fmt.Errorf("download exceeded max bytes %d", maxBytes)
	}

	return written, nil
}

// RegisterRequest is the payload for agent registration
type RegisterRequest struct {
	Hostname             string            `json:"hostname"`
	OSType               string            `json:"os_type"`
	OSVersion            string            `json:"os_version"`
	OSArchitecture       string            `json:"os_architecture"`
	AgentVersion         string            `json:"agent_version"`
	RegistrationToken    string            `json:"registration_token,omitempty"` // Fallback method
	MachineID            string            `json:"machine_id"`
	PublicKeyFingerprint string            `json:"public_key_fingerprint"`
	Metadata             map[string]string `json:"metadata"`
	AvailableScanners    []string          `json:"available_scanners"` // Platform-specific package managers (apt, dnf, winget, windows)
	DeviceType           string            `json:"device_type"`        // Auto-detected form factor (DEVICE-001)
	DeviceModel          string            `json:"device_model"`       // Hardware model string
	OSDistro             string            `json:"os_distro"`          // Distro ID from /etc/os-release
}

// RegisterResponse is returned after successful registration
type RegisterResponse struct {
	AgentID      uuid.UUID              `json:"agent_id"`
	Token        string                 `json:"token"`         // Short-lived access token (24h)
	RefreshToken string                 `json:"refresh_token"` // Long-lived refresh token (90d)
	Config       map[string]interface{} `json:"config"`
}

// Register registers the agent with the server
func (c *Client) Register(req RegisterRequest) (*RegisterResponse, error) {
	url := fmt.Sprintf("%s/api/v1/agents/register", c.baseURL)

	// If we have a registration token, include it in the request
	// Registration tokens are longer than regular JWT tokens (usually 64 chars vs JWT ~400 chars)
	if c.token != "" && len(c.token) > 40 {
		req.RegistrationToken = c.token
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	// Add Authorization header if we have a registration token (preferred method)
	// Registration tokens are longer than regular JWT tokens (usually 64 chars vs JWT ~400 chars)
	if c.token != "" && len(c.token) > 40 {
		httpReq.Header.Set("Authorization", "Bearer "+c.token)
	}

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		errorMsg := fmt.Sprintf("registration failed: %s - %s", resp.Status, string(bodyBytes))
		return nil, fmt.Errorf("%s", errorMsg)
	}

	var result RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Update client token
	c.token = result.Token

	return &result, nil
}

// TokenRenewalRequest is the payload for token renewal using refresh token
type TokenRenewalRequest struct {
	AgentID      uuid.UUID `json:"agent_id"`
	RefreshToken string    `json:"refresh_token"`
	AgentVersion string    `json:"agent_version,omitempty"` // Agent's current version for upgrade tracking
}

// TokenRenewalResponse is returned after successful token renewal
type TokenRenewalResponse struct {
	Token        string `json:"token"`         // New short-lived access token (24h)
	RefreshToken string `json:"refresh_token"` // Rotated refresh token (migration 045) — must be persisted
}

// RenewToken uses refresh token to get a new access token (proper implementation)
func (c *Client) RenewToken(agentID uuid.UUID, refreshToken string, agentVersion string) error {
	url := fmt.Sprintf("%s/api/v1/agents/renew", c.baseURL)

	renewalReq := TokenRenewalRequest{
		AgentID:      agentID,
		RefreshToken: refreshToken,
		AgentVersion: agentVersion,
	}

	body, err := json.Marshal(renewalReq)
	if err != nil {
		return err
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	c.addMachineIDHeader(httpReq) // Renewal is machine-bound: a refresh token only works from the registered host.

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		errorMsg := fmt.Sprintf("token renewal failed: %s - %s", resp.Status, string(bodyBytes))

		// A 401/403 on the renew endpoint means the refresh token itself is no
		// longer valid (expired, revoked, or machine unbound). That's terminal —
		// no amount of retrying recovers it, the agent must be re-registered.
		// Wrap the terminal sentinel so the loop can react and buffer the
		// operational event through its own producer-owned event buffer.
		if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
			return fmt.Errorf("%w: %s", ErrRefreshTokenInvalid, string(bodyBytes))
		}
		return fmt.Errorf("%s", errorMsg)
	}

	var result TokenRenewalResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	// Update client token, and the rotated refresh token if the server sent one.
	// The caller reads GetRefreshToken() and persists it to config; if persistence
	// fails or the agent crashes first, the server's accept-previous-once grace
	// lets the next attempt with the old token recover.
	c.token = result.Token
	if result.RefreshToken != "" {
		c.refreshToken = result.RefreshToken
	}

	return nil
}

// Command represents a command from the server
type Command struct {
	ID        string                 `json:"id"`
	Type      string                 `json:"type"`
	Params    map[string]interface{} `json:"params"`
	Signature string                 `json:"signature,omitempty"`  // Ed25519 signature of the command
	KeyID     string                 `json:"key_id,omitempty"`     // Fingerprint of the signing key used
	SignedAt  *time.Time             `json:"signed_at,omitempty"`  // Timestamp when command was signed
	AgentID   string                 `json:"agent_id,omitempty"`   // Target agent ID (F-1 fix: included in signed payload)
	CreatedAt *time.Time             `json:"created_at,omitempty"` // Server-side creation time (F-3 fix: old-format expiry)
}

// CommandItem is an alias for Command for consistency with server models
type CommandItem = Command

// CommandsResponse contains pending commands
type CommandsResponse struct {
	Commands            []Command           `json:"commands"`
	RapidPolling        *RapidPollingConfig `json:"rapid_polling,omitempty"`
	AcknowledgedIDs     []string            `json:"acknowledged_ids,omitempty"`      // Result IDs server has recorded (drop from pending_acks)
	ReceiptConfirmedIDs []string            `json:"receipt_confirmed_ids,omitempty"` // Command IDs server flipped sent→received (drop from outbound receipts)
	ConfirmedCommandIDs []string            `json:"confirmed_command_ids,omitempty"` // Command IDs server confirmed as completed (via ReportLog)
}

// RapidPollingConfig contains rapid polling configuration from server
type RapidPollingConfig struct {
	Enabled bool   `json:"enabled"`
	Until   string `json:"until"` // ISO 8601 timestamp
}

// SystemMetrics represents lightweight system metrics sent with check-ins
type SystemMetrics struct {
	CPUPercent    float64                `json:"cpu_percent,omitempty"`
	MemoryPercent float64                `json:"memory_percent,omitempty"`
	MemoryUsedGB  float64                `json:"memory_used_gb,omitempty"`
	MemoryTotalGB float64                `json:"memory_total_gb,omitempty"`
	DiskUsedGB    float64                `json:"disk_used_gb,omitempty"`
	DiskTotalGB   float64                `json:"disk_total_gb,omitempty"`
	DiskPercent   float64                `json:"disk_percent,omitempty"`
	Uptime        string                 `json:"uptime,omitempty"`
	Version       string                 `json:"version,omitempty"`  // Agent version
	Metadata      map[string]interface{} `json:"metadata,omitempty"` // Additional metadata

	// Command acknowledgment tracking
	PendingAcknowledgments []string `json:"pending_acknowledgments,omitempty"` // Command result IDs awaiting server ACK

	// Receipt confirmation (Migration 033 §2): command IDs the agent has received but
	// not yet completed. Server flips these sent→received and returns them in
	// ReceiptConfirmedIDs, at which point the agent drops them from its outbound buffer.
	ReceivedCommandIDs []string `json:"received_command_ids,omitempty"`

	// Capability advertisement (ARC-001): scanners present on this host right now.
	// Server diffs this against agent_subsystems each poll so newly-installed
	// scanners (e.g. Docker added post-registration) get scheduled.
	AvailableScanners []string `json:"available_scanners,omitempty"`
}

// GetCommands retrieves pending commands from the server
// Optionally sends lightweight system metrics in the request
// Returns the full response including commands and acknowledged IDs
func (c *Client) GetCommands(agentID uuid.UUID, metrics *SystemMetrics) (*CommandsResponse, error) {
	url := fmt.Sprintf("%s/api/v1/agents/%s/commands", c.baseURL, agentID)

	var req *http.Request
	var err error

	// If metrics provided, send them in request body
	if metrics != nil {
		body, err := json.Marshal(metrics)
		if err != nil {
			return nil, err
		}
		req, err = http.NewRequest("GET", url, bytes.NewBuffer(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
	} else {
		req, err = http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req) // Security: Validate machine binding (v0.1.22+)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("%w: %s", ErrUnauthorized, string(bodyBytes))
		}
		if resp.StatusCode == http.StatusForbidden {
			// Machine binding rejected us — this config is being used from a host
			// it wasn't registered on. Return a sentinel; the loop owns buffering
			// the critical operational event through the agent event buffer.
			return nil, fmt.Errorf("%w: %s", ErrMachineMismatch, string(bodyBytes))
		}
		return nil, fmt.Errorf("failed to get commands: %s - %s", resp.Status, string(bodyBytes))
	}

	var result CommandsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	// Handle rapid polling configuration if provided
	if result.RapidPolling != nil {
		// Parse the timestamp
		if until, err := time.Parse(time.RFC3339, result.RapidPolling.Until); err == nil {
			// Update client's rapid polling configuration
			c.RapidPollingEnabled = result.RapidPolling.Enabled
			c.RapidPollingUntil = until
		}
	}

	return &result, nil
}

// CapabilityTokensResponse is the server's reply for the capability-token feed.
type CapabilityTokensResponse struct {
	Tokens []*capability.Token `json:"tokens"`
}

// GetCapabilityTokens fetches this agent's minted-but-undelivered capability
// tokens. The agent must bind-check each token's agent_id and hand it to the
// privileged executor, which verifies signature and artifact hashes before
// acting. The agent holds no signing key and makes no allow/deny decision.
func (c *Client) GetCapabilityTokens(agentID uuid.UUID) ([]*capability.Token, error) {
	url := fmt.Sprintf("%s/api/v1/agents/%s/capability-tokens", c.baseURL, agentID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusServiceUnavailable {
		// Gate not enabled server-side; not an error for the agent.
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get capability tokens: %s - %s", resp.Status, string(bodyBytes))
	}

	var result CapabilityTokensResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Tokens, nil
}

// ReportCapabilityResult posts an audit receipt for a processed capability token.
// Best-effort: the executor's local replay guard is authoritative on single use,
// so a failed receipt does not change install correctness.
func (c *Client) ReportCapabilityResult(agentID uuid.UUID, tokenID string, decision, reason string, exitCode int) error {
	url := fmt.Sprintf("%s/api/v1/agents/%s/capability-tokens/%s/receipt", c.baseURL, agentID, tokenID)

	body, err := json.Marshal(map[string]interface{}{
		"decision":  decision,
		"reason":    reason,
		"exit_code": exitCode,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to report capability result: %s - %s", resp.Status, string(bodyBytes))
	}
	return nil
}

// UpdateReport represents discovered updates.
// RECONCILE-001 contract extension (2026-06-06):
//   - Ecosystem: the package manager that produced this scan (e.g. "dnf", "apt").
//     Must be set when ScanSucceeded is true so the server can scope the set-diff
//     closure to a single (agent, ecosystem) pair.
//   - ScanSucceeded: set true only when the scanner returned exit 0 and a complete
//     result. Must be false for partial, failed, or errored scans. The server NEVER
//     closes rows by absence unless this is true.
//   - An empty Updates slice with ScanSucceeded=true is valid: it means the
//     ecosystem has nothing pending, and the server should close all tracked rows.
type UpdateReport struct {
	CommandID     string             `json:"command_id"`
	Timestamp     time.Time          `json:"timestamp"`
	Updates       []UpdateReportItem `json:"updates"`
	Ecosystem     string             `json:"ecosystem,omitempty"`      // RECONCILE-001
	ScanSucceeded bool               `json:"scan_succeeded,omitempty"` // RECONCILE-001
}

// UpdateReportItem represents a single update
type UpdateReportItem struct {
	PackageType        string                 `json:"package_type"`
	PackageName        string                 `json:"package_name"`
	PackageDescription string                 `json:"package_description"`
	CurrentVersion     string                 `json:"current_version"`
	AvailableVersion   string                 `json:"available_version"`
	Severity           string                 `json:"severity"`
	CVEList            []string               `json:"cve_list"`
	KBID               string                 `json:"kb_id"`
	RepositorySource   string                 `json:"repository_source"`
	SizeBytes          int64                  `json:"size_bytes"`
	Metadata           map[string]interface{} `json:"metadata"`
}

// ReportUpdates sends discovered updates to the server
func (c *Client) ReportUpdates(agentID uuid.UUID, report UpdateReport) error {
	url := fmt.Sprintf("%s/api/v1/agents/%s/updates", c.baseURL, agentID)

	body, err := json.Marshal(report)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req) // Security: Validate machine binding (v0.1.22+)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to report updates: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// MetricsReport represents metrics data (storage, system, CPU, memory)
type MetricsReport struct {
	CommandID string              `json:"command_id"`
	Timestamp time.Time           `json:"timestamp"`
	Metrics   []MetricsReportItem `json:"metrics"`
}

// MetricsReportItem represents a single metric
type MetricsReportItem struct {
	PackageType      string                 `json:"package_type"`
	PackageName      string                 `json:"package_name"`
	CurrentVersion   string                 `json:"current_version"`
	AvailableVersion string                 `json:"available_version"`
	Severity         string                 `json:"severity"`
	RepositorySource string                 `json:"repository_source"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// ReportMetrics sends metrics data to the server
func (c *Client) ReportMetrics(agentID uuid.UUID, report MetricsReport) error {
	url := fmt.Sprintf("%s/api/v1/agents/%s/metrics", c.baseURL, agentID)

	body, err := json.Marshal(report)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req) // Security: Validate machine binding (v0.1.22+)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to report metrics: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// DockerReport represents Docker image information
type DockerReport struct {
	CommandID     string                  `json:"command_id"`
	Timestamp     time.Time               `json:"timestamp"`
	Images        []DockerReportItem      `json:"images"`
	Containers    []DockerReportContainer `json:"containers,omitempty"`
	Stacks        []DockerReportStack     `json:"stacks,omitempty"`
	EngineVersion string                  `json:"engine_version,omitempty"`
}

// DockerReportContainer represents a running/stopped container reported by the agent.
type DockerReportContainer struct {
	ContainerID string            `json:"container_id"`
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	ImageID     string            `json:"image_id"`
	State       string            `json:"state"`                // running, stopped, paused, etc.
	Health      string            `json:"health"`               // healthy, unhealthy, starting, ""
	StackName   string            `json:"stack_name,omitempty"` // compose stack label
	Ports       string            `json:"ports,omitempty"`      // "0.0.0.0:80->80/tcp"
	CreatedAt   int64             `json:"created_at"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// DockerReportStack represents a Docker Compose stack derived from container labels.
type DockerReportStack struct {
	Name           string `json:"name"`
	ContainerCount int    `json:"container_count"`
	RunningCount   int    `json:"running_count"`
}

// DockerReportItem represents a single Docker image
type DockerReportItem struct {
	PackageType      string                 `json:"package_type"`
	PackageName      string                 `json:"package_name"`
	CurrentVersion   string                 `json:"current_version"`
	AvailableVersion string                 `json:"available_version"`
	Severity         string                 `json:"severity"`
	RepositorySource string                 `json:"repository_source"`
	Metadata         map[string]interface{} `json:"metadata"`
}

// ReportDockerImages sends Docker image information to the server
func (c *Client) ReportDockerImages(agentID uuid.UUID, report DockerReport) error {
	url := fmt.Sprintf("%s/api/v1/agents/%s/docker-images", c.baseURL, agentID)

	body, err := json.Marshal(report)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req) // Security: Validate machine binding (v0.1.22+)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to report docker images: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// ReportStorageMetrics sends storage metrics to the server via dedicated endpoint
func (c *Client) ReportStorageMetrics(agentID uuid.UUID, report models.StorageMetricReport) error {
	url := fmt.Sprintf("%s/api/v1/agents/%s/storage-metrics", c.baseURL, agentID)

	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to marshal storage metrics: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req) // Security: Validate machine binding (v0.1.22+)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to report storage metrics: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// ProcessScanReport represents a full process scan result
type ProcessScanReport struct {
	AgentID   uuid.UUID                  `json:"agent_id"`
	CommandID string                     `json:"command_id"`
	Timestamp time.Time                  `json:"timestamp"`
	Snapshot  system.FullProcessSnapshot `json:"snapshot"`
}

// ReportProcessScan sends a full process scan to the server via dedicated endpoint
func (c *Client) ReportProcessScan(agentID uuid.UUID, report ProcessScanReport) error {
	url := fmt.Sprintf("%s/api/v1/agents/%s/process-scan", c.baseURL, agentID)

	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to marshal process scan: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to report process scan: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// LogReport represents an execution log
type LogReport struct {
	CommandID       string            `json:"command_id"`
	Action          string            `json:"action"`
	Result          string            `json:"result"`
	Stdout          string            `json:"stdout"`
	Stderr          string            `json:"stderr"`
	ExitCode        int               `json:"exit_code"`
	DurationSeconds int               `json:"duration_seconds"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// ReportLog sends an execution log to the server
func (c *Client) ReportLog(agentID uuid.UUID, report LogReport) error {
	url := fmt.Sprintf("%s/api/v1/agents/%s/logs", c.baseURL, agentID)

	// Extract subsystem from metadata if present
	subsystem := ""
	if report.Metadata != nil {
		subsystem = report.Metadata["subsystem"]
	}

	// Create UpdateLogRequest with subsystem extracted from metadata
	logRequest := struct {
		CommandID       string `json:"command_id"`
		Action          string `json:"action"`
		Subsystem       string `json:"subsystem,omitempty"`
		Result          string `json:"result"`
		Stdout          string `json:"stdout"`
		Stderr          string `json:"stderr"`
		ExitCode        int    `json:"exit_code"`
		DurationSeconds int    `json:"duration_seconds"`
	}{
		CommandID:       report.CommandID,
		Action:          report.Action,
		Subsystem:       subsystem,
		Result:          report.Result,
		Stdout:          report.Stdout,
		Stderr:          report.Stderr,
		ExitCode:        report.ExitCode,
		DurationSeconds: report.DurationSeconds,
	}

	body, err := json.Marshal(logRequest)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req) // Security: Validate machine binding (v0.1.22+)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to report log: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// DependencyReport represents a dependency report after dry run
type DependencyReport struct {
	PackageName   string         `json:"package_name"`
	PackageType   string         `json:"package_type"`
	TargetVersion string         `json:"target_version,omitempty"`
	Dependencies  []string       `json:"dependencies"`
	UpdateID      string         `json:"update_id"`
	DryRunResult  *InstallResult `json:"dry_run_result,omitempty"`
	// Closure carries the resolved artifacts (top-level + dependencies) with the
	// SHA256 the agent's signed repo metadata anchors. The server pins these and
	// mints the capability token over them. Empty for package types whose hash is
	// sourced server-side (npm/PyPI) or that do not support resolution.
	Closure []ClosureItem `json:"closure,omitempty"`
}

// ClosureItem is one resolved artifact reported to the server. JSON tags match
// the server's capability.ClosureEntry so the reported set maps directly onto a
// minted token's closure.
type ClosureItem struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	SHA256  string `json:"sha256"`
	Source  string `json:"source"`
}

// InstallResult represents the result of a package installation attempt
type InstallResult struct {
	Success           bool     `json:"success"`
	ErrorMessage      string   `json:"error_message,omitempty"`
	Stdout            string   `json:"stdout,omitempty"`
	Stderr            string   `json:"stderr,omitempty"`
	ExitCode          int      `json:"exit_code"`
	DurationSeconds   int      `json:"duration_seconds"`
	Action            string   `json:"action,omitempty"`
	PackagesInstalled []string `json:"packages_installed,omitempty"`
	ContainersUpdated []string `json:"containers_updated,omitempty"`
	Dependencies      []string `json:"dependencies,omitempty"`
	IsDryRun          bool     `json:"is_dry_run"`
}

// ReportDependencies sends dependency report to the server
func (c *Client) ReportDependencies(agentID uuid.UUID, report DependencyReport) error {
	url := fmt.Sprintf("%s/api/v1/agents/%s/dependencies", c.baseURL, agentID)

	body, err := json.Marshal(report)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req) // Security: Validate machine binding (v0.1.22+)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to report dependencies: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// SystemInfoReport represents system information updates
type SystemInfoReport struct {
	Timestamp   time.Time              `json:"timestamp"`
	CPUModel    string                 `json:"cpu_model,omitempty"`
	CPUCores    int                    `json:"cpu_cores,omitempty"`
	CPUThreads  int                    `json:"cpu_threads,omitempty"`
	MemoryTotal uint64                 `json:"memory_total,omitempty"`
	DiskTotal   uint64                 `json:"disk_total,omitempty"`
	DiskUsed    uint64                 `json:"disk_used,omitempty"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	Processes   int                    `json:"processes,omitempty"`
	Uptime      string                 `json:"uptime,omitempty"`
	DeviceType  string                 `json:"device_type,omitempty"`  // Re-reported so reinstalls reclassify (DEVICE-001)
	DeviceModel string                 `json:"device_model,omitempty"`
	OSDistro    string                 `json:"os_distro,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

// ReportSystemInfo sends updated system information to the server
func (c *Client) ReportSystemInfo(agentID uuid.UUID, report SystemInfoReport) error {
	url := fmt.Sprintf("%s/api/v1/agents/%s/system-info", c.baseURL, agentID)

	body, err := json.Marshal(report)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req) // Security: Validate machine binding (v0.1.22+)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Accept 200 OK or 404 Not Found (if endpoint doesn't exist yet)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to report system info: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// CircuitBreakerReport represents circuit breaker health status
// [ISSUE-004] Added for circuit breaker monitoring and alerting
type CircuitBreakerReport struct {
	Timestamp  time.Time              `json:"timestamp"`
	Subsystems []CircuitBreakerStatus `json:"subsystems"`
}

// CircuitBreakerStatus represents the state of a single circuit breaker
type CircuitBreakerStatus struct {
	Name               string     `json:"name"`
	State              string     `json:"state"` // closed, open, half-open
	RecentFailures     int        `json:"recent_failures"`
	ConsecutiveSuccess int        `json:"consecutive_success"`
	NextAttempt        *time.Time `json:"next_attempt,omitempty"`
}

// ReportCircuitBreakerStats sends circuit breaker health to the server
// [ISSUE-004] Enables server-side monitoring and alerting for circuit breaker states
func (c *Client) ReportCircuitBreakerStats(agentID uuid.UUID, report CircuitBreakerReport) error {
	url := fmt.Sprintf("%s/api/v1/agents/%s/circuit-breakers", c.baseURL, agentID)

	body, err := json.Marshal(report)
	if err != nil {
		return fmt.Errorf("failed to marshal circuit breaker report: %w", err)
	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req) // Security: Validate machine binding (v0.1.22+)

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	// Accept 200 OK or 404 Not Found (if endpoint doesn't exist yet on older servers)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to report circuit breaker stats: %s - %s", resp.Status, string(bodyBytes))
	}

	return nil
}

// DetectSystem returns basic system information (deprecated, use system.GetSystemInfo instead)
func DetectSystem() (osType, osVersion, osArch string) {
	osType = runtime.GOOS
	osArch = runtime.GOARCH

	// Read OS version
	switch osType {
	case "linux":
		data, _ := os.ReadFile("/etc/os-release")
		if data != nil {
			osVersion = parseOSRelease(data)
		}
	case "windows":
		osVersion = "Windows"
	case "darwin":
		osVersion = "macOS"
	}

	return
}

// AgentInfo represents agent information from the server
type AgentInfo struct {
	ID             string `json:"id"`
	Hostname       string `json:"hostname"`
	CurrentVersion string `json:"current_version"`
	OSType         string `json:"os_type"`
	OSVersion      string `json:"os_version"`
	OSArchitecture string `json:"os_architecture"`
	LastCheckIn    string `json:"last_check_in"`
}

// GetAgent retrieves agent information from the server
func (c *Client) GetAgent(agentID string) (*AgentInfo, error) {
	url := fmt.Sprintf("%s/api/v1/agents/%s", c.baseURL, agentID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	c.addMachineIDHeader(req) // Security: Validate machine binding (v0.1.22+)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned status %d: %s", resp.StatusCode, string(body))
	}

	var agent AgentInfo
	if err := json.NewDecoder(resp.Body).Decode(&agent); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return &agent, nil
}

// parseOSRelease parses /etc/os-release to get proper distro name
func parseOSRelease(data []byte) string {
	lines := strings.Split(string(data), "\n")
	id := ""
	prettyName := ""
	version := ""

	for _, line := range lines {
		if strings.HasPrefix(line, "ID=") {
			id = strings.Trim(strings.TrimPrefix(line, "ID="), "\"")
		}
		if strings.HasPrefix(line, "PRETTY_NAME=") {
			prettyName = strings.Trim(strings.TrimPrefix(line, "PRETTY_NAME="), "\"")
		}
		if strings.HasPrefix(line, "VERSION_ID=") {
			version = strings.Trim(strings.TrimPrefix(line, "VERSION_ID="), "\"")
		}
	}

	// Prefer PRETTY_NAME if available
	if prettyName != "" {
		return prettyName
	}

	// Fall back to ID + VERSION
	if id != "" {
		if version != "" {
			return strings.Title(id) + " " + version
		}
		return strings.Title(id)
	}

	return "Linux"
}

// AgentConfigResponse contains configuration delivered by the server.
type AgentConfigResponse struct {
	Subsystems     map[string]interface{}        `json:"subsystems"`
	Polling        *PollingConfigResponse        `json:"polling,omitempty"`
	CommandSigning *CommandSigningConfigResponse `json:"command_signing,omitempty"`
	Version        int64                         `json:"version"`
}

// CommandSigningConfigResponse carries fleet-wide command-signing policy from
// the server. The agent clamps StaleKeyMaxAgeHours to its doctrinal range.
type CommandSigningConfigResponse struct {
	StaleKeyMaxAgeHours int `json:"stale_key_max_age_hours"`
}

// PollingConfigResponse carries fleet-wide polling resilience tuning from the
// server. The agent merges non-zero values into its local config.PollingConfig.
type PollingConfigResponse struct {
	JitterMaxSeconds   int `json:"jitter_max_seconds"`
	BackoffBaseSeconds int `json:"backoff_base_seconds"`
	BackoffMaxSeconds  int `json:"backoff_max_seconds"`
}

// GetConfig retrieves current subsystem configuration from server
func (c *Client) GetConfig(agentID uuid.UUID) (*AgentConfigResponse, error) {
	url := fmt.Sprintf("%s/api/v1/agents/%s/config", c.baseURL, agentID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+c.token)
	c.addMachineIDHeader(req) // Security: Validate machine binding (v0.1.22+)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get config: %s - %s", resp.Status, string(bodyBytes))
	}

	var result AgentConfigResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

// ActivePublicKeyEntry represents a single active key from the server's /api/v1/public-keys endpoint
type ActivePublicKeyEntry struct {
	KeyID     string `json:"key_id"`
	PublicKey string `json:"public_key"`
	IsPrimary bool   `json:"is_primary"`
	Version   int    `json:"version"`
	Algorithm string `json:"algorithm"`
}

// GetActivePublicKeys fetches all currently active public keys from the server.
// Used during key rotation to pre-cache new keys before they become the primary signing key.
func (c *Client) GetActivePublicKeys(serverURL string) ([]ActivePublicKeyEntry, error) {
	url := fmt.Sprintf("%s/api/v1/public-keys", serverURL)
	resp, err := c.http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch active public keys: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("server returned %d: %s", resp.StatusCode, string(body))
	}
	var keys []ActivePublicKeyEntry
	if err := json.NewDecoder(resp.Body).Decode(&keys); err != nil {
		return nil, fmt.Errorf("failed to decode public keys: %w", err)
	}
	return keys, nil
}

// GetExpectedHash retrieves the expected SHA256 hash for a package (Layer 1: Hash Registry)
func (c *Client) GetExpectedHash(packageType, packageName string, agentID uuid.UUID) (string, error) {
	url := fmt.Sprintf("%s/api/v1/updates/verify-hash?package_type=%s&package_name=%s&agent_id=%s",
		c.baseURL, packageType, packageName, agentID.String())

	resp, err := c.http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		var body struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(bodyBytes, &body); err == nil && body.Error != "" {
			return "", fmt.Errorf("%s", body.Error)
		}
		return "", fmt.Errorf("unexpected status: %d - %s", resp.StatusCode, string(bodyBytes))
	}

	var result struct {
		ExpectedSHA256 string `json:"expected_sha256"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	return result.ExpectedSHA256, nil
}

// GetWdacPolicyHash returns the WDAC policy hash (stub for Windows WDAC enforcement)
// This is a placeholder - actual WDAC policy fetching requires Windows-specific COM APIs
func (c *Client) GetWdacPolicyHash(agentID uuid.UUID, serverURL string) (string, error) {
	// WDAC not implemented yet - return empty hash
	// This causes fail-closed behavior in WDAC enforcer
	return "", fmt.Errorf("wdac_policy_not_implemented")
}
