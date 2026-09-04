package installer

import "fmt"

// Installer is the agent-side interface for package ecosystems.
// Discovery methods only. Mutation (install, upgrade) goes through
// the capability-token path (supplychain.Consumer → redflag-helper),
// not through this interface.
type Installer interface {
	IsAvailable() bool
	GetPackageType() string
	DryRun(packageName, version string) (*InstallResult, error)
	VerifyHash(packageName, version, expectedSHA256 string) error
}

// NonGatedInstaller is the set of ecosystems still permitted to mutate
// directly from the agent (winget, docker, windows_update). dnf/apt
// deliberately do NOT implement these methods — their mutation flows only
// through the capability-token path (supplychain.Consumer → redflag-helper).
//
// This interface is the ledger of "what may still bypass the gate." As
// ecosystems move behind the capability gate, drop them from this set and
// the type assertions in the handlers begin failing for them automatically —
// the boundary is structural, not a guard someone has to remember to add.
type NonGatedInstaller interface {
	UpdatePackage(packageName string) (*InstallResult, error)
	Upgrade() (*InstallResult, error)
	InstallMultiple(packageNames []string) (*InstallResult, error)
}

// InstallerFactory creates appropriate installer based on package type
func InstallerFactory(packageType string, serverURL string) (Installer, error) {
	switch packageType {
	case "apt":
		return NewAPTInstaller(), nil
	case "dnf":
		return NewDNFInstaller(serverURL), nil
	case "docker_image":
		installer, err := NewDockerInstaller()
		if err != nil {
			return nil, fmt.Errorf("docker installer failed: %w", err)
		}
		return installer, nil
	case "windows_update":
		return NewWindowsUpdateInstaller(), nil
	case "winget":
		return NewWingetInstaller(), nil
	default:
		return nil, fmt.Errorf("unsupported package type: %s", packageType)
	}
}