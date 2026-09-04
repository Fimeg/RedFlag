package scanner

import (
	"testing"
)

// Real `dnf check-update` output captured from dnf5 v5.2.18 on Fedora 43.
// Format: pkgname.arch  version  repo  (three columns).

func TestParseDNFOutput_StandardRepos(t *testing.T) {
	output := []byte("SDL3.x86_64                            3.4.8-1.fc43                         updates\n")
	updates, err := parseDNFOutput(output)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	u := updates[0]
	if u.PackageName != "SDL3" {
		t.Errorf("PackageName = %q, want %q", u.PackageName, "SDL3")
	}
	if u.AvailableVersion != "3.4.8-1.fc43" {
		t.Errorf("AvailableVersion = %q, want %q", u.AvailableVersion, "3.4.8-1.fc43")
	}
	if u.RepositorySource != "updates" {
		t.Errorf("RepositorySource = %q, want %q", u.RepositorySource, "updates")
	}
	if u.PackageType != "dnf" {
		t.Errorf("PackageType = %q, want %q", u.PackageType, "dnf")
	}
}

func TestParseDNFOutput_CoprRepo(t *testing.T) {
	output := []byte("7zip.x86_64                          26.01-1.fc43                         copr:copr.fedorainfracloud.org:errornointernet:packages\n")
	updates, err := parseDNFOutput(output)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	u := updates[0]
	if u.PackageName != "7zip" {
		t.Errorf("PackageName = %q, want %q", u.PackageName, "7zip")
	}
	if u.AvailableVersion != "26.01-1.fc43" {
		t.Errorf("AvailableVersion = %q, want %q", u.AvailableVersion, "26.01-1.fc43")
	}
	if u.RepositorySource != "copr:copr.fedorainfracloud.org:errornointernet:packages" {
		t.Errorf("RepositorySource = %q, want %q", u.RepositorySource, "copr:copr.fedorainfracloud.org:errornointernet:packages")
	}
}

func TestParseDNFOutput_DockerRepo(t *testing.T) {
	output := []byte("docker-ce.x86_64                     3:29.5.2-1.fc43                      docker-ce-stable\n")
	updates, err := parseDNFOutput(output)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	u := updates[0]
	if u.AvailableVersion != "3:29.5.2-1.fc43" {
		t.Errorf("AvailableVersion = %q, want %q", u.AvailableVersion, "3:29.5.2-1.fc43")
	}
	if u.RepositorySource != "docker-ce-stable" {
		t.Errorf("RepositorySource = %q, want %q", u.RepositorySource, "docker-ce-stable")
	}
}

func TestParseDNFOutput_CoprRepoWithColons(t *testing.T) {
	output := []byte("coolercontrol.x86_64                4.3.1-1.fc43                         copr:copr.fedorainfracloud.org:codifryed:CoolerControl\n")
	updates, err := parseDNFOutput(output)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	u := updates[0]
	if u.AvailableVersion != "4.3.1-1.fc43" {
		t.Errorf("AvailableVersion = %q, want %q", u.AvailableVersion, "4.3.1-1.fc43")
	}
	if u.RepositorySource != "copr:copr.fedorainfracloud.org:codifryed:CoolerControl" {
		t.Errorf("RepositorySource = %q, want %q", u.RepositorySource, "copr:copr.fedorainfracloud.org:codifryed:CoolerControl")
	}
}

func TestParseDNFOutput_SkipsHeader(t *testing.T) {
	output := []byte("Last metadata expiration check: 0:05:23 ago on Wed 2026-05-28.\n\nSDL3.x86_64  3.4.8-1.fc43  updates\n")
	updates, err := parseDNFOutput(output)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update (skip header+blank), got %d", len(updates))
	}
}

func TestParseDNFOutput_SkipsObsoleting(t *testing.T) {
	output := []byte("Obsoleting Packages:\nSDL3.x86_64  3.4.8-1.fc43  updates\n")
	updates, err := parseDNFOutput(output)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update (skip Obsoleting), got %d", len(updates))
	}
}

func TestParseDNFOutput_Empty(t *testing.T) {
	updates, err := parseDNFOutput([]byte{})
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("expected 0 updates from empty input, got %d", len(updates))
	}
}

func TestParseDNFOutput_ArchitectureInMetadata(t *testing.T) {
	output := []byte("curl.x86_64  8.15.0-7.fc43  updates\n")
	updates, err := parseDNFOutput(output)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	u := updates[0]
	arch, ok := u.Metadata["architecture"].(string)
	if !ok {
		t.Fatal("metadata.architecture missing or not a string")
	}
	if arch != "x86_64" {
		t.Errorf("architecture = %q, want %q", arch, "x86_64")
	}
}

func TestFormatRPMEVR(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "no epoch",
			raw:  "0|3.4.8|1.fc43",
			want: "3.4.8-1.fc43",
		},
		{
			name: "epoch",
			raw:  "3|29.5.2|1.fc43",
			want: "3:29.5.2-1.fc43",
		},
		{
			name: "none epoch",
			raw:  "(none)|8.15.0|7.fc43",
			want: "8.15.0-7.fc43",
		},
		{
			name: "legacy raw fallback",
			raw:  "3.4.8",
			want: "3.4.8",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatRPMEVR(tt.raw); got != tt.want {
				t.Fatalf("formatRPMEVR(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestNewestInstalledEVR(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "single install",
			raw:  "0|3.4.8|1.fc43\n",
			want: "3.4.8-1.fc43",
		},
		{
			name: "kernel keeps three installs",
			raw:  "0|6.19.14|200.fc43\n0|7.0.10|101.fc43\n0|7.0.11|100.fc43\n",
			want: "7.0.11-100.fc43",
		},
		{
			name: "install order not version order",
			raw:  "0|7.0.11|100.fc43\n0|6.19.14|200.fc43\n",
			want: "7.0.11-100.fc43",
		},
		{
			name: "multilib identical EVRs",
			raw:  "1|2.4.18|1.fc43\n1|2.4.18|1.fc43\n",
			want: "1:2.4.18-1.fc43",
		},
		{
			name: "higher epoch wins over higher version",
			raw:  "0|9.0.0|1.fc43\n2|1.0.0|1.fc43\n",
			want: "2:1.0.0-1.fc43",
		},
		{
			name: "empty output",
			raw:  "",
			want: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newestInstalledEVR(tt.raw); got != tt.want {
				t.Fatalf("newestInstalledEVR(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestRpmvercmp(t *testing.T) {
	tests := []struct {
		a, b string
		want int
	}{
		{"1.0", "1.0", 0},
		{"1.0", "2.0", -1},
		{"2.0.1", "2.0", 1},
		{"7.0.11", "6.19.14", 1},
		{"1.05", "1.5", 0},     // leading zeros stripped
		{"10", "9", 1},          // numeric, not lexical
		{"1.0a", "1.0", 1},      // extra segment wins
		{"1.0~rc1", "1.0", -1},  // tilde is pre-release
		{"1.0~rc1", "1.0~rc2", -1},
		{"1.0^git1", "1.0", 1},  // caret is post-release
		{"1.0^git1", "1.0.1", -1},
		{"2.92rel2", "2.92rel1", 1},
		{"1a", "12", -1}, // digit segment beats alpha
	}
	for _, tt := range tests {
		t.Run(tt.a+"_vs_"+tt.b, func(t *testing.T) {
			if got := rpmvercmp(tt.a, tt.b); got != tt.want {
				t.Fatalf("rpmvercmp(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
			if got := rpmvercmp(tt.b, tt.a); got != -tt.want {
				t.Fatalf("rpmvercmp(%q, %q) = %d, want %d (symmetry)", tt.b, tt.a, got, -tt.want)
			}
		})
	}
}

func TestParseDNFOutput_MultipleLines(t *testing.T) {
	output := []byte("curl.x86_64      8.15.0-7.fc43       updates\ndnsmasq.x86_64   2.92rel2-2.fc43     updates\nbind-libs.x86_64 32:9.18.49-1.fc43    updates\n")
	updates, err := parseDNFOutput(output)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(updates) != 3 {
		t.Fatalf("expected 3 updates, got %d", len(updates))
	}
	names := []string{"curl", "dnsmasq", "bind-libs"}
	for i, name := range names {
		if updates[i].PackageName != name {
			t.Errorf("updates[%d].PackageName = %q, want %q", i, updates[i].PackageName, name)
		}
	}
}

// CurrentVersion is populated via `rpm -q` which won't work in test
// environments. Verify it returns "unknown" rather than crashing.
func TestParseDNFOutput_CurrentVersionUnknown(t *testing.T) {
	output := []byte("nonexistent-package-12345.x86_64  1.0-1.fc43  updates\n")
	updates, err := parseDNFOutput(output)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if len(updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(updates))
	}
	// getInstalledVersion fails for fake packages → returns "unknown"
	if updates[0].CurrentVersion != "unknown" {
		t.Errorf("CurrentVersion = %q, want %q", updates[0].CurrentVersion, "unknown")
	}
}
