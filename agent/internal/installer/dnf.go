package installer

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// HashVerifier checks package hashes against expected values
type HashVerifier struct {
	serverURL string
}

// NewHashVerifier creates a new hash verifier
func NewHashVerifier(serverURL string) *HashVerifier {
	return &HashVerifier{
		serverURL: serverURL,
	}
}

// VerifyPackageHash checks that the package's current canonical artifact hash
// matches the pinned expected value. The hash is re-resolved locally from the
// agent's signed repo metadata (the same source the server pinned), so this
// catches an artifact swapped under a fixed version since pinning. dnf cannot be
// fetched from the server (the server has no access to the agent's repos), so
// verification is agent-local; RPM's own GPG check remains as a second layer at
// install.
func (h *HashVerifier) VerifyPackageHash(packageName, version, expectedSHA256 string) error {
	if expectedSHA256 == "" {
		return fmt.Errorf("no expected hash registered for package %q — hash verification is mandatory for capability-gated installs", packageName)
	}

	resolved, err := ResolveArtifactSHA256("dnf", packageName, version)
	if err != nil {
		return fmt.Errorf("failed to resolve package artifact: %w", err)
	}

	if !strings.EqualFold(resolved.SHA256, expectedSHA256) {
		return fmt.Errorf("package hash mismatch: expected %s, got %s", expectedSHA256, resolved.SHA256)
	}

	return nil
}

// DNFInstaller handles DNF package installations
type DNFInstaller struct {
	hashVerifier   *HashVerifier
	expectedSHA256 string
}

// NewDNFInstaller creates a new DNF installer with hash verification
func NewDNFInstaller(serverURL string) *DNFInstaller {
	return &DNFInstaller{
		hashVerifier:   NewHashVerifier(serverURL),
		expectedSHA256: "",
	}
}

// IsAvailable checks if DNF is available on this system
func (i *DNFInstaller) IsAvailable() bool {
	_, err := exec.LookPath("dnf")
	return err == nil
}

// DryRun performs a dry run installation to check dependencies
func (i *DNFInstaller) DryRun(packageName, version string) (*InstallResult, error) {
	startTime := time.Now()

	runner, err := NewDiscoveryRunner("dnf")
	if err != nil {
		return nil, fmt.Errorf("[ERROR] [agent] [installer] discovery_runner_create error=%w", err)
	}

	// makecache — best effort, don't fail if it doesn't work
	if _, refreshErr := runner.Run(context.Background(), "makecache"); refreshErr != nil {
		log.Printf("Warning: DNF makecache failed (continuing with dry run): %v", refreshErr)
	}

	// NEVRA: packageName-version resolves the specific upgrade, not "is it installed?"
	target := packageName
	if version != "" {
		target = packageName + "-" + version
	}

	result, err := runner.Run(context.Background(), "install", "--assumeno", "--downloadonly", target)
	duration := int(time.Since(startTime).Seconds())

	if result == nil {
		result = &RunResult{}
	}
	deps := i.parseDependenciesFromDNFOutput(result.Stdout, packageName)

	installResult := &InstallResult{
		Stdout:          result.Stdout,
		Stderr:          result.Stderr,
		ExitCode:        result.ExitCode,
		DurationSeconds: duration,
		IsDryRun:        true,
		Action:          "dry_run",
	}

	// --assumeno cancels the transaction, so DNF exits non-zero on a dry run
	// that resolved successfully — a non-zero exit is not by itself a failure.
	// But a non-empty stdout is not success either: "No match for argument",
	// "Nothing to do", and "Error:" all print output and exit non-zero. Success
	// means DNF actually resolved a transaction that would change packages,
	// marked by a "Transaction Summary" block (which never coexists with
	// "Nothing to do"). Gate on that, whether or not there are extra deps.
	if dnfTransactionResolved(result.Stdout) {
		installResult.Success = true
		installResult.Dependencies = deps
		return installResult, nil
	}

	installResult.Success = false
	if err != nil {
		installResult.ErrorMessage = fmt.Sprintf("DNF dry run did not resolve a transaction: %v", err)
		return installResult, err
	}
	installResult.ErrorMessage = "DNF dry run did not resolve an installable transaction"
	return installResult, fmt.Errorf("[ERROR] [agent] [installer] dnf_dry_run_no_transaction package=%s", target)
}

// dnfTransactionResolved reports whether DNF dry-run output describes a
// transaction that would actually change packages. "Transaction Summary" is
// printed only when at least one package will be installed/upgraded/removed and
// never appears alongside "Nothing to do", so it is the reliable success signal
// — distinct from "got output," which failed resolutions also produce.
func dnfTransactionResolved(output string) bool {
	if strings.Contains(output, "Nothing to do") {
		return false
	}
	return strings.Contains(output, "Transaction Summary")
}

// parseDependenciesFromDNFOutput extracts dependency package names from DNF dry run output
func (i *DNFInstaller) parseDependenciesFromDNFOutput(output string, packageName string) []string {
	var dependencies []string

	// Regex patterns to find dependencies in DNF output
	patterns := []*regexp.Regexp{
		// Match "Installing dependencies:" section
		regexp.MustCompile(`(?s)Installing dependencies:(.*?)(\n\n|\z|Transaction Summary:)`),
		// Match "Dependencies resolved." section and package list
		regexp.MustCompile(`(?s)Dependencies resolved\.(.*?)(\n\n|\z|Transaction Summary:)`),
		// Match package installation lines
		regexp.MustCompile(`^\s*([a-zA-Z0-9][a-zA-Z0-9+._-]*)\s+[a-zA-Z0-9:.]+(?:\s+[a-zA-Z]+)?$`),
	}

	for _, pattern := range patterns {
		if strings.Contains(pattern.String(), "Installing dependencies:") {
			matches := pattern.FindStringSubmatch(output)
			if len(matches) > 1 {
				// Extract package names from the dependencies section
				lines := strings.Split(matches[1], "\n")
				for _, line := range lines {
					line = strings.TrimSpace(line)
					if line != "" && !strings.Contains(line, "Dependencies") {
						pkg := i.extractPackageNameFromDNFLine(line)
						if pkg != "" {
							dependencies = append(dependencies, pkg)
						}
					}
				}
			}
		}
	}

	// Also look for transaction summary which lists all packages to be installed
	transactionPattern := regexp.MustCompile(`(?s)Transaction Summary:\s*\n\s*Install\s+(\d+) Packages?\s*\n((?:\s+\d+\s+[a-zA-Z0-9+._-]+\s+[a-zA-Z0-9:.]+.*\n?)*)`)
	matches := transactionPattern.FindStringSubmatch(output)
	if len(matches) > 2 {
		installLines := strings.Split(matches[2], "\n")
		for _, line := range installLines {
			line = strings.TrimSpace(line)
			if line != "" {
				pkg := i.extractPackageNameFromDNFLine(line)
				if pkg != "" && pkg != packageName {
					dependencies = append(dependencies, pkg)
				}
			}
		}
	}

	// Remove duplicates
	uniqueDeps := make([]string, 0)
	seen := make(map[string]bool)
	for _, dep := range dependencies {
		if dep != packageName && !seen[dep] {
			seen[dep] = true
			uniqueDeps = append(uniqueDeps, dep)
		}
	}

	return uniqueDeps
}

// extractPackageNameFromDNFLine extracts package name from a DNF output line
func (i *DNFInstaller) extractPackageNameFromDNFLine(line string) string {
	// Remove architecture info if present
	if idx := strings.LastIndex(line, "."); idx > 0 {
		archSuffix := line[idx:]
		if strings.Contains(archSuffix, ".x86_64") || strings.Contains(archSuffix, ".noarch") ||
			strings.Contains(archSuffix, ".i386") || strings.Contains(archSuffix, ".arm64") {
			line = line[:idx]
		}
	}

	// Extract package name (typically at the start of the line)
	fields := strings.Fields(line)
	if len(fields) > 0 {
		pkg := fields[0]
		// Remove version info if present
		if idx := strings.Index(pkg, "-"); idx > 0 {
			potentialName := pkg[:idx]
			// Check if this looks like a version (contains numbers)
			versionPart := pkg[idx+1:]
			if strings.Contains(versionPart, ".") || regexp.MustCompile(`\d`).MatchString(versionPart) {
				return potentialName
			}
		}
		return pkg
	}

	return ""
}

// GetPackageType returns type of packages this installer handles
func (i *DNFInstaller) GetPackageType() string {
	return "dnf"
}

// VerifyHash verifies the package hash before installation
func (i *DNFInstaller) VerifyHash(packageName, version, expectedSHA256 string) error {
	return i.hashVerifier.VerifyPackageHash(packageName, version, expectedSHA256)
}
