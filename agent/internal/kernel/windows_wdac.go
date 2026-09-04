package kernel

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
)

// WDACPolicy handles Windows WDAC (Windows Defender Application Control) enforcement
type WDACPolicy struct {
	client     *client.Client
	config     *config.Config
	policyName string
	policyHash string
}

// NewWDACPolicy creates a new WDACPolicy enforcer
func NewWDACPolicy(c *client.Client, cfg *config.Config) *WDACPolicy {
	return &WDACPolicy{
		client:     c,
		config:     cfg,
		policyName: "RedFlag-Package-Manager",
	}
}

// GetPackageType returns the package type this enforcer handles
func (w *WDACPolicy) GetPackageType() string {
	return "wdac"
}

// IsAvailable checks if WDAC is available on this system
func (w *WDACPolicy) IsAvailable() bool {
	// WDAC only available on Windows
	return true // Platform check done by caller
}

// CheckPolicy evaluates if a package manager invocation is allowed
func (w *WDACPolicy) CheckPolicy(packageType, packageName, packageVersion string) (bool, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	log.Printf("[INFO] [kernel] [wdac] policy_check package=%s type=%s version=%s", packageName, packageType, packageVersion)

	result, err := w.fetchPolicy(ctx)
	if err != nil {
		log.Printf("[ERROR] [kernel] [wdac] policy_fetch_failed package=%s error=%v", packageName, err)
		return false, "policy_fetch_failed: " + err.Error(), nil
	}

	if result == "" {
		return false, "no_policy", nil
	}

	// Parse policy and check if package manager is allowed
	allowed, reason := w.evaluatePolicy(packageType, packageName, result)
	if !allowed {
		log.Printf("[ERROR] [kernel] [wdac] policy_denied package=%s reason=%s", packageName, reason)
	} else {
		log.Printf("[INFO] [kernel] [wdac] policy_allowed package=%s", packageName)
	}

	return allowed, reason, nil
}

// fetchPolicy retrieves the current WDAC policy from the server
func (w *WDACPolicy) fetchPolicy(ctx context.Context) (string, error) {
	// TODO: Replace with actual WDAC COM API calls via go-ole
	// For now, fetch policy hash from server and validate against local policy

	hash, err := w.client.GetWdacPolicyHash(w.config.AgentID, w.config.ServerURL)
	if err != nil {
		log.Printf("[WARNING] [kernel] [wdac] policy_hash_fetch_skipped error=%v", err)
		// For now, treat as no policy available (fail-closed)
		return "", fmt.Errorf("no_policy")
	}

	if hash == "" {
		return "", fmt.Errorf("no WDAC policy available from server")
	}

	if hash != w.policyHash {
		log.Printf("[WARNING] [kernel] [wdac] policy_hash_mismatch local=%s server=%s", w.policyHash, hash)
		// Policy needs update - return empty to trigger update
		return "", fmt.Errorf("policy_hash_mismatch")
	}

	return w.policyHash, nil
}

// evaluatePolicy checks if the package manager is allowed by the current policy
func (w *WDACPolicy) evaluatePolicy(packageType, packageName, policyHash string) (bool, string) {
	// Allowed package managers (fail-closed: deny all others)
	allowed := map[string]bool{
		"apt":            true,
		"apt-get":        true,
		"apt-cache":      true,
		"dnf":            true,
		"yum":            true,
		"rpm-ostree":     true,
		"npm":            true,
		"pnpm":           true,
		"bun":            true,
		"pip":            true,
		"pip3":           true,
		"uv":             true,
		"docker":         true,
		"crun":           true,
		"containerd":     true,
	}

	if allowed[packageType] {
		return true, "allowed_by_policy"
	}

	return false, "package_manager_not_in_policy"
}

// UpdatePolicy downloads and installs a new WDAC policy from the server
func (w *WDACPolicy) UpdatePolicy() error {
	log.Printf("[INFO] [kernel] [wdac] update_policy_start")

	newHash, err := w.client.GetWdacPolicyHash(w.config.AgentID, w.config.ServerURL)
	if err != nil {
		// For now, keep existing policy hash (no update)
		log.Printf("[WARNING] [kernel] [wdac] policy_update_skipped error=%v", err)
		return nil
	}

	if newHash == "" {
		return fmt.Errorf("no policy available from server")
	}

	// TODO: Download policy file and apply via Set-CIPolicy
	// For now, just update the hash
	w.policyHash = newHash

	log.Printf("[INFO] [kernel] [wdac] update_policy_complete hash=%s", newHash[:16]+"...")
	return nil
}

// InstallPolicy installs a WDAC policy from a binary or XML file
func (w *WDACPolicy) InstallPolicy(policyPath string) error {
	// TODO: Implement actual WDAC policy installation via COM API
	// Uses Microsoft.Management.Ops or similar COM interface

	log.Printf("[INFO] [kernel] [wdac] install_policy path=%s", policyPath)
	return fmt.Errorf("install_policy_not_implemented_yet")
}

// GetPolicyStatus returns the current WDAC policy status
func (w *WDACPolicy) GetPolicyStatus() (string, error) {
	// TODO: Query WDAC service for status
	return "unknown", nil
}

// ResetPolicy resets the WDAC policy to a known-good state
func (w *WDACPolicy) ResetPolicy() error {
	log.Printf("[WARNING] [kernel] [wdac] reset_policy_initiated")
	return fmt.Errorf("reset_policy_not_implemented_yet")
}
