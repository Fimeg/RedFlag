package scanner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/installer"
)

// PacmanScanner scans for Arch Linux pacman package updates.
//
// Detection uses `checkupdates` (pacman-contrib), which syncs a private copy
// of the repo databases into a temp dir and runs `pacman -Qu` against it.
// This means:
//   - No root required (unprivileged discovery, same as dnf/apt)
//   - No mutation of the live pacman database
//   - checkupdates handles its own sandboxing internally
//
// checkupdates output format:
//   pkgname oldver -> newver
//   fakeroot 1:1.37.2-1 -> 1:1.37.2-2
//
// The current (installed) version is embedded in the output, so no secondary
// query is needed (unlike dnf which calls rpm -q).
type PacmanScanner struct{}

// NewPacmanScanner creates a new pacman scanner
func NewPacmanScanner() *PacmanScanner {
	return &PacmanScanner{}
}

func (s *PacmanScanner) Name() string { return "Pacman Update Scanner" }

// IsAvailable checks if pacman-contrib's checkupdates is installed.
// pacman alone is not sufficient — checkupdates is a separate package
// (pacman-contrib) that may not be installed.
func (s *PacmanScanner) IsAvailable() bool {
	if _, err := exec.LookPath("checkupdates"); err != nil {
		return false
	}
	_, err := exec.LookPath("pacman")
	return err == nil
}

// Scan runs checkupdates and parses the output into update report items.
func (s *PacmanScanner) Scan() ([]client.UpdateReportItem, error) {
	runner, err := installer.NewDiscoveryRunner("pacman")
	if err != nil {
		return nil, fmt.Errorf("[ERROR] [agent] [pacman] discovery_runner_create error=%w", err)
	}
	// checkupdates needs --color=never for parseable output
	result, err := runner.Run(context.Background(), "--color=never")
	if err != nil && (result == nil || result.ExitCode != 0) {
		return nil, fmt.Errorf("[ERROR] [agent] [pacman] checkupdates_failed error=%w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("[ERROR] [agent] [pacman] checkupdates_no_result")
	}

	return parsePacmanOutput([]byte(result.Stdout))
}

// checkupdatesLine matches: pkgname oldver -> newver
var checkupdatesLine = regexp.MustCompile(`^([^\s]+)\s+(\S+)\s+->\s+(\S+)$`)

func parsePacmanOutput(output []byte) ([]client.UpdateReportItem, error) {
	var updates []client.UpdateReportItem
	scanner := bufio.NewScanner(bytes.NewReader(output))

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		matches := checkupdatesLine.FindStringSubmatch(line)
		if len(matches) < 4 {
			continue
		}

		packageName := matches[1]
		currentVersion := matches[2]
		availableVersion := matches[3]

		// Determine repository via pacman -Si (best effort, unprivileged)
		repository := getPacmanRepository(packageName)

		severity := determinePacmanSeverity(packageName)

		update := client.UpdateReportItem{
			PackageType:      "pacman",
			PackageName:      packageName,
			CurrentVersion:   currentVersion,
			AvailableVersion: availableVersion,
			Severity:         severity,
			RepositorySource: repository,
			Metadata: map[string]interface{}{
				"architecture": "x86_64", // checkupdates doesn't report arch; Arch is typically single-arch per machine
			},
		}

		updates = append(updates, update)
	}

	return updates, nil
}

// getPacmanRepository queries pacman -Si for the package's repository.
// Returns "unknown" if the query fails (best effort — the update data is
// still valid without it).
func getPacmanRepository(packageName string) string {
	cmd := exec.Command("pacman", "-Si", "--color=never", packageName)
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(output), "\n") {
		if strings.HasPrefix(line, "Repository") {
			// Format: "Repository      : core"
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return "unknown"
}

// determinePacmanSeverity assigns a heuristic severity for Arch packages.
// Arch does not have RPM/Debian-style security advisories — security updates
// are intermixed with all other updates in the rolling release. This heuristic
// flags packages that are commonly security-sensitive based on package name.
// OSV.dev (Arch ecosystem) provides actual vulnerability data at the server
// level during the supply-chain check.
func determinePacmanSeverity(packageName string) string {
	lower := strings.ToLower(packageName)

	// Trust root / crypto
	if strings.Contains(lower, "keyring") ||
		strings.Contains(lower, "archlinux-keyring") ||
		strings.Contains(lower, "openssl") ||
		strings.Contains(lower, "gnutls") ||
		strings.Contains(lower, "crypto") ||
		strings.Contains(lower, "nss") ||
		strings.Contains(lower, "gnupg") ||
		strings.Contains(lower, "gpgme") {
		return "critical"
	}

	// Kernel and boot
	if strings.Contains(lower, "linux") || // linux, linux-lts, linux-zen, linux-hardened
		strings.Contains(lower, "kernel") ||
		strings.Contains(lower, "systemd") ||
		strings.Contains(lower, "grub") ||
		strings.Contains(lower, "bootloader") {
		return "important"
	}

	// Core system
	if strings.Contains(lower, "glibc") ||
		strings.Contains(lower, "bash") ||
		strings.Contains(lower, "coreutils") ||
		strings.Contains(lower, "sudo") ||
		strings.Contains(lower, "pam") ||
		strings.Contains(lower, "openssh") ||
		strings.Contains(lower, "shadow") {
		return "important"
	}

	// Network-facing
	if strings.Contains(lower, "networkmanager") ||
		strings.Contains(lower, "iptables") ||
		strings.Contains(lower, "nftables") ||
		strings.Contains(lower, "wireguard") ||
		strings.Contains(lower, "openvpn") ||
		strings.Contains(lower, "curl") ||
		strings.Contains(lower, "wget") ||
		strings.Contains(lower, "openssh") {
		return "moderate"
	}

	// Runtime / development
	if strings.Contains(lower, "python") ||
		strings.Contains(lower, "nodejs") ||
		strings.Contains(lower, "go") ||
		strings.Contains(lower, "rust") ||
		strings.Contains(lower, "gcc") ||
		strings.Contains(lower, "java") ||
		strings.Contains(lower, "dotnet") {
		return "moderate"
	}

	return "low"
}
