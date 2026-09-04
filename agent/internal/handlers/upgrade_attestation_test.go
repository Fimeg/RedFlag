package handlers

import "testing"

func TestVersionAtLeast(t *testing.T) {
	cases := []struct {
		running, target string
		want            bool
	}{
		{"0.2.7.0", "0.2.7.0", true},
		{"v0.2.7.0", "0.2.7.0", true},
		{"0.2.7.0", "v0.2.7.0", true},
		{"0.2.7.1", "0.2.7.0", true},  // past the target still proves the swap
		{"0.2.8.0", "0.2.7.9", true},
		{"0.2.7.0", "0.2.7.1", false}, // rolled back / swap failed
		{"0.2.6.9", "0.2.7.0", false},
		{"0.2.10.0", "0.2.9.0", true}, // numeric, not lexicographic
		{"0.2.7", "0.2.7.0", false},   // shorter = missing segment treated as lower
		{"0.2.7.0", "0.2.7", true},
	}
	for _, c := range cases {
		if got := versionAtLeast(c.running, c.target); got != c.want {
			t.Errorf("versionAtLeast(%q, %q) = %v, want %v", c.running, c.target, got, c.want)
		}
	}
}
