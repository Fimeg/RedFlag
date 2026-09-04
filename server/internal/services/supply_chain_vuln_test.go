package services

import "testing"

func TestToVulnerabilityInfoEnrichesCVSSAffectedRangesAndKEV(t *testing.T) {
	info := toVulnerabilityInfo(OSVVuln{
		ID:      "CVE-2026-0001",
		Summary: "demo vulnerability",
		Severity: []OSVSeverity{
			{Type: "CVSS_V3", Score: "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H"},
		},
		Affected: []OSVAffected{
			{
				Ranges: []OSVRange{
					{
						Type: "SEMVER",
						Events: []OSVEvent{
							{Introduced: "1.0.0"},
							{Fixed: "1.2.3"},
						},
					},
				},
			},
		},
		DatabaseSpecific: map[string]interface{}{"cisa_kev": true},
	})

	if info.CVSSVector != "CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H" {
		t.Fatalf("CVSSVector = %q", info.CVSSVector)
	}
	if info.CVSSScore != 9.8 {
		t.Fatalf("CVSSScore = %v, want 9.8", info.CVSSScore)
	}
	if info.Severity != "CRITICAL" {
		t.Fatalf("Severity = %q, want CRITICAL", info.Severity)
	}
	if info.FixedVersion != "1.2.3" {
		t.Fatalf("FixedVersion = %q, want 1.2.3", info.FixedVersion)
	}
	if len(info.AffectedRanges) != 1 || info.AffectedRanges[0] != "SEMVER: >= 1.0.0, < 1.2.3" {
		t.Fatalf("AffectedRanges = %#v", info.AffectedRanges)
	}
	if !info.KnownExploited {
		t.Fatal("KnownExploited = false, want true")
	}
}

func TestToVulnerabilityInfoKeepsDatabaseSpecificSeverity(t *testing.T) {
	info := toVulnerabilityInfo(OSVVuln{
		ID: "GHSA-demo",
		Severity: []OSVSeverity{
			{Type: "CVSS_V3", Score: "CVSS:3.1/AV:L/AC:H/PR:H/UI:R/S:U/C:L/I:N/A:N"},
		},
		DatabaseSpecific: map[string]interface{}{"severity": "high"},
	})

	if info.Severity != "HIGH" {
		t.Fatalf("Severity = %q, want HIGH", info.Severity)
	}
	if info.CVSSScore == 0 {
		t.Fatal("CVSSScore was not parsed")
	}
}

func TestIsSecurityAdvisoryFiltersErrata(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"ALSA-2023:4838", true},   // AlmaLinux security
		{"ALBA-2022:2032", false},  // AlmaLinux bugfix
		{"ALEA-2022:1985", false},  // AlmaLinux enhancement
		{"RHSA-2024:0001", true},
		{"RHBA-2024:0001", false},
		{"RHEA-2024:0001", false},
		{"RLBA-2024:0001", false},  // Rocky bugfix
		{"ELBA-2024:0001", false},  // Oracle bugfix
		{"CVE-2024-12345", true},
		{"GHSA-xxxx-yyyy-zzzz", true},
		{"USN-6000-1", true},
	}
	for _, tt := range tests {
		if got := IsSecurityAdvisory(tt.id); got != tt.want {
			t.Errorf("IsSecurityAdvisory(%q) = %v, want %v", tt.id, got, tt.want)
		}
	}
}

func TestFilterSecurityVulns(t *testing.T) {
	in := []OSVVuln{
		{ID: "ALBA-2022:2032"},
		{ID: "ALSA-2023:4838"},
		{ID: "ALEA-2022:1985"},
		{ID: "CVE-2024-12345"},
	}
	out := filterSecurityVulns(in)
	if len(out) != 2 {
		t.Fatalf("expected 2 security vulns, got %d", len(out))
	}
	if out[0].ID != "ALSA-2023:4838" || out[1].ID != "CVE-2024-12345" {
		t.Fatalf("unexpected survivors: %v, %v", out[0].ID, out[1].ID)
	}
}
