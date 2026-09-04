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
	// Parse version strings (format "0.1.4" or "0.2.3.10" — any octet count)
	v1Parts := parseVersion(version1)
	v2Parts := parseVersion(version2)

	maxLen := len(v1Parts)
	if len(v2Parts) > maxLen {
		maxLen = len(v2Parts)
	}

	// Compare octet by octet; missing trailing octets count as 0
	for i := 0; i < maxLen; i++ {
		v1 := 0
		v2 := 0
		if i < len(v1Parts) {
			v1 = v1Parts[i]
		}
		if i < len(v2Parts) {
			v2 = v2Parts[i]
		}
		if v1 < v2 {
			return -1
		}
		if v1 > v2 {
			return 1
		}
	}

	return 0
}

// IsNewerVersion returns true if version1 is newer than version2
func IsNewerVersion(version1, version2 string) bool {
	return CompareVersions(version1, version2) == 1
}

// IsNewerOrEqualVersion returns true if version1 is newer than or equal to version2
func IsNewerOrEqualVersion(version1, version2 string) bool {
	cmp := CompareVersions(version1, version2)
	return cmp == 1 || cmp == 0
}

// parseVersion parses a version string like "0.1.4" or "0.2.3.10" into its
// octets, e.g. [0, 2, 3, 10]. Unparsable octets default to 0.
func parseVersion(version string) []int {
	cleanVersion := strings.TrimPrefix(version, "v")
	parts := strings.Split(cleanVersion, ".")

	result := make([]int, len(parts))
	for i, p := range parts {
		if num, err := strconv.Atoi(p); err == nil {
			result[i] = num
		}
	}

	return result
}