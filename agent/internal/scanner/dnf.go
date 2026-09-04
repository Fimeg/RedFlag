package scanner

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"os/exec"
	"regexp"
	"strconv"
	"strings"

	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/installer"
)

// DNFScanner scans for DNF/RPM package updates
type DNFScanner struct{}

// NewDNFScanner creates a new DNF scanner
func NewDNFScanner() *DNFScanner {
	return &DNFScanner{}
}

func (s *DNFScanner) Name() string { return "DNF Update Scanner" }

// IsAvailable checks if DNF is available on this system
func (s *DNFScanner) IsAvailable() bool {
	_, err := exec.LookPath("dnf")
	return err == nil
}

// Scan scans for available DNF updates
func (s *DNFScanner) Scan() ([]client.UpdateReportItem, error) {
	runner, err := installer.NewDiscoveryRunner("dnf")
	if err != nil {
		return nil, fmt.Errorf("[ERROR] [agent] [dnf] discovery_runner_create error=%w", err)
	}
	result, err := runner.Run(context.Background(), "check-update")
	if err != nil && (result == nil || result.ExitCode != 100) {
		return nil, fmt.Errorf("failed to run dnf check-update: %w", err)
	}
	if result == nil {
		return nil, fmt.Errorf("failed to run dnf check-update: no result")
	}
	output := []byte(result.Stdout)

	return parseDNFOutput(output)
}

func parseDNFOutput(output []byte) ([]client.UpdateReportItem, error) {
	var updates []client.UpdateReportItem
	scanner := bufio.NewScanner(bytes.NewReader(output))

	// Regex to parse dnf check-update output.
	// Format: pkgname.arch  version  repo  (three columns, verified on dnf5)
	re := regexp.MustCompile(`^([^\s]+)\.([^\s]+)\s+([^\s]+)\s+([^\s]+)$`)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and header/footer
		if line == "" ||
			strings.HasPrefix(line, "Last metadata") ||
			strings.HasPrefix(line, "Dependencies") ||
			strings.HasPrefix(line, "Obsoleting") ||
			strings.Contains(line, "Upgraded") {
			continue
		}

		matches := re.FindStringSubmatch(line)
		if len(matches) < 5 {
			log.Printf("[WARNING] [scanner] [dnf] unparseable_line line=%q", line)
			continue
		}

		packageName := matches[1]
		arch := matches[2]
		version := matches[3]
		repository := matches[4]

		// Get currently installed version via rpm
		currentVersion := getInstalledVersion(packageName)

		// Determine severity based on repository and update type
		severity := determineSeverity(repository, packageName, version)

		update := client.UpdateReportItem{
			PackageType:      "dnf",
			PackageName:      packageName,
			CurrentVersion:   currentVersion,
			AvailableVersion: version,
			Severity:         severity,
			RepositorySource: repository,
			Metadata: map[string]interface{}{
				"architecture": arch,
			},
		}

		updates = append(updates, update)
	}

	return updates, nil
}

// getInstalledVersion gets the currently installed version of a package.
// Packages can have multiple installed instances — kernel keeps several,
// multilib ships i686+x86_64 — and rpm -q emits one record per instance.
// The queryformat newline keeps records from concatenating into an
// unparseable blob; the newest EVR is what threat checks care about.
func getInstalledVersion(packageName string) string {
	cmd := exec.Command("rpm", "-q", "--queryformat", "%{EPOCHNUM}|%{VERSION}|%{RELEASE}\n", packageName)
	output, err := cmd.Output()
	if err != nil {
		return "unknown"
	}
	version := newestInstalledEVR(string(output))
	if version == "" {
		return "unknown"
	}
	return version
}

// newestInstalledEVR picks the newest epoch|version|release record from
// multi-line rpm -q output and formats it. Returns "" on no usable records.
func newestInstalledEVR(output string) string {
	var newest string
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if newest == "" || rawEVRCompare(line, newest) > 0 {
			newest = line
		}
	}
	if newest == "" {
		return ""
	}
	return formatRPMEVR(newest)
}

func formatRPMEVR(raw string) string {
	parts := strings.Split(raw, "|")
	if len(parts) != 3 {
		return strings.TrimSpace(raw)
	}

	epoch := strings.TrimSpace(parts[0])
	version := strings.TrimSpace(parts[1])
	release := strings.TrimSpace(parts[2])
	if version == "" {
		return ""
	}

	evr := version
	if release != "" {
		evr += "-" + release
	}
	if epoch != "" && epoch != "0" && epoch != "(none)" {
		evr = epoch + ":" + evr
	}
	return evr
}

// rawEVRCompare orders two raw "epoch|version|release" records per RPM
// semantics: epoch numerically, then version, then release via rpmvercmp.
// Malformed records fall back to string comparison for a stable order.
func rawEVRCompare(a, b string) int {
	pa, pb := strings.SplitN(a, "|", 3), strings.SplitN(b, "|", 3)
	if len(pa) != 3 || len(pb) != 3 {
		return strings.Compare(a, b)
	}
	ea, _ := strconv.Atoi(strings.TrimSpace(pa[0]))
	eb, _ := strconv.Atoi(strings.TrimSpace(pb[0]))
	if ea != eb {
		if ea > eb {
			return 1
		}
		return -1
	}
	if c := rpmvercmp(pa[1], pb[1]); c != 0 {
		return c
	}
	return rpmvercmp(pa[2], pb[2])
}

// rpmvercmp implements RPM's version comparison: alternating numeric/alpha
// segments split on separators, digits beat alphas, tilde sorts before
// everything (pre-release), caret sorts after end-of-string but before any
// other segment.
func rpmvercmp(a, b string) int {
	if a == b {
		return 0
	}
	isDigit := func(c byte) bool { return c >= '0' && c <= '9' }
	isAlpha := func(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }
	isAlnum := func(c byte) bool { return isDigit(c) || isAlpha(c) }

	i, j := 0, 0
	for i < len(a) || j < len(b) {
		for i < len(a) && !isAlnum(a[i]) && a[i] != '~' && a[i] != '^' {
			i++
		}
		for j < len(b) && !isAlnum(b[j]) && b[j] != '~' && b[j] != '^' {
			j++
		}

		ta, tb := i < len(a) && a[i] == '~', j < len(b) && b[j] == '~'
		if ta || tb {
			if ta && tb {
				i++
				j++
				continue
			}
			if ta {
				return -1
			}
			return 1
		}

		ca, cb := i < len(a) && a[i] == '^', j < len(b) && b[j] == '^'
		if ca || cb {
			if ca && cb {
				i++
				j++
				continue
			}
			if ca {
				if j >= len(b) {
					return 1
				}
				return -1
			}
			if i >= len(a) {
				return -1
			}
			return 1
		}

		if i >= len(a) || j >= len(b) {
			if i < len(a) {
				return 1
			}
			if j < len(b) {
				return -1
			}
			return 0
		}

		if isDigit(a[i]) {
			if !isDigit(b[j]) {
				return 1 // numeric segment beats alpha
			}
			si, sj := i, j
			for i < len(a) && isDigit(a[i]) {
				i++
			}
			for j < len(b) && isDigit(b[j]) {
				j++
			}
			sa, sb := strings.TrimLeft(a[si:i], "0"), strings.TrimLeft(b[sj:j], "0")
			if len(sa) != len(sb) {
				if len(sa) > len(sb) {
					return 1
				}
				return -1
			}
			if c := strings.Compare(sa, sb); c != 0 {
				return c
			}
		} else {
			if isDigit(b[j]) {
				return -1
			}
			si, sj := i, j
			for i < len(a) && isAlpha(a[i]) {
				i++
			}
			for j < len(b) && isAlpha(b[j]) {
				j++
			}
			if c := strings.Compare(a[si:i], b[sj:j]); c != 0 {
				return c
			}
		}
	}
	return 0
}

// determineSeverity determines the severity of an update based on repository and package information
func determineSeverity(repository, packageName, newVersion string) string {
	// Security updates
	if strings.Contains(strings.ToLower(repository), "security") ||
		strings.Contains(strings.ToLower(packageName), "security") ||
		strings.Contains(strings.ToLower(packageName), "selinux") ||
		strings.Contains(strings.ToLower(packageName), "crypto") ||
		strings.Contains(strings.ToLower(packageName), "openssl") ||
		strings.Contains(strings.ToLower(packageName), "gnutls") {
		return "critical"
	}

	// Kernel updates are important
	if strings.Contains(strings.ToLower(packageName), "kernel") {
		return "important"
	}

	// Core system packages
	if strings.Contains(strings.ToLower(packageName), "glibc") ||
		strings.Contains(strings.ToLower(packageName), "systemd") ||
		strings.Contains(strings.ToLower(packageName), "bash") ||
		strings.Contains(strings.ToLower(packageName), "coreutils") {
		return "important"
	}

	// Development tools
	if strings.Contains(strings.ToLower(packageName), "gcc") ||
		strings.Contains(strings.ToLower(packageName), "python") ||
		strings.Contains(strings.ToLower(packageName), "nodejs") ||
		strings.Contains(strings.ToLower(packageName), "java") ||
		strings.Contains(strings.ToLower(packageName), "go") {
		return "moderate"
	}

	// Default severity
	return "low"
}
