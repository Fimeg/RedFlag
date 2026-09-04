package version

import (
	"strconv"
	"strings"
)

// Version represents a semantic version string
type Version string

// Platform represents combined platform-architecture format (e.g., "linux-amd64")
type Platform string

// ParsePlatform converts "linux-amd64" → platform="linux", arch="amd64"
func ParsePlatform(p Platform) (platform, architecture string) {
	parts := strings.SplitN(string(p), "-", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return string(p), ""
}

// String returns the full platform string
func (p Platform) String() string {
	return string(p)
}

// Compare returns -1, 0, or 1 for v < other, v == other, v > other
func (v Version) Compare(other Version) int {
	v1Parts := strings.Split(string(v), ".")
	v2Parts := strings.Split(string(other), ".")

	maxLen := len(v1Parts)
	if len(v2Parts) > maxLen {
		maxLen = len(v2Parts)
	}

	for i := 0; i < maxLen; i++ {
		v1Num := 0
		v2Num := 0

		if i < len(v1Parts) {
			v1Num, _ = strconv.Atoi(v1Parts[i])
		}
		if i < len(v2Parts) {
			v2Num, _ = strconv.Atoi(v2Parts[i])
		}

		if v1Num < v2Num {
			return -1
		}
		if v1Num > v2Num {
			return 1
		}
	}

	return 0
}

// IsUpgrade returns true if other is newer than v
func (v Version) IsUpgrade(other Version) bool {
	return v.Compare(other) < 0
}

// IsValid returns true if version string is non-empty
func (v Version) IsValid() bool {
	return string(v) != ""
}
