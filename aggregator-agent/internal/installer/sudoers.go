package installer

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

// SudoersConfig represents the sudoers configuration for the RedFlag agent
const SudoersTemplate = `# RedFlag Agent minimal sudo permissions
# This file is generated automatically during RedFlag agent installation
# Location: /etc/sudoers.d/redflag-agent

# APT package management commands
redflag-agent ALL=(root) NOPASSWD: /usr/bin/apt-get update
redflag-agent ALL=(root) NOPASSWD: /usr/bin/apt-get install -y *
redflag-agent ALL=(root) NOPASSWD: /usr/bin/apt-get upgrade -y
redflag-agent ALL=(root) NOPASSWD: /usr/bin/apt-get install --dry-run --yes *

# DNF package management commands
redflag-agent ALL=(root) NOPASSWD: /usr/bin/dnf refresh -y
redflag-agent ALL=(root) NOPASSWD: /usr/bin/dnf install -y *
redflag-agent ALL=(root) NOPASSWD: /usr/bin/dnf upgrade -y
redflag-agent ALL=(root) NOPASSWD: /usr/bin/dnf install --assumeno --downloadonly *

# Docker operations (alternative approach - uncomment if using Docker group instead of sudo)
# redflag-agent ALL=(root) NOPASSWD: /usr/bin/docker pull *
# redflag-agent ALL=(root) NOPASSWD: /usr/bin/docker image inspect *
# redflag-agent ALL=(root) NOPASSWD: /usr/bin/docker manifest inspect *
`

// SudoersInstaller handles the installation of sudoers configuration
type SudoersInstaller struct{}

// NewSudoersInstaller creates a new sudoers installer
func NewSudoersInstaller() *SudoersInstaller {
	return &SudoersInstaller{}
}

// InstallSudoersConfig installs the sudoers configuration
func (s *SudoersInstaller) InstallSudoersConfig() error {
	// Create the sudoers configuration content
	tmpl, err := template.New("sudoers").Parse(SudoersTemplate)
	if err != nil {
		return fmt.Errorf("failed to parse sudoers template: %w", err)
	}

	// Ensure the sudoers.d directory exists
	sudoersDir := "/etc/sudoers.d"
	if _, err := os.Stat(sudoersDir); os.IsNotExist(err) {
		if err := os.MkdirAll(sudoersDir, 0755); err != nil {
			return fmt.Errorf("failed to create sudoers.d directory: %w", err)
		}
	}

	// Create the sudoers file
	sudoersFile := filepath.Join(sudoersDir, "redflag-agent")
	file, err := os.OpenFile(sudoersFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0440)
	if err != nil {
		return fmt.Errorf("failed to create sudoers file: %w", err)
	}
	defer file.Close()

	// Write the template to the file
	if err := tmpl.Execute(file, nil); err != nil {
		return fmt.Errorf("failed to write sudoers configuration: %w", err)
	}

	// Verify the sudoers file syntax
	if err := s.validateSudoersFile(sudoersFile); err != nil {
		// Remove the invalid file
		os.Remove(sudoersFile)
		return fmt.Errorf("invalid sudoers configuration: %w", err)
	}

	fmt.Printf("Successfully installed sudoers configuration at: %s\n", sudoersFile)
	return nil
}

// validateSudoersFile validates the syntax of a sudoers file
func (s *SudoersInstaller) validateSudoersFile(sudoersFile string) error {
	// Use visudo to validate the sudoers file
	cmd := exec.Command("visudo", "-c", "-f", sudoersFile)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("sudoers validation failed: %v\nOutput: %s", err, string(output))
	}
	return nil
}

// CreateRedflagAgentUser creates the redflag-agent user if it doesn't exist
func (s *SudoersInstaller) CreateRedflagAgentUser() error {
	// Check if user already exists
	if _, err := os.Stat("/var/lib/redflag-agent"); err == nil {
		fmt.Println("redflag-agent user already exists")
		return nil
	}

	// Create the user with systemd as a system user
	commands := [][]string{
		{"useradd", "-r", "-s", "/bin/false", "-d", "/var/lib/redflag-agent", "redflag-agent"},
		{"mkdir", "-p", "/var/lib/redflag-agent"},
		{"chown", "redflag-agent:redflag-agent", "/var/lib/redflag-agent"},
	}

	for _, cmdArgs := range commands {
		cmd := exec.Command(cmdArgs[0], cmdArgs[1:]...)
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("failed to execute %v: %v\nOutput: %s", cmdArgs, err, string(output))
		}
	}

	fmt.Println("Successfully created redflag-agent user")
	return nil
}

// SetupDockerGroup adds the redflag-agent user to the docker group (alternative to sudo for Docker)
func (s *SudoersInstaller) SetupDockerGroup() error {
	// Check if docker group exists
	if _, err := os.Stat("/var/run/docker.sock"); os.IsNotExist(err) {
		fmt.Println("Docker is not installed, skipping docker group setup")
		return nil
	}

	// Add user to docker group
	cmd := exec.Command("usermod", "-aG", "docker", "redflag-agent")
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("failed to add redflag-agent to docker group: %v\nOutput: %s", err, string(output))
	}

	fmt.Println("Successfully added redflag-agent to docker group")
	return nil
}

// CreateSystemdService creates a systemd service file for the agent
func (s *SudoersInstaller) CreateSystemdService() error {
	const serviceTemplate = `[Unit]
Description=RedFlag Update Agent
After=network.target

[Service]
Type=simple
User=redflag-agent
Group=redflag-agent
WorkingDirectory=/var/lib/redflag-agent
ExecStart=/usr/local/bin/redflag-agent
Restart=always
RestartSec=30

# Security hardening
NoNewPrivileges=true
ProtectSystem=strict
ProtectHome=true
ReadWritePaths=/var/lib/redflag-agent
PrivateTmp=true

[Install]
WantedBy=multi-user.target
`

	serviceFile := "/etc/systemd/system/redflag-agent.service"
	file, err := os.OpenFile(serviceFile, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create systemd service file: %w", err)
	}
	defer file.Close()

	if _, err := file.WriteString(serviceTemplate); err != nil {
		return fmt.Errorf("failed to write systemd service file: %w", err)
	}

	// Reload systemd
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemd: %w", err)
	}

	fmt.Printf("Successfully created systemd service at: %s\n", serviceFile)
	return nil
}

// Cleanup removes sudoers configuration
func (s *SudoersInstaller) Cleanup() error {
	sudoersFile := "/etc/sudoers.d/redflag-agent"
	if _, err := os.Stat(sudoersFile); err == nil {
		if err := os.Remove(sudoersFile); err != nil {
			return fmt.Errorf("failed to remove sudoers file: %w", err)
		}
		fmt.Println("Successfully removed sudoers configuration")
	}
	return nil
}