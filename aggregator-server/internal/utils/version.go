package utils

import (
	"strconv"
	"strings"
)

// CompareVersions compares two semantic version strings
// Returns:
//   -1 if version1 < version2
//    0 if version1 == version2
//    1 if version1 > version2
func CompareVersions(version1, version2 string) int {
	// Parse version strings (expected format: "0.1.4")
	v1Parts := parseVersion(version1)
	v2Parts := parseVersion(version2)

	// Compare major, minor, patch versions
	for i := 0; i < 3; i++ {
		if v1Parts[i] < v2Parts[i] {
			return -1
		}
		if v1Parts[i] > v2Parts[i] {
			return 1
		}
	}

	return 0
}

// IsNewerVersion returns true if version1 is newer than version2
func IsNewerVersion(version1, version2 string) bool {
	return CompareVersions(version1, version2) == 1
}

// parseVersion parses a version string like "0.1.4" into [0, 1, 4]
func parseVersion(version string) [3]int {
	// Default version if parsing fails
	result := [3]int{0, 0, 0}

	// Remove any 'v' prefix and split by dots
	cleanVersion := strings.TrimPrefix(version, "v")
	parts := strings.Split(cleanVersion, ".")

	// Parse each part, defaulting to 0 if parsing fails
	for i := 0; i < len(parts) && i < 3; i++ {
		if num, err := strconv.Atoi(parts[i]); err == nil {
			result[i] = num
		}
	}

	return result
}