package installer

import (
	"fmt"
	"os/exec"
	"strings"
)

// SecureCommandExecutor handles secure execution of privileged commands
type SecureCommandExecutor struct{}

// NewSecureCommandExecutor creates a new secure command executor
func NewSecureCommandExecutor() *SecureCommandExecutor {
	return &SecureCommandExecutor{}
}

// AllowedCommands defines the commands that can be executed with elevated privileges
var AllowedCommands = map[string][]string{
	"apt-get": {
		"update",
		"install",
		"upgrade",
	},
	"dnf": {
		"refresh",
		"makecache",
		"install",
		"upgrade",
	},
	"docker": {
		"pull",
		"image",
		"manifest",
	},
}

// validateCommand checks if a command is allowed to be executed
func (e *SecureCommandExecutor) validateCommand(baseCmd string, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("no arguments provided for command: %s", baseCmd)
	}

	allowedArgs, ok := AllowedCommands[baseCmd]
	if !ok {
		return fmt.Errorf("command not allowed: %s", baseCmd)
	}

	// Check if the first argument (subcommand) is allowed
	if !contains(allowedArgs, args[0]) {
		return fmt.Errorf("command not allowed: %s %s", baseCmd, args[0])
	}

	// Additional validation for specific commands
	switch baseCmd {
	case "apt-get":
		return e.validateAPTCommand(args)
	case "dnf":
		return e.validateDNFCommand(args)
	case "docker":
		return e.validateDockerCommand(args)
	}

	return nil
}

// validateAPTCommand performs additional validation for APT commands
func (e *SecureCommandExecutor) validateAPTCommand(args []string) error {
	switch args[0] {
	case "install":
		// Ensure install commands have safe flags
		if !contains(args, "-y") && !contains(args, "--yes") {
			return fmt.Errorf("apt-get install must include -y or --yes flag")
		}
		// Check for dangerous flags
		dangerousFlags := []string{"--allow-unauthenticated", "--allow-insecure-repositories"}
		for _, flag := range dangerousFlags {
			if contains(args, flag) {
				return fmt.Errorf("dangerous flag not allowed: %s", flag)
			}
		}
	case "upgrade":
		// Ensure upgrade commands have safe flags
		if !contains(args, "-y") && !contains(args, "--yes") {
			return fmt.Errorf("apt-get upgrade must include -y or --yes flag")
		}
	}
	return nil
}

// validateDNFCommand performs additional validation for DNF commands
func (e *SecureCommandExecutor) validateDNFCommand(args []string) error {
	switch args[0] {
	case "refresh":
		if !contains(args, "-y") {
			return fmt.Errorf("dnf refresh must include -y flag")
		}
	case "makecache":
		// makecache doesn't require -y flag as it's read-only
		return nil
	case "install":
		// Allow dry-run flags for dependency checking
		dryRunFlags := []string{"--assumeno", "--downloadonly"}
		hasDryRun := false
		for _, flag := range dryRunFlags {
			if contains(args, flag) {
				hasDryRun = true
				break
			}
		}
		// If it's a dry run, allow it without -y
		if hasDryRun {
			return nil
		}
		// Otherwise require -y flag for regular installs
		if !contains(args, "-y") {
			return fmt.Errorf("dnf install must include -y flag")
		}
	case "upgrade":
		if !contains(args, "-y") {
			return fmt.Errorf("dnf upgrade must include -y flag")
		}
	}
	return nil
}

// validateDockerCommand performs additional validation for Docker commands
func (e *SecureCommandExecutor) validateDockerCommand(args []string) error {
	switch args[0] {
	case "pull":
		if len(args) < 2 {
			return fmt.Errorf("docker pull requires an image name")
		}
		// Basic image name validation
		imageName := args[1]
		if strings.Contains(imageName, "..") || strings.HasPrefix(imageName, "-") {
			return fmt.Errorf("invalid docker image name: %s", imageName)
		}
	case "image":
		if len(args) < 2 {
			return fmt.Errorf("docker image requires a subcommand")
		}
		if args[1] != "inspect" {
			return fmt.Errorf("docker image subcommand not allowed: %s", args[1])
		}
		if len(args) < 3 {
			return fmt.Errorf("docker image inspect requires an image name")
		}
	case "manifest":
		if len(args) < 2 {
			return fmt.Errorf("docker manifest requires a subcommand")
		}
		if args[1] != "inspect" {
			return fmt.Errorf("docker manifest subcommand not allowed: %s", args[1])
		}
		if len(args) < 3 {
			return fmt.Errorf("docker manifest inspect requires an image name")
		}
	}
	return nil
}

// ExecuteCommand securely executes a command with validation
func (e *SecureCommandExecutor) ExecuteCommand(baseCmd string, args []string) (*InstallResult, error) {
	// Validate the command before execution
	if err := e.validateCommand(baseCmd, args); err != nil {
		return &InstallResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Command validation failed: %v", err),
		}, fmt.Errorf("command validation failed: %w", err)
	}

	// Resolve the full path to the command (required for sudo to match sudoers rules)
	fullPath, err := exec.LookPath(baseCmd)
	if err != nil {
		return &InstallResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Command not found: %s", baseCmd),
		}, fmt.Errorf("command not found: %w", err)
	}

	// Log the command for audit purposes (in a real implementation, this would go to a secure log)
	fmt.Printf("[AUDIT] Executing command: sudo %s %s\n", fullPath, strings.Join(args, " "))

	// Execute the command with sudo - requires sudoers configuration
	// Use full path to match sudoers rules exactly
	fullArgs := append([]string{fullPath}, args...)
	cmd := exec.Command("sudo", fullArgs...)

	output, err := cmd.CombinedOutput()

	if err != nil {
		return &InstallResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Command execution failed: %v", err),
			Stdout:       string(output),
			Stderr:       "",
			ExitCode:     getExitCode(err),
		}, err
	}

	return &InstallResult{
		Success:  true,
		Stdout:   string(output),
		Stderr:   "",
		ExitCode: 0,
	}, nil
}

// contains checks if a string slice contains a specific string
func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}