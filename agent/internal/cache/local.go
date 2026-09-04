package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/constants"
	"github.com/gofrs/uuid/v5"
)

// LocalCache stores scan results locally for offline viewing
type LocalCache struct {
	LastScanTime time.Time                 `json:"last_scan_time"`
	LastCheckIn  time.Time                 `json:"last_check_in"`
	AgentID      uuid.UUID                 `json:"agent_id"`
	ServerURL    string                    `json:"server_url"`
	UpdateCount  int                       `json:"update_count"`
	Updates      []client.UpdateReportItem `json:"updates"`
	AgentStatus  string                    `json:"agent_status"`
	Summary      UpdateSummary             `json:"summary"`
	Scanners     map[string]ScannerState   `json:"scanners,omitempty"`
	Capabilities CapabilityTokenState      `json:"capabilities"`
	LastUpdated  time.Time                 `json:"last_updated"`
}

// UpdateSummary is the local rollup consumed by status surfaces.
type UpdateSummary struct {
	Total       int            `json:"total"`
	ByEcosystem map[string]int `json:"by_ecosystem,omitempty"`
	BySeverity  map[string]int `json:"by_severity,omitempty"`
}

// ScannerState records the latest local scan status for one scanner.
type ScannerState struct {
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	LastScanTime   time.Time `json:"last_scan_time,omitempty"`
	LastDurationMS int64     `json:"last_duration_ms,omitempty"`
	LastError      string    `json:"last_error,omitempty"`
	UpdateCount    int       `json:"update_count"`
}

// CapabilityTokenState is count-only local state for the supply-chain token path.
// It intentionally excludes token IDs, token payloads, signatures, and artifacts.
type CapabilityTokenState struct {
	LastFetchTime      time.Time `json:"last_fetch_time,omitempty"`
	LastProcessTime    time.Time `json:"last_process_time,omitempty"`
	PendingCount       int       `json:"pending_count"`
	LastFetchedCount   int       `json:"last_fetched_count"`
	LastProcessedCount int       `json:"last_processed_count"`
	LastFailedCount    int       `json:"last_failed_count"`
	LastError          string    `json:"last_error,omitempty"`
}

// cacheFile is the file where scan results are cached
const cacheFile = "last_scan.json"

// GetCachePath returns the full path to the cache file
func GetCachePath() string {
	return filepath.Join(constants.GetAgentCacheDir(), cacheFile)
}

// Load reads the local cache from disk
func Load() (*LocalCache, error) {
	return LoadFromPath(GetCachePath())
}

// LoadFromPath reads a local cache file from disk.
func LoadFromPath(cachePath string) (*LocalCache, error) {
	// Check if cache file exists
	if _, err := os.Stat(cachePath); os.IsNotExist(err) {
		// Return empty cache if file doesn't exist
		return &LocalCache{}, nil
	}

	// Read cache file
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read cache file: %w", err)
	}

	var cache LocalCache
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse cache file: %w", err)
	}

	return &cache, nil
}

// Save writes the local cache to disk
func (c *LocalCache) Save() error {
	return c.SaveToPath(GetCachePath())
}

// SaveToPath writes a local cache file to disk.
func (c *LocalCache) SaveToPath(cachePath string) error {
	// Ensure cache directory exists
	if err := os.MkdirAll(filepath.Dir(cachePath), 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	c.refreshSummary()
	if c.LastUpdated.IsZero() {
		c.LastUpdated = time.Now().UTC()
	}

	// Marshal cache to JSON with indentation
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache: %w", err)
	}

	// Write cache file with restricted permissions
	if err := os.WriteFile(cachePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write cache file: %w", err)
	}

	return nil
}

// UpdateScanResults updates the cache with new scan results
func (c *LocalCache) UpdateScanResults(updates []client.UpdateReportItem) {
	now := time.Now().UTC()
	c.LastScanTime = now
	c.LastUpdated = now
	c.Updates = updates
	c.UpdateCount = len(updates)
	c.refreshSummary()
}

// UpdateCheckIn updates the last check-in time
func (c *LocalCache) UpdateCheckIn() {
	now := time.Now().UTC()
	c.LastCheckIn = now
	c.LastUpdated = now
}

// SetAgentInfo sets agent identification information
func (c *LocalCache) SetAgentInfo(agentID uuid.UUID, serverURL string) {
	c.AgentID = agentID
	c.ServerURL = serverURL
	c.LastUpdated = time.Now().UTC()
}

// SetAgentStatus sets the current agent status
func (c *LocalCache) SetAgentStatus(status string) {
	c.AgentStatus = status
	c.LastUpdated = time.Now().UTC()
}

// RecordScannerResult updates the local read model with one scanner execution.
// Successful package-update scans replace only their ecosystem slice, so a
// scan_apt command does not erase the latest DNF/Winget/Docker observations.
func (c *LocalCache) RecordScannerResult(scannerName, status string, updates []client.UpdateReportItem, scanErr error, duration time.Duration, affectsUpdateList bool) {
	name := normalizeScannerName(scannerName)
	if status == "" {
		status = "unknown"
	}

	if c.Scanners == nil {
		c.Scanners = make(map[string]ScannerState)
	}

	now := time.Now().UTC()
	lastError := ""
	if scanErr != nil {
		lastError = scanErr.Error()
	}

	c.Scanners[name] = ScannerState{
		Name:           name,
		Status:         status,
		LastScanTime:   now,
		LastDurationMS: duration.Milliseconds(),
		LastError:      lastError,
		UpdateCount:    len(updates),
	}
	c.LastUpdated = now

	if affectsUpdateList && status == "success" {
		c.replaceScannerUpdates(name, updates)
		c.LastScanTime = now
		c.UpdateCount = len(c.Updates)
		c.refreshSummary()
	}
}

// RecordCapabilityTokenFetch records the count of tokens fetched on the latest poll.
func (c *LocalCache) RecordCapabilityTokenFetch(fetched int, fetchErr error) {
	now := time.Now().UTC()
	c.Capabilities.LastFetchTime = now
	c.Capabilities.LastFetchedCount = fetched
	c.Capabilities.PendingCount = fetched
	c.Capabilities.LastError = ""
	if fetchErr != nil {
		c.Capabilities.LastError = fetchErr.Error()
	}
	c.LastUpdated = now
}

// RecordCapabilityTokenProcess records count-only token processing results.
func (c *LocalCache) RecordCapabilityTokenProcess(processed, failed int) {
	now := time.Now().UTC()
	c.Capabilities.LastProcessTime = now
	c.Capabilities.LastProcessedCount = processed
	c.Capabilities.LastFailedCount = failed
	remaining := c.Capabilities.LastFetchedCount - processed - failed
	if remaining < 0 {
		remaining = 0
	}
	c.Capabilities.PendingCount = remaining
	c.LastUpdated = now
}

// IsExpired checks if the cache is older than the specified duration
func (c *LocalCache) IsExpired(maxAge time.Duration) bool {
	return time.Since(c.LastScanTime) > maxAge
}

// GetUpdatesByType returns updates filtered by package type
func (c *LocalCache) GetUpdatesByType(packageType string) []client.UpdateReportItem {
	var filtered []client.UpdateReportItem
	for _, update := range c.Updates {
		if update.PackageType == packageType {
			filtered = append(filtered, update)
		}
	}
	return filtered
}

// Clear clears the cache
func (c *LocalCache) Clear() {
	c.LastScanTime = time.Time{}
	c.LastCheckIn = time.Time{}
	c.UpdateCount = 0
	c.Updates = []client.UpdateReportItem{}
	c.AgentStatus = ""
	c.Summary = UpdateSummary{}
	c.Scanners = nil
	c.Capabilities = CapabilityTokenState{}
	c.LastUpdated = time.Now().UTC()
}

func (c *LocalCache) replaceScannerUpdates(scannerName string, updates []client.UpdateReportItem) {
	packageTypes := packageTypesForScanner(scannerName, updates)
	if len(packageTypes) == 0 {
		return
	}

	filtered := make([]client.UpdateReportItem, 0, len(c.Updates)+len(updates))
	for _, update := range c.Updates {
		if _, replace := packageTypes[normalizeScannerName(update.PackageType)]; replace {
			continue
		}
		filtered = append(filtered, update)
	}
	filtered = append(filtered, updates...)
	c.Updates = filtered
}

func (c *LocalCache) refreshSummary() {
	summary := UpdateSummary{
		Total:       len(c.Updates),
		ByEcosystem: make(map[string]int),
		BySeverity:  make(map[string]int),
	}
	for _, update := range c.Updates {
		ecosystem := normalizeScannerName(update.PackageType)
		if ecosystem == "" {
			ecosystem = "unknown"
		}
		severity := strings.ToLower(strings.TrimSpace(update.Severity))
		if severity == "" {
			severity = "unknown"
		}
		summary.ByEcosystem[ecosystem]++
		summary.BySeverity[severity]++
	}
	if len(summary.ByEcosystem) == 0 {
		summary.ByEcosystem = nil
	}
	if len(summary.BySeverity) == 0 {
		summary.BySeverity = nil
	}
	c.UpdateCount = summary.Total
	c.Summary = summary
}

func packageTypesForScanner(scannerName string, updates []client.UpdateReportItem) map[string]struct{} {
	packageTypes := make(map[string]struct{})
	switch normalizeScannerName(scannerName) {
	case "apt", "dnf", "pacman", "docker", "winget":
		packageTypes[normalizeScannerName(scannerName)] = struct{}{}
	case "windows":
		packageTypes["windows_update"] = struct{}{}
		packageTypes["windows_update_history"] = struct{}{}
	}

	for _, update := range updates {
		if packageType := normalizeScannerName(update.PackageType); packageType != "" {
			packageTypes[packageType] = struct{}{}
		}
	}
	return packageTypes
}

func normalizeScannerName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
