package upstream

import (
	"regexp"
	"strconv"
	"strings"
)

// Version comparison that doesn't pretend to be full SemVer 2.0.0. Real
// upstream versions are messy — "15.4", "v1.27.3", "1.0.0-rc1+meta",
// "20231130-1.fc40". We extract the leading numeric segments and compare
// those; the trailing suffix is split into alternating digit / non-digit
// chunks and compared so "-rc10" sorts after "-rc2" the way humans expect.
//
// This is deliberately not pulling golang.org/x/mod/semver because that
// library rejects "15.4" (no patch) outright. Repology and endoflife are
// happy to return such versions, so we have to handle them.

var numericPrefix = regexp.MustCompile(`^v?(\d+)(?:\.(\d+))?(?:\.(\d+))?`)

// VersionParts captures up to three leading numeric components.
type VersionParts struct {
	Major    int
	Minor    int
	Patch    int
	HasMinor bool
	HasPatch bool
	Suffix   string // anything after the numeric prefix, including separators
}

func ParseVersion(s string) VersionParts {
	s = strings.TrimSpace(s)
	m := numericPrefix.FindStringSubmatch(s)
	if m == nil {
		return VersionParts{Suffix: s}
	}
	out := VersionParts{Suffix: s[len(m[0]):]}
	if n, err := strconv.Atoi(m[1]); err == nil {
		out.Major = n
	}
	if m[2] != "" {
		out.HasMinor = true
		if n, err := strconv.Atoi(m[2]); err == nil {
			out.Minor = n
		}
	}
	if m[3] != "" {
		out.HasPatch = true
		if n, err := strconv.Atoi(m[3]); err == nil {
			out.Patch = n
		}
	}
	return out
}

// CompareVersions returns:
//
//	-1 if a < b
//	 0 if a == b (or both unparseable and equal)
//	 1 if a > b
//
// The leading numeric segments win. When those tie, the suffix is split into
// alternating runs of digits and non-digits and compared chunk-by-chunk so
// "-rc10" correctly sorts after "-rc2".
func CompareVersions(a, b string) int {
	pa, pb := ParseVersion(a), ParseVersion(b)
	if pa.Major != pb.Major {
		if pa.Major < pb.Major {
			return -1
		}
		return 1
	}
	if pa.Minor != pb.Minor {
		if pa.Minor < pb.Minor {
			return -1
		}
		return 1
	}
	if pa.Patch != pb.Patch {
		if pa.Patch < pb.Patch {
			return -1
		}
		return 1
	}
	return compareSuffix(pa.Suffix, pb.Suffix)
}

// compareSuffix compares two version suffixes (the tail after the leading
// M.m.p numeric prefix). Returns -1, 0, or 1.
//
// Pragmatic SemVer subset:
//   - Build metadata (everything after '+') is stripped before compare
//     (SemVer §10).
//   - An empty suffix beats any non-empty (release > prerelease).
//   - Otherwise split into maximal runs of digits and non-digits, compare
//     chunk-by-chunk: digit runs numerically, non-digit runs lexically.
//   - When a digit run meets a non-digit run, the digit run is lesser
//     (SemVer §11.4.3, also matches the rpmvercmp convention).
//   - Shorter prefix wins if all common chunks are equal (SemVer §11.4.4).
func compareSuffix(a, b string) int {
	if i := strings.IndexByte(a, '+'); i >= 0 {
		a = a[:i]
	}
	if i := strings.IndexByte(b, '+'); i >= 0 {
		b = b[:i]
	}
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}
	for len(a) > 0 && len(b) > 0 {
		chunkA, restA := nextChunk(a)
		chunkB, restB := nextChunk(b)
		if c := compareChunks(chunkA, chunkB); c != 0 {
			return c
		}
		a, b = restA, restB
	}
	if len(a) == len(b) {
		return 0
	}
	if len(a) < len(b) {
		return -1
	}
	return 1
}

func nextChunk(s string) (chunk, rest string) {
	if s == "" {
		return "", ""
	}
	isDigit := s[0] >= '0' && s[0] <= '9'
	i := 1
	for i < len(s) {
		d := s[i] >= '0' && s[i] <= '9'
		if d != isDigit {
			break
		}
		i++
	}
	return s[:i], s[i:]
}

func compareChunks(a, b string) int {
	if a == b {
		return 0
	}
	aIsDigit := a != "" && a[0] >= '0' && a[0] <= '9'
	bIsDigit := b != "" && b[0] >= '0' && b[0] <= '9'
	if aIsDigit && bIsDigit {
		na, _ := strconv.Atoi(a)
		nb, _ := strconv.Atoi(b)
		if na < nb {
			return -1
		}
		if na > nb {
			return 1
		}
		return 0
	}
	if aIsDigit {
		return -1
	}
	if bIsDigit {
		return 1
	}
	if a < b {
		return -1
	}
	return 1
}

// ClassifyDrift returns the severity label for two known versions. When the
// majors differ it's "major"; minors differ → "minor"; patches differ →
// "patch"; otherwise "metadata" (build/rc/suffix shift). Caller may override
// with "eol" when an EOL date has passed.
func ClassifyDrift(from, to string) string {
	pf, pt := ParseVersion(from), ParseVersion(to)
	if pf.Major != pt.Major {
		return "major"
	}
	if pf.Minor != pt.Minor {
		return "minor"
	}
	if pf.Patch != pt.Patch {
		return "patch"
	}
	return "metadata"
}
