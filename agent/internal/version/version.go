package version

import (
	"fmt"
	"runtime"
	"strings"
	"time"
)

// Build-time injected version information (SERVER AUTHORITY)
// Injected by server during build via ldflags
var (
	Version       = "dev" // Agent version (format: 0.1.26.0)
	ConfigVersion = "dev" // Config schema version (format: 0, 1, 2, etc.)
	BuildTime     = "unknown"
	GitCommit     = "unknown"
	GoVersion     = runtime.Version()
)

// ExtractConfigVersionFromAgent extracts the config version from the agent version
// Agent version format: v0.1.23.6 where the fourth octet (.6) maps to config version
// This provides the traditional mapping when only agent version is available
func ExtractConfigVersionFromAgent(agentVer string) string {
	// Strip 'v' prefix if present
	cleanVersion := strings.TrimPrefix(agentVer, "v")

	// Split version parts
	parts := strings.Split(cleanVersion, ".")
	if len(parts) == 4 {
		// Return the fourth octet as the config version
		// v0.1.23.6 → "6"
		return parts[3]
	}

	// If we have a build-time injected ConfigVersion, use it
	if ConfigVersion != "dev" {
		return ConfigVersion
	}

	// Default fallback
	return "6"
}

// Info holds complete version information
type Info struct {
	AgentVersion    string `json:"agent_version"`
	ConfigVersion   string `json:"config_version"`
	BuildTime       string `json:"build_time"`
	GitCommit       string `json:"git_commit"`
	GoVersion       string `json:"go_version"`
	BuildTimestamp  int64  `json:"build_timestamp"`
}

// GetInfo returns complete version information
func GetInfo() Info {
	// Parse build time if available
	timestamp := time.Now().Unix()
	if BuildTime != "unknown" {
		if t, err := time.Parse(time.RFC3339, BuildTime); err == nil {
			timestamp = t.Unix()
		}
	}

	return Info{
		AgentVersion:   Version,
		ConfigVersion:  ConfigVersion,
		BuildTime:      BuildTime,
		GitCommit:      GitCommit,
		GoVersion:      GoVersion,
		BuildTimestamp: timestamp,
	}
}

// String returns a human-readable version string
func String() string {
	return fmt.Sprintf("RedFlag Agent v%s (config v%s)", Version, ConfigVersion)
}

// FullString returns detailed version information
func FullString() string {
	info := GetInfo()
	return fmt.Sprintf("RedFlag Agent v%s (config v%s)\n"+
		"Built: %s\n"+
		"Commit: %s\n"+
		"Go: %s",
		info.AgentVersion,
		info.ConfigVersion,
		info.BuildTime,
		info.GitCommit,
		info.GoVersion)
}

// CheckCompatible checks if the given config version is compatible with this agent
func CheckCompatible(configVer string) error {
	if configVer == "" {
		return fmt.Errorf("config version is empty")
	}

	// For now, require exact match
	// In the future, we may support backward/forward compatibility matrices
	if configVer != ConfigVersion {
		return fmt.Errorf("config version mismatch: agent expects v%s, config has v%s",
			ConfigVersion, configVer)
	}

	return nil
}

// Valid checks if version information is properly set
func Valid() bool {
	return Version != "dev" && ConfigVersion != "dev"
}