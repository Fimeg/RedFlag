package scanner

import (
	"testing"
)

func TestParsePacmanOutput(t *testing.T) {
	// checkupdates format: pkgname oldver -> newver
	sample := `fakeroot 1:1.37.2-1 -> 1:1.37.2-2
firefox 151.0.4-1 -> 152.0-1
linux 6.14.4-1 -> 6.14.5-1
openssl 3.5.0-1 -> 3.5.1-1
`

	updates, err := parsePacmanOutput([]byte(sample))
	if err != nil {
		t.Fatalf("parsePacmanOutput failed: %v", err)
	}

	if len(updates) != 4 {
		t.Fatalf("expected 4 updates, got %d", len(updates))
	}

	// fakeroot with epoch
	if updates[0].PackageName != "fakeroot" {
		t.Errorf("expected fakeroot, got %s", updates[0].PackageName)
	}
	if updates[0].CurrentVersion != "1:1.37.2-1" {
		t.Errorf("expected current 1:1.37.2-1, got %s", updates[0].CurrentVersion)
	}
	if updates[0].AvailableVersion != "1:1.37.2-2" {
		t.Errorf("expected available 1:1.37.2-2, got %s", updates[0].AvailableVersion)
	}
	if updates[0].PackageType != "pacman" {
		t.Errorf("expected package_type pacman, got %s", updates[0].PackageType)
	}

	// firefox — no epoch
	if updates[1].PackageName != "firefox" {
		t.Errorf("expected firefox, got %s", updates[1].PackageName)
	}
	if updates[1].CurrentVersion != "151.0.4-1" {
		t.Errorf("expected current 151.0.4-1, got %s", updates[1].CurrentVersion)
	}

	// linux — kernel severity
	if updates[2].PackageName != "linux" {
		t.Errorf("expected linux, got %s", updates[2].PackageName)
	}
	if updates[2].Severity != "important" {
		t.Errorf("expected severity important for kernel, got %s", updates[2].Severity)
	}

	// openssl — crypto severity
	if updates[3].PackageName != "openssl" {
		t.Errorf("expected openssl, got %s", updates[3].PackageName)
	}
	if updates[3].Severity != "critical" {
		t.Errorf("expected severity critical for openssl, got %s", updates[3].Severity)
	}
}

func TestParsePacmanEmpty(t *testing.T) {
	updates, err := parsePacmanOutput([]byte(""))
	if err != nil {
		t.Fatalf("parsePacmanOutput on empty input failed: %v", err)
	}
	if len(updates) != 0 {
		t.Errorf("expected 0 updates from empty input, got %d", len(updates))
	}
}

func TestParsePacmanGibberish(t *testing.T) {
	// Lines that don't match the pattern should be silently skipped
	sample := `some random text
-> dangling arrow
no version info here
pkg without arrow
`

	updates, err := parsePacmanOutput([]byte(sample))
	if err != nil {
		t.Fatalf("parsePacmanOutput on gibberish failed: %v", err)
	}
	if len(updates) != 0 {
		t.Errorf("expected 0 updates from gibberish, got %d", len(updates))
	}
}
