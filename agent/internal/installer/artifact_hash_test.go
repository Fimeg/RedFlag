package installer

import "testing"

func TestValidatePackageName(t *testing.T) {
	valid := []string{
		"bash",
		"kernel-core",
		"glibc-common",
		"python3.12",
		"lib64gcc-s1",
		"foo+bar",
		"3:firefox", // epoch-prefixed reference
		"gcc~snapshot",
	}
	for _, name := range valid {
		if err := validatePackageName(name); err != nil {
			t.Errorf("validatePackageName(%q) = %v, want nil", name, err)
		}
	}

	invalid := []string{
		"",
		"-leadingdash",
		"semi;colon",
		"pipe|inject",
		"space here",
		"back`tick",
		"new\nline",
		"$(cmd)",
	}
	for _, name := range invalid {
		if err := validatePackageName(name); err == nil {
			t.Errorf("validatePackageName(%q) = nil, want error", name)
		}
	}
}

func TestFormatRPMVersion(t *testing.T) {
	tests := []struct {
		name    string
		epoch   string
		version string
		release string
		want    string
	}{
		{
			name:    "no epoch",
			epoch:   "0",
			version: "3.4.8",
			release: "1.fc43",
			want:    "3.4.8-1.fc43",
		},
		{
			name:    "epoch",
			epoch:   "3",
			version: "29.5.2",
			release: "1.fc43",
			want:    "3:29.5.2-1.fc43",
		},
		{
			name:    "none epoch",
			epoch:   "(none)",
			version: "8.15.0",
			release: "7.fc43",
			want:    "8.15.0-7.fc43",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := formatRPMVersion(tt.epoch, tt.version, tt.release); got != tt.want {
				t.Fatalf("formatRPMVersion(%q, %q, %q) = %q, want %q",
					tt.epoch, tt.version, tt.release, got, tt.want)
			}
		})
	}
}

func TestParseAptCandidate(t *testing.T) {
	cases := []struct {
		name   string
		output string
		want   string
	}{
		{
			name: "normal candidate",
			output: `nginx:
  Installed: 1.18.0-6ubuntu14.4
  Candidate: 1.18.0-6ubuntu14.5
  Version table:
     1.18.0-6ubuntu14.5 500
        500 http://archive.ubuntu.com/ubuntu jammy-updates/main amd64 Packages`,
			want: "1.18.0-6ubuntu14.5",
		},
		{
			name: "no candidate",
			output: `bogus:
  Installed: (none)
  Candidate: (none)
  Version table:`,
			want: "",
		},
		{
			name:   "missing line",
			output: "some unrelated output\n",
			want:   "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseAptCandidate(tc.output); got != tc.want {
				t.Errorf("parseAptCandidate() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestAptFieldFromStanza(t *testing.T) {
	stanza := `Package: nginx
Version: 1.18.0-6ubuntu14.5
Architecture: amd64
SHA256: 9f7c1e2b3a4d5e6f7081920a1b2c3d4e5f60718293a4b5c6d7e8f90112233445
Filename: pool/main/n/nginx/nginx_1.18.0-6ubuntu14.5_amd64.deb`

	if got := aptFieldFromStanza(stanza, "SHA256:"); got != "9f7c1e2b3a4d5e6f7081920a1b2c3d4e5f60718293a4b5c6d7e8f90112233445" {
		t.Errorf("aptFieldFromStanza(SHA256) = %q", got)
	}
	if got := aptFieldFromStanza(stanza, "Version:"); got != "1.18.0-6ubuntu14.5" {
		t.Errorf("aptFieldFromStanza(Version) = %q", got)
	}
	if got := aptFieldFromStanza(stanza, "MD5sum:"); got != "" {
		t.Errorf("aptFieldFromStanza(missing) = %q, want empty", got)
	}
}
