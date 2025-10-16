package installer

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/aggregator-project/aggregator-agent/internal/client"
)

// DockerInstaller handles Docker image updates
type DockerInstaller struct{}

// NewDockerInstaller creates a new Docker installer
func NewDockerInstaller() (*DockerInstaller, error) {
	// Check if docker is available first
	if _, err := exec.LookPath("docker"); err != nil {
		return nil, err
	}

	return &DockerInstaller{}, nil
}

// IsAvailable checks if Docker is available on this system
func (i *DockerInstaller) IsAvailable() bool {
	_, err := exec.LookPath("docker")
	return err == nil
}

// Update pulls a new image using docker CLI
func (i *DockerInstaller) Update(imageName, targetVersion string) (*InstallResult, error) {
	startTime := time.Now()

	// Pull the new image
	fmt.Printf("Pulling Docker image: %s...\n", imageName)
	pullCmd := exec.Command("sudo", "docker", "pull", imageName)
	output, err := pullCmd.CombinedOutput()
	if err != nil {
		return &InstallResult{
			Success:      false,
			ErrorMessage:   fmt.Sprintf("Failed to pull Docker image: %v\nStdout: %s", err, string(output)),
			Stdout:       string(output),
			Stderr:       "",
			ExitCode:     getExitCode(err),
			DurationSeconds: int(time.Since(startTime).Seconds()),
			Action:       "pull",
		}, fmt.Errorf("docker pull failed: %w", err)
	}

	fmt.Printf("Successfully pulled image: %s\n", string(output))

	duration := int(time.Since(startTime).Seconds())
	return &InstallResult{
		Success:          true,
		Stdout:           string(output),
		Stderr:           "",
		ExitCode:         0,
		DurationSeconds:    duration,
		Action:            "pull",
		ContainersUpdated: []string{}, // Would find and recreate containers in a real implementation
	}, nil
}

// Install installs a Docker image (alias for Update)
func (i *DockerInstaller) Install(imageName string) (*InstallResult, error) {
	return i.Update(imageName, "")
}

// InstallMultiple installs multiple Docker images
func (i *DockerInstaller) InstallMultiple(imageNames []string) (*InstallResult, error) {
	if len(imageNames) == 0 {
		return &InstallResult{
			Success:      false,
			ErrorMessage: "No images specified for installation",
		}, fmt.Errorf("no images specified")
	}

	startTime := time.Now()
	var allOutput strings.Builder
	var errors []string

	for _, imageName := range imageNames {
		fmt.Printf("Pulling Docker image: %s...\n", imageName)
		pullCmd := exec.Command("sudo", "docker", "pull", imageName)
		output, err := pullCmd.CombinedOutput()
		allOutput.WriteString(string(output))

		if err != nil {
			errors = append(errors, fmt.Sprintf("Failed to pull %s: %v", imageName, err))
		} else {
			fmt.Printf("Successfully pulled image: %s\n", imageName)
		}
	}

	duration := int(time.Since(startTime).Seconds())

	if len(errors) > 0 {
		return &InstallResult{
			Success:      false,
			ErrorMessage: fmt.Sprintf("Docker pull errors: %v", strings.Join(errors, "; ")),
			Stdout:       allOutput.String(),
			Stderr:       "",
			ExitCode:     1,
			DurationSeconds: duration,
			Action:       "pull_multiple",
		}, fmt.Errorf("docker pull failed for some images")
	}

	return &InstallResult{
		Success:          true,
		Stdout:           allOutput.String(),
		Stderr:           "",
		ExitCode:         0,
		DurationSeconds:    duration,
		Action:            "pull_multiple",
		ContainersUpdated: imageNames,
	}, nil
}

// Upgrade is not applicable for Docker in the same way
func (i *DockerInstaller) Upgrade() (*InstallResult, error) {
	return &InstallResult{
		Success:      false,
		ErrorMessage: "Docker upgrade not implemented - use specific image updates",
		ExitCode:     1,
		DurationSeconds: 0,
		Action:       "upgrade",
	}, fmt.Errorf("docker upgrade not implemented")
}

// GetPackageType returns type of packages this installer handles
func (i *DockerInstaller) GetPackageType() string {
	return "docker_image"
}

// getExitCode extracts exit code from exec error
func getExitCode(err error) int {
	if err == nil {
		return 0
	}

	if exitError, ok := err.(*exec.ExitError); ok {
		return exitError.ExitCode()
	}

	return 1 // Default error code
}