package version

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// Version coordination for Server Authority model
// The server is the single source of truth for all version information

// Version information (SERVER AUTHORITY).
// Values are maintained by scripts/bump-version.sh and must match the release
// tag — the release gate enforces this. ldflags may override at build time;
// the release pipeline injects the tag so binaries and source agree.
var (
	AgentVersion    = "0.2.9.3"
	ConfigVersion   = "0.2.9.3"
	MinAgentVersion = "0.1.22"
)

// CurrentVersions holds the authoritative version information
type CurrentVersions struct {
	AgentVersion    string    `json:"agent_version"`
	ConfigVersion   string    `json:"config_version"`
	MinAgentVersion string    `json:"min_agent_version"`
	BuildTime       time.Time `json:"build_time"`
}

// GetCurrentVersions returns the current version information
func GetCurrentVersions() CurrentVersions {
	return CurrentVersions{
		AgentVersion:    AgentVersion,
		ConfigVersion:   ConfigVersion,
		MinAgentVersion: MinAgentVersion,
		BuildTime:       time.Now(),
	}
}

// CompareVersions compares two version strings using octet-based comparison.
// Returns -1 if a < b, 0 if a == b, 1 if a > b.
// Handles "dev" as always older than any release version.
// Version format: "0.1.26.0" (up to 4 octets, padded with zeros).
func CompareVersions(a, b string) int {
	a = strings.TrimPrefix(a, "v")
	b = strings.TrimPrefix(b, "v")

	if a == b {
		return 0
	}
	if a == "dev" || a == "" {
		return -1
	}
	if b == "dev" || b == "" {
		return 1
	}

	aParts := strings.Split(a, ".")
	bParts := strings.Split(b, ".")

	maxLen := len(aParts)
	if len(bParts) > maxLen {
		maxLen = len(bParts)
	}

	for i := 0; i < maxLen; i++ {
		aVal := 0
		bVal := 0
		if i < len(aParts) {
			if n, err := strconv.Atoi(aParts[i]); err == nil {
				aVal = n
			}
		}
		if i < len(bParts) {
			if n, err := strconv.Atoi(bParts[i]); err == nil {
				bVal = n
			}
		}
		if aVal < bVal {
			return -1
		}
		if aVal > bVal {
			return 1
		}
	}
	return 0
}

// ExtractConfigVersionFromAgent extracts config version from agent version.
// Agent version format: "0.1.23.6" where the last octet is the config version.
func ExtractConfigVersionFromAgent(agentVersion string) string {
	cleanVersion := strings.TrimPrefix(agentVersion, "v")
	parts := strings.Split(cleanVersion, ".")
	if len(parts) >= 1 {
		return parts[len(parts)-1]
	}
	return "3"
}

// ValidateAgentVersion checks if an agent version is compatible
func ValidateAgentVersion(agentVersion string) error {
	current := GetCurrentVersions()
	if CompareVersions(agentVersion, current.MinAgentVersion) < 0 {
		return fmt.Errorf("agent version %s is below minimum %s", agentVersion, current.MinAgentVersion)
	}
	return nil
}

// GetBuildFlags returns the ldflags to inject versions into agent builds
func GetBuildFlags() []string {
	versions := GetCurrentVersions()
	return []string{
		fmt.Sprintf("-X github.com/Fimeg/RedFlag/agent/internal/version.Version=%s", versions.AgentVersion),
		fmt.Sprintf("-X github.com/Fimeg/RedFlag/agent/internal/version.ConfigVersion=%s", versions.ConfigVersion),
		fmt.Sprintf("-X github.com/Fimeg/RedFlag/agent/internal/version.BuildTime=%s", versions.BuildTime.Format(time.RFC3339)),
	}
}
