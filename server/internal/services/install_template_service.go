package services

import (
	"bytes"
	"embed"
	"fmt"
	"log"
	"strings"
	"text/template"

	"github.com/Fimeg/RedFlag/server/internal/models"
	"github.com/gofrs/uuid/v5"
)

//go:embed templates/install/scripts/*.tmpl
var installScriptTemplates embed.FS

// InstallTemplateService renders installation scripts from templates
type InstallTemplateService struct{}

// NewInstallTemplateService creates a new template service
func NewInstallTemplateService() *InstallTemplateService {
	return &InstallTemplateService{}
}

// RenderInstallScript renders an installation script for the specified platform
func (s *InstallTemplateService) RenderInstallScript(agent *models.Agent, binaryURL, configURL string) (string, error) {
	// Define template data
	data := struct {
		AgentID        string
		BinaryURL      string
		ConfigURL      string
		Platform       string
		Architecture   string
		Version        string
		AgentUser      string
		AgentHome      string
		ConfigDir      string
		LogDir         string
		AgentConfigDir string
		AgentLogDir    string
		ServerKeyDir   string
	}{
		AgentID:        agent.ID.String(),
		BinaryURL:      binaryURL,
		ConfigURL:      configURL,
		Platform:       agent.OSType,
		Architecture:   agent.OSArchitecture,
		Version:        agent.CurrentVersion,
		AgentUser:      "redflag-agent",
		AgentHome:      "/var/lib/redflag/agent",
		ConfigDir:      "/etc/redflag",
		LogDir:         "/var/log/redflag",
		AgentConfigDir: "/etc/redflag/agent",
		AgentLogDir:    "/var/log/redflag/agent",
		ServerKeyDir:   "/etc/redflag/server",
	}

	// Choose template based on platform
	var templateName string
	if strings.Contains(agent.OSType, "windows") {
		templateName = "templates/install/scripts/windows.ps1.tmpl"
	} else {
		templateName = "templates/install/scripts/linux.sh.tmpl"
	}

	// Load and parse template
	tmpl, err := template.ParseFS(installScriptTemplates, templateName)
	if err != nil {
		return "", fmt.Errorf("failed to load template: %w", err)
	}

	// Render template
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", fmt.Errorf("failed to render template: %w", err)
	}

	return buf.String(), nil
}

// RenderInstallScriptFromBuild renders script using build response
// ISSUE-002: Added serverPublicKey parameter for signature verification
func (s *InstallTemplateService) RenderInstallScriptFromBuild(
	agentIDParam string,
	platform string,
	architecture string,
	version string,
	serverURL string,
	registrationToken string,
	serverPublicKey string,
) (string, error) {
	// Extract or generate agent ID
	agentID := s.extractOrGenerateAgentID(agentIDParam)

	// Build correct URLs in Go, not templates
	binaryURL := fmt.Sprintf("%s/api/v1/downloads/%s-%s?version=%s", serverURL, platform, architecture, version)
	configURL := fmt.Sprintf("%s/api/v1/downloads/config/%s", serverURL, agentID)

	data := struct {
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
		ServerPublicKey   string // ISSUE-002: For signature verification
	}{
		AgentID:           agentID,
		BinaryURL:         binaryURL,
		ConfigURL:         configURL,
		Platform:          platform,
		Architecture:      architecture,
		Version:           version,
		ServerURL:         serverURL,
		RegistrationToken: registrationToken,
		AgentUser:         "redflag-agent",
		AgentHome:         "/var/lib/redflag/agent",
		ConfigDir:         "/etc/redflag",
		LogDir:            "/var/log/redflag",
		AgentConfigDir:    "/etc/redflag/agent",
		AgentLogDir:       "/var/log/redflag/agent",
		ServerKeyDir:      "/etc/redflag/server",
		ServerPublicKey:   serverPublicKey, // ISSUE-002
	}

	templateName := "templates/install/scripts/linux.sh.tmpl"
	if strings.Contains(platform, "windows") {
		templateName = "templates/install/scripts/windows.ps1.tmpl"
	}

	tmpl, err := template.ParseFS(installScriptTemplates, templateName)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// BuildAgentConfigWithAgentID builds config for an existing agent (for upgrades)
func (s *InstallTemplateService) BuildAgentConfigWithAgentID(
	agentID string,
	platform string,
	architecture string,
	version string,
	serverURL string,
) (string, error) {
	// Validate agent ID
	if _, err := uuid.FromString(agentID); err != nil {
		return "", fmt.Errorf("invalid agent ID: %w", err)
	}

	// Build correct URLs using existing agent ID
	binaryURL := fmt.Sprintf("%s/api/v1/downloads/%s-%s?version=%s", serverURL, platform, architecture, version)
	configURL := fmt.Sprintf("%s/api/v1/downloads/config/%s", serverURL, agentID)

	data := struct {
		AgentID      string
		BinaryURL    string
		ConfigURL    string
		Platform     string
		Architecture string
		Version      string
		ServerURL    string
		AgentUser    string
		AgentHome    string
		ConfigDir    string
		LogDir       string
		ServerKeyDir string
	}{
		AgentID:      agentID,
		BinaryURL:    binaryURL,
		ConfigURL:    configURL,
		Platform:     platform,
		Architecture: architecture,
		Version:      version,
		ServerURL:    serverURL,
		AgentUser:    "redflag-agent",
		AgentHome:    "/var/lib/redflag-agent",
		ConfigDir:    "/etc/redflag",
		LogDir:       "/var/log/redflag",
		ServerKeyDir: "/etc/redflag/server",
	}

	templateName := "templates/install/scripts/linux.sh.tmpl"
	if strings.Contains(platform, "windows") {
		templateName = "templates/install/scripts/windows.ps1.tmpl"
	}

	tmpl, err := template.ParseFS(installScriptTemplates, templateName)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// extractOrGenerateAgentID extracts or generates a valid agent ID
func (s *InstallTemplateService) extractOrGenerateAgentID(param string) string {
	log.Printf("[DEBUG] extractOrGenerateAgentID received param: %s", param)

	// If we got a real agent ID (UUID format), validate and use it
	if param != "" && param != "<AGENT_ID>" {
		// Validate it's a UUID
		if _, err := uuid.FromString(param); err == nil {
			log.Printf("[DEBUG] Using passed UUID: %s", param)
			return param
		}
		log.Printf("[DEBUG] Invalid UUID format, generating new one")
	}

	// Placeholder case - generate new UUID for fresh installation
	newID := uuid.Must(uuid.NewV4()).String()
	log.Printf("[DEBUG] Generated new UUID: %s", newID)
	return newID
}
