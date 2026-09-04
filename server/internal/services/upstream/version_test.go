package upstream

import "testing"

func TestParseVersion(t *testing.T) {
	cases := []struct {
		in       string
		major    int
		minor    int
		patch    int
		hasMinor bool
		hasPatch bool
	}{
		{"15.4", 15, 4, 0, true, false},
		{"v1.27.3", 1, 27, 3, true, true},
		{"1.0.0-rc1+meta", 1, 0, 0, true, true},
		{"20231130-1.fc40", 20231130, 0, 0, false, false},
		{"3", 3, 0, 0, false, false},
		{"", 0, 0, 0, false, false},
		{"nginx", 0, 0, 0, false, false},
	}
	for _, c := range cases {
		got := ParseVersion(c.in)
		if got.Major != c.major || got.Minor != c.minor || got.Patch != c.patch ||
			got.HasMinor != c.hasMinor || got.HasPatch != c.hasPatch {
			t.Errorf("ParseVersion(%q) = %+v, want major=%d minor=%d patch=%d", c.in, got, c.major, c.minor, c.patch)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		// Numeric prefix
		{"15.4", "15.5", -1},
		{"15.5", "15.4", 1},
		{"15.4", "15.4", 0},
		{"v1.0.0", "1.0.0", 0},
		{"2.0.0", "1.99.99", 1},
		// Release vs prerelease
		{"1.0.0", "1.0.0-rc1", 1},
		{"1.0.0-rc1", "1.0.0", -1},
		// Single-digit rc ordering (worked before; lock it in)
		{"1.0.0-rc1", "1.0.0-rc2", -1},
		// Multi-digit rc ordering — the bug this commit fixes
		{"1.0.0-rc2", "1.0.0-rc10", -1},
		{"1.0.0-rc10", "1.0.0-rc2", 1},
		// Dot-separated SemVer-canonical prerelease
		{"1.0.0-rc.2", "1.0.0-rc.10", -1},
		{"1.0.0-rc.10", "1.0.0-rc.2", 1},
		// Build metadata is ignored
		{"1.0.0+build1", "1.0.0+build9999", 0},
		{"1.0.0-rc1+meta", "1.0.0-rc1", 0},
		// Prerelease label ordering (alpha < beta < rc by lex)
		{"1.0.0-alpha", "1.0.0-beta", -1},
		{"1.0.0-beta", "1.0.0-rc", -1},
		// More-fields wins per SemVer §11.4.4
		{"1.0.0-rc.1", "1.0.0-rc.1.2", -1},
		// Postgres-style "no separator" suffix
		{"15rc1", "15", -1},
		{"15rc1", "15.0", -1},
		{"15rc2", "15rc10", -1},
		// Distro-tag suffix (date-based major)
		{"20231130-1.fc40", "20231201-1.fc40", -1},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestClassifyDrift(t *testing.T) {
	cases := []struct {
		from, to string
		want     string
	}{
		{"14.5", "15.0", "major"},
		{"15.3", "15.4", "minor"},
		{"1.27.2", "1.27.3", "patch"},
		{"1.0.0-rc1", "1.0.0-rc2", "metadata"},
	}
	for _, c := range cases {
		if got := ClassifyDrift(c.from, c.to); got != c.want {
			t.Errorf("ClassifyDrift(%q, %q) = %q, want %q", c.from, c.to, got, c.want)
		}
	}
}
