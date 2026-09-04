package system

import "time"

const MaxPackageFiles = 5000

// SoftwareSnapshot is the Agent-owned view of installed software. Managers
// are reported independently so one broken inventory source does not turn
// another manager's known state into absence.
type SoftwareSnapshot struct {
	Supported       bool               `json:"supported"`
	Managers        []SoftwareManager  `json:"managers"`
	Packages        []InstalledPackage `json:"packages"`
	Count           int                `json:"count"`
	ExplicitCount   int                `json:"explicit_count"`
	DependencyCount int                `json:"dependency_count"`
	ForeignCount    int                `json:"foreign_count"`
	CollectedAt     time.Time          `json:"collected_at"`
}

// SoftwareManager records the state of one inventory source. Available and
// Error stay separate: installed-but-failing is not the same as absent.
type SoftwareManager struct {
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Count     int    `json:"count"`
	Error     string `json:"error,omitempty"`
}

// InstalledPackage is the common local-machine package identity. Identity is
// the exact manager-owned query key; Name is the human/cross-domain package
// name used to join pending updates where that mapping is unambiguous.
type InstalledPackage struct {
	PackageType        string `json:"package_type"`
	Identity           string `json:"identity"`
	Name               string `json:"name"`
	Version            string `json:"version"`
	Architecture       string `json:"architecture,omitempty"`
	Description        string `json:"description,omitempty"`
	Repository         string `json:"repository,omitempty"`
	InstallReason      string `json:"install_reason"` // explicit, dependency, unknown
	Origin             string `json:"origin"`         // repository, foreign, unknown
	InstalledSizeBytes uint64 `json:"installed_size_bytes,omitempty"`
	InstalledAt        string `json:"installed_at,omitempty"`
}

// PackageDetail adds provenance and relationship data only when an operator
// drills into one package. Files are bounded; FileCount preserves the real
// total when the returned list is truncated.
type PackageDetail struct {
	InstalledPackage
	URL                  string   `json:"url,omitempty"`
	Licenses             []string `json:"licenses,omitempty"`
	Groups               []string `json:"groups,omitempty"`
	DependsOn            []string `json:"depends_on,omitempty"`
	OptionalDependencies []string `json:"optional_dependencies,omitempty"`
	RequiredBy           []string `json:"required_by,omitempty"`
	Provides             []string `json:"provides,omitempty"`
	ConflictsWith        []string `json:"conflicts_with,omitempty"`
	Replaces             []string `json:"replaces,omitempty"`
	Packager             string   `json:"packager,omitempty"`
	BuildDate            string   `json:"build_date,omitempty"`
	Files                []string `json:"files,omitempty"`
	FileCount            int      `json:"file_count"`
	FilesTruncated       bool     `json:"files_truncated"`
}

// SoftwareOwner is the package identity that owns an executable path.
type SoftwareOwner struct {
	PackageType string `json:"package_type"`
	PackageName string `json:"package_name"`
}

func GetSoftwareSnapshot() (*SoftwareSnapshot, error) { return getSoftwareSnapshot() }

func GetPackageDetail(packageType, identity string) (*PackageDetail, error) {
	return getPackageDetail(packageType, identity)
}

func FindSoftwareOwner(path string) (*SoftwareOwner, error) { return findSoftwareOwner(path) }
