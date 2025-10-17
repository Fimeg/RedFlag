package installer

// InstallResult represents the result of a package installation attempt
type InstallResult struct {
	Success          bool     `json:"success"`
	ErrorMessage      string    `json:"error_message,omitempty"`
	Stdout           string    `json:"stdout,omitempty"`
	Stderr           string    `json:"stderr,omitempty"`
	ExitCode         int       `json:"exit_code"`
	DurationSeconds  int       `json:"duration_seconds"`
	Action           string    `json:"action,omitempty"`         // "install", "upgrade", etc.
	PackagesInstalled []string  `json:"packages_installed,omitempty"`
	ContainersUpdated []string  `json:"containers_updated,omitempty"`
	Dependencies      []string  `json:"dependencies,omitempty"`      // List of dependency packages found during dry run
	IsDryRun         bool      `json:"is_dry_run"`                  // Whether this is a dry run result
}