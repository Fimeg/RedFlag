package installer

import "fmt"

// Installer interface for different package types
type Installer interface {
	IsAvailable() bool
	Install(packageName string) (*InstallResult, error)
	InstallMultiple(packageNames []string) (*InstallResult, error)
	Upgrade() (*InstallResult, error)
	UpdatePackage(packageName string) (*InstallResult, error)  // New: Update specific package
	GetPackageType() string
	DryRun(packageName string) (*InstallResult, error)  // New: Perform dry run to check dependencies
}

// InstallerFactory creates appropriate installer based on package type
func InstallerFactory(packageType string) (Installer, error) {
	switch packageType {
	case "apt":
		return NewAPTInstaller(), nil
	case "dnf":
		return NewDNFInstaller(), nil
	case "docker_image":
		return NewDockerInstaller()
	case "windows_update":
		return NewWindowsUpdateInstaller(), nil
	case "winget":
		return NewWingetInstaller(), nil
	default:
		return nil, fmt.Errorf("unsupported package type: %s", packageType)
	}
}