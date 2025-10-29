package cache

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Fimeg/RedFlag/aggregator-agent/internal/client"
	"github.com/google/uuid"
)

// LocalCache stores scan results locally for offline viewing
type LocalCache struct {
	LastScanTime   time.Time              `json:"last_scan_time"`
	LastCheckIn    time.Time              `json:"last_check_in"`
	AgentID        uuid.UUID              `json:"agent_id"`
	ServerURL      string                 `json:"server_url"`
	UpdateCount    int                    `json:"update_count"`
	Updates        []client.UpdateReportItem `json:"updates"`
	AgentStatus    string                 `json:"agent_status"`
}

// CacheDir is the directory where local cache is stored
const CacheDir = "/var/lib/aggregator"

// CacheFile is the file where scan results are cached
const CacheFile = "last_scan.json"

// GetCachePath returns the full path to the cache file
func GetCachePath() string {
	return filepath.Join(CacheDir, CacheFile)
}

// Load reads the local cache from disk
func Load() (*LocalCache, error) {
	cachePath := GetCachePath()

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
	cachePath := GetCachePath()

	// Ensure cache directory exists
	if err := os.MkdirAll(CacheDir, 0755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
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
	c.LastScanTime = time.Now()
	c.Updates = updates
	c.UpdateCount = len(updates)
}

// UpdateCheckIn updates the last check-in time
func (c *LocalCache) UpdateCheckIn() {
	c.LastCheckIn = time.Now()
}

// SetAgentInfo sets agent identification information
func (c *LocalCache) SetAgentInfo(agentID uuid.UUID, serverURL string) {
	c.AgentID = agentID
	c.ServerURL = serverURL
}

// SetAgentStatus sets the current agent status
func (c *LocalCache) SetAgentStatus(status string) {
	c.AgentStatus = status
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
}