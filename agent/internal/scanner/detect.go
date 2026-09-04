package scanner

import (
	"os/exec"
)

// DetectAvailable returns the package-manager / Docker scanners that are
// usable on the current host. The check is stateless and side-effect free
// (no config writes, no network) so it is safe to call from both the
// registration path and every poll of the agent loop (ARC-001).
//
// The returned identifiers match the strings the server uses for
// agent_subsystems.subsystem (apt, dnf, winget, windows, docker).
func DetectAvailable() []string {
	scanners := []string{}

	if NewAPTScanner().IsAvailable() {
		scanners = append(scanners, "apt")
	}
	if NewDNFScanner().IsAvailable() {
		scanners = append(scanners, "dnf")
	}
	if NewWingetScanner().IsAvailable() {
		scanners = append(scanners, "winget")
	}
	if NewWindowsUpdateScanner().IsAvailable() {
		scanners = append(scanners, "windows")
	}
	if NewPacmanScanner().IsAvailable() {
		scanners = append(scanners, "pacman")
	}
	// Inline Docker availability check — avoids importing orchestrator (circular).
	if _, err := exec.LookPath("docker"); err == nil {
		scanners = append(scanners, "docker")
	}

	return scanners
}
