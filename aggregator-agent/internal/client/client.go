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
	AgentID uuid.UUID              `json:"agent_id"`
	Token   string                 `json:"token"`
	Config  map[string]interface{} `json:"config"`
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

// GetCommands retrieves pending commands from the server
func (c *Client) GetCommands(agentID uuid.UUID) ([]Command, error) {
	url := fmt.Sprintf("%s/api/v1/agents/%s/commands", c.baseURL, agentID)

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
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
