package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Client handles API communication with the server
type Client struct {
	baseURL string
	token   string
	http    *http.Client
}

// NewClient creates a new API client
func NewClient(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
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

// RegisterRequest is the payload for agent registration
type RegisterRequest struct {
	Hostname       string            `json:"hostname"`
	OSType         string            `json:"os_type"`
	OSVersion      string            `json:"os_version"`
	OSArchitecture string            `json:"os_architecture"`
	AgentVersion   string            `json:"agent_version"`
	Metadata       map[string]string `json:"metadata"`
}

// RegisterResponse is returned after successful registration
type RegisterResponse struct {
	AgentID      uuid.UUID              `json:"agent_id"`
	Token        string                 `json:"token"`          // Short-lived access token (24h)
	RefreshToken string                 `json:"refresh_token"`  // Long-lived refresh token (90d)
	Config       map[string]interface{} `json:"config"`
}

// Register registers the agent with the server
func (c *Client) Register(req RegisterRequest) (*RegisterResponse, error) {
	url := fmt.Sprintf("%s/api/v1/agents/register", c.baseURL)

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("registration failed: %s - %s", resp.Status, string(bodyBytes))
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
}

// TokenRenewalResponse is returned after successful token renewal
type TokenRenewalResponse struct {
	Token string `json:"token"`  // New short-lived access token (24h)
}

// RenewToken uses refresh token to get a new access token (proper implementation)
func (c *Client) RenewToken(agentID uuid.UUID, refreshToken string) error {
	url := fmt.Sprintf("%s/api/v1/agents/renew", c.baseURL)

	renewalReq := TokenRenewalRequest{
		AgentID:      agentID,
		RefreshToken: refreshToken,
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

	resp, err := c.http.Do(httpReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("token renewal failed: %s - %s", resp.Status, string(bodyBytes))
	}

	var result TokenRenewalResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return err
	}

	// Update client token
	c.token = result.Token

	return nil
}

// Command represents a command from the server
type Command struct {
	ID     string                 `json:"id"`
	Type   string                 `json:"type"`
	Params map[string]interface{} `json:"params"`
}

// CommandsResponse contains pending commands
type CommandsResponse struct {
	Commands []Command `json:"commands"`
}

// SystemMetrics represents lightweight system metrics sent with check-ins
type SystemMetrics struct {
	CPUPercent    float64 `json:"cpu_percent,omitempty"`
	MemoryPercent float64 `json:"memory_percent,omitempty"`
	MemoryUsedGB  float64 `json:"memory_used_gb,omitempty"`
	MemoryTotalGB float64 `json:"memory_total_gb,omitempty"`
	DiskUsedGB    float64 `json:"disk_used_gb,omitempty"`
	DiskTotalGB   float64 `json:"disk_total_gb,omitempty"`
	DiskPercent   float64 `json:"disk_percent,omitempty"`
	Uptime        string  `json:"uptime,omitempty"`
	Version       string  `json:"version,omitempty"`        // Agent version
}

// GetCommands retrieves pending commands from the server
// Optionally sends lightweight system metrics in the request
func (c *Client) GetCommands(agentID uuid.UUID, metrics *SystemMetrics) ([]Command, error) {
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

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get commands: %s - %s", resp.Status, string(bodyBytes))
	}

	var result CommandsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return result.Commands, nil
}

// UpdateReport represents discovered updates
type UpdateReport struct {
	CommandID string               `json:"command_id"`
	Timestamp time.Time            `json:"timestamp"`
	Updates   []UpdateReportItem   `json:"updates"`
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

// LogReport represents an execution log
type LogReport struct {
	CommandID       string `json:"command_id"`
	Action          string `json:"action"`
	Result          string `json:"result"`
	Stdout          string `json:"stdout"`
	Stderr          string `json:"stderr"`
	ExitCode        int    `json:"exit_code"`
	DurationSeconds int    `json:"duration_seconds"`
}

// ReportLog sends an execution log to the server
func (c *Client) ReportLog(agentID uuid.UUID, report LogReport) error {
	url := fmt.Sprintf("%s/api/v1/agents/%s/logs", c.baseURL, agentID)

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
	PackageName   string        `json:"package_name"`
	PackageType   string        `json:"package_type"`
	Dependencies  []string      `json:"dependencies"`
	UpdateID      string        `json:"update_id"`
	DryRunResult  *InstallResult `json:"dry_run_result,omitempty"`
}

// InstallResult represents the result of a package installation attempt
type InstallResult struct {
	Success           bool     `json:"success"`
	ErrorMessage      string   `json:"error_message,omitempty"`
	Stdout           string   `json:"stdout,omitempty"`
	Stderr           string   `json:"stderr,omitempty"`
	ExitCode         int      `json:"exit_code"`
	DurationSeconds  int      `json:"duration_seconds"`
	Action           string   `json:"action,omitempty"`
	PackagesInstalled []string `json:"packages_installed,omitempty"`
	ContainersUpdated []string `json:"containers_updated,omitempty"`
	Dependencies      []string `json:"dependencies,omitempty"`
	IsDryRun         bool     `json:"is_dry_run"`
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
	Timestamp  time.Time              `json:"timestamp"`
	CPUModel    string                 `json:"cpu_model,omitempty"`
	CPUCores    int                    `json:"cpu_cores,omitempty"`
	CPUThreads  int                    `json:"cpu_threads,omitempty"`
	MemoryTotal uint64                 `json:"memory_total,omitempty"`
	DiskTotal   uint64                 `json:"disk_total,omitempty"`
	DiskUsed    uint64                 `json:"disk_used,omitempty"`
	IPAddress   string                 `json:"ip_address,omitempty"`
	Processes   int                    `json:"processes,omitempty"`
	Uptime      string                 `json:"uptime,omitempty"`
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
