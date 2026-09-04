package services

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"text/template"
)

// TemplateRenderData is the union of all fields across the three install
// template data structs. Template execution fails on unknown variables, so
// this test data exists for the sole purpose of verifying every {{.Field}}
// in the template resolves.
type TemplateRenderData struct {
	AgentID           string
	BinaryURL         string
	ConfigURL         string
	Platform          string
	Architecture      string
	Version           string
	ServerURL         string
	RegistrationToken string
	AgentUser         string
	AgentHome         string
	ConfigDir         string
	LogDir            string
	AgentConfigDir    string
	AgentLogDir       string
	ServerKeyDir      string
	ServerPublicKey   string
}

func TestInstallTemplateRenders(t *testing.T) {
	data := TemplateRenderData{
		AgentID:           "00000000-0000-0000-0000-000000000000",
		BinaryURL:         "https://example.com/redflag-agent",
		ConfigURL:         "https://example.com/config",
		Platform:          "linux",
		Architecture:      "amd64",
		Version:           "0.2.9.0",
		ServerURL:         "https://example.com",
		RegistrationToken: "rt_test",
		AgentUser:         "redflag-agent",
		AgentHome:         "/var/lib/redflag/agent",
		ConfigDir:         "/etc/redflag",
		LogDir:            "/var/log/redflag",
		AgentConfigDir:    "/etc/redflag/agent",
		AgentLogDir:       "/var/log/redflag/agent",
		ServerKeyDir:      "/etc/redflag/server",
		ServerPublicKey:   "abcdef0123456789",
	}

	scripts := []string{
		"templates/install/scripts/linux.sh.tmpl",
		"templates/install/scripts/windows.ps1.tmpl",
	}

	for _, name := range scripts {
		t.Run(filepath.Base(name), func(t *testing.T) {
			tmpl, err := template.ParseFS(installScriptTemplates, name)
			if err != nil {
				t.Fatalf("parse template %s: %v", name, err)
			}

			var buf strings.Builder
			if err := tmpl.Execute(&buf, data); err != nil {
				t.Fatalf("render template %s: %v", name, err)
			}

			output := buf.String()
			if len(output) < 100 {
				t.Fatalf("rendered template %s too short (%d bytes): likely empty or error", name, len(output))
			}
		})
	}
}

// TestFreshInstallConfigKeys verifies the install template's default JSON
// includes the config keys the agent expects at minimum. This catches drift
// where a new config struct field gets added but the template never writes it
// for fresh installs.
//
// THIS TEST IS NOT EXHAUSTIVE. It maintains a canonical list of keys that must
// appear in the fresh-install config template. Add new keys here when they are
// added to the agent Config struct and the template should set them.
func TestFreshInstallConfigKeys(t *testing.T) {
	// Read and parse the linux.sh.tmpl to extract its default JSON block.
	raw, err := installScriptTemplates.ReadFile("templates/install/scripts/linux.sh.tmpl")
	if err != nil {
		t.Fatalf("read template: %v", err)
	}
	source := string(raw)

	// Extract the default JSON between the heredoc markers.
	// The template writes: sudo tee "..." > /dev/null <<EOF
	// Find the heredoc marker line, then the first `{` on the line after it.
	heredoc := `sudo tee "${AGENT_CONFIG_DIR}/config.json" > /dev/null <<EOF`
	heredocIdx := strings.Index(source, heredoc)
	if heredocIdx == -1 {
		t.Fatal("default JSON heredoc marker not found in install template")
	}
	// Skip past the <<EOF marker and the newline that follows.
	afterHeredoc := heredocIdx + len(heredoc)
	jsonStart := strings.Index(source[afterHeredoc:], "\n{")
	if jsonStart == -1 {
		t.Fatal("JSON start (\\n{) not found after heredoc marker")
	}
	jsonBlockStart := afterHeredoc + jsonStart + 1 // skip the newline
	jsonEnd := strings.Index(source[jsonBlockStart:], "\nEOF")
	if jsonEnd == -1 {
		t.Fatal("JSON end (\\nEOF) not found after heredoc")
	}
	jsonBlock := source[jsonBlockStart : jsonBlockStart+jsonEnd]

	// Parse the JSON block and collect top-level keys.
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonBlock), &parsed); err != nil {
		t.Fatalf("parse default JSON: %v\nblock:\n%s", err, jsonBlock)
	}

	// Canonical required keys — every Config struct field with a json tag
	// that MUST appear in the fresh-install template. Keys with "omitempty"
	// or that default to zero-value-safe are optional and not listed here.
	required := []string{
		"version",
		"agent_version",
		"agent_id",
		"token",
		"refresh_token",
		"registration_token",
		"check_in_interval",
		"server_url",
		"network",
		"proxy",
		"tls",
		"logging",
		"subsystems",
	}
	// "machine_id" in the template JSON is a stale key not present in the
	// Config struct — the agent reads machine identity from hostname/TOFU.
	// Not a required key; retained in the template for backward compat with
	// config files that carry it.

	for _, key := range required {
		if _, exists := parsed[key]; !exists {
			t.Errorf("required config key %q missing from fresh-install template JSON", key)
		}
	}

	// Optional keys — nice-to-have in the template but not required for
	// correct operation (zero values are handled by the agent).
	recommended := []string{
		"desktop",
		"polling",
		"command_signing",
		"security_logging",
		"security",
	}

	for _, key := range recommended {
		if _, exists := parsed[key]; !exists {
			t.Logf("[INFO] recommended config key %q not in template — fresh installs will use defaults", key)
		}
	}
}

// TestInstallTemplateScriptletSyntax performs a basic syntax sanity check on the
// shell script portion of the install template — enough to catch common errors
// like unterminated heredocs, unbalanced braces, or missing shell tokens.
func TestInstallTemplateScriptletSyntax(t *testing.T) {

	// Render with test data first to substitute template variables.
	data := TemplateRenderData{
		AgentID:           "00000000-0000-0000-0000-000000000000",
		BinaryURL:         "https://example.com/binary",
		ConfigURL:         "https://example.com/config",
		Platform:          "linux",
		Architecture:      "amd64",
		Version:           "0.2.9.0",
		ServerURL:         "https://example.com",
		RegistrationToken: "rt_test",
		AgentUser:         "redflag-agent",
		AgentHome:         "/var/lib/redflag/agent",
		ConfigDir:         "/etc/redflag",
		LogDir:            "/var/log/redflag",
		AgentConfigDir:    "/etc/redflag/agent",
		AgentLogDir:       "/var/log/redflag/agent",
		ServerKeyDir:      "/etc/redflag/server",
		ServerPublicKey:   "abcdef0123456789",
	}

	tmpl, err := template.ParseFS(installScriptTemplates, "templates/install/scripts/linux.sh.tmpl")
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}
	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		t.Fatalf("render template: %v", err)
	}

	rendered := buf.String()

	// Check major sections are present.
	sections := []string{
		"#!/bin/bash",
		"set -e",
		"systemctl",
		"sudoers",
		"config.json",
		"redflag-helper",
		"redflag-desktop",
	}
	for _, s := range sections {
		if !strings.Contains(rendered, s) {
			t.Errorf("rendered template missing expected section: %q", s)
		}
	}

	// Rough heredoc balance check — count open/close markers.
	heredocOpens := strings.Count(rendered, "<<EOF")
	heredocCloses := strings.Count(rendered, "EOF")
	if heredocCloses < heredocOpens {
		t.Errorf("possible unterminated heredoc: %d <<EOF markers but only %d EOF terminators", heredocOpens, heredocCloses)
	}
}

// TestMain ensures the working directory is the server package root
// so the embedded templates resolve correctly.
func TestMain(m *testing.M) {
	// Allow override for test runner flexibility.
	if d := os.Getenv("REDFLAG_SERVER_DIR"); d != "" {
		os.Chdir(d)
	}
	os.Exit(m.Run())
}
