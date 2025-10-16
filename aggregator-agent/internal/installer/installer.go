package installer

// Installer interface for different package types
type Installer interface {
	IsAvailable() bool
	Install(packageName string) (*InstallResult, error)
	InstallMultiple(packageNames []string) (*InstallResult, error)
	Upgrade() (*InstallResult, error)
	GetPackageType() string
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
	default:
		return nil, fmt.Errorf("unsupported package type: %s", packageType)
	}
}