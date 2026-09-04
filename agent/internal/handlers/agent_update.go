package handlers

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/acknowledgment"
	"github.com/Fimeg/RedFlag/agent/internal/capability"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/constants"
	"github.com/Fimeg/RedFlag/agent/internal/supplychain"
)

// HandleUpdateAgent handles agent update commands with signature verification
func HandleUpdateAgent(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker, params map[string]interface{}, commandID string) error {
	version, ok := params["version"].(string)
	if !ok {
		return fmt.Errorf("missing version parameter")
	}

	platform, ok := params["platform"].(string)
	if !ok {
		return fmt.Errorf("missing platform parameter")
	}

	downloadURL, ok := params["download_url"].(string)
	if !ok {
		return fmt.Errorf("missing download_url parameter")
	}

	signature, ok := params["signature"].(string)
	if !ok {
		return fmt.Errorf("missing signature parameter")
	}

	checksum, ok := params["checksum"].(string)
	if !ok {
		return fmt.Errorf("missing checksum parameter")
	}

	nonceUUIDStr, ok := params["nonce_uuid"].(string)
	if !ok {
		return fmt.Errorf("missing nonce_uuid parameter")
	}

	nonceTimestampStr, ok := params["nonce_timestamp"].(string)
	if !ok {
		return fmt.Errorf("missing nonce_timestamp parameter")
	}

	nonceSignature, ok := params["nonce_signature"].(string)
	if !ok {
		return fmt.Errorf("missing nonce_signature parameter")
	}

	log.Printf("[INFO] [agent] [update] starting version=%s platform=%s", version, platform)

	// Nonce max-age = 2 × check-in interval so the nonce survives the gap between
	// being queued server-side and being fetched at the agent's next poll cycle.
	nonceMaxAge := time.Duration(cfg.CheckInInterval*2) * time.Second

	log.Printf("[tunturi_ed25519] Validating nonce...")
	if err := validateNonce(nonceUUIDStr, nonceTimestampStr, nonceSignature, nonceMaxAge); err != nil {
		log.Printf("[ERROR] [agent] [security] nonce_validation_failed error=%v", err)
		return fmt.Errorf("[tunturi_ed25519] nonce validation failed: %w", err)
	}
	log.Printf("[INFO] [agent] [security] nonce_validated")

	updateStartTime := time.Now()

	logReport := client.LogReport{
		CommandID:       commandID,
		Action:          "update_agent",
		Result:          "started",
		Stdout:          fmt.Sprintf("Starting agent update to version %s\n", version),
		Stderr:          "",
		ExitCode:        0,
		DurationSeconds: 0,
		Metadata: map[string]string{
			"subsystem_label": "Agent Update",
			"subsystem":       "agent",
			"target_version":  version,
		},
	}

	if err := ReportLogWithAck(apiClient, cfg, ackTracker, logReport); err != nil {
		log.Printf("[ERROR] [agent] [update] report_start_failed error=%v", err)
	}

	log.Printf("[INFO] [agent] [update] download_start url=%s", downloadURL)

	// The /api/v1/downloads/updates/:package_id route is protected by AuthMiddleware
	// — the prior implementation used a raw http.Client.Get and would 401 in
	// production. Use the authenticated client so JWT + X-Machine-ID accompany the
	// request. Relative-URL resolution happens inside DownloadAuthenticatedToFile.
	tempBinaryPath, err := downloadUpdatePackage(apiClient, downloadURL)
	if err != nil {
		return fmt.Errorf("failed to download update package: %w", err)
	}
	defer os.Remove(tempBinaryPath)
	log.Printf("[INFO] [agent] [update] download_complete")

	actualChecksum, err := computeSHA256(tempBinaryPath)
	if err != nil {
		return fmt.Errorf("failed to compute checksum: %w", err)
	}

	if actualChecksum != checksum {
		return fmt.Errorf("checksum mismatch: expected %s, got %s", checksum, actualChecksum)
	}
	log.Printf("[INFO] [agent] [upgrade] checksum_verified checksum=%s", actualChecksum)

	log.Printf("[tunturi_ed25519] Step 3: Verifying Ed25519 signature...")
	if err := verifyBinarySignature(tempBinaryPath, signature); err != nil {
		return fmt.Errorf("[tunturi_ed25519] signature verification failed: %w", err)
	}
	log.Printf("[INFO] [agent] [upgrade] ed25519_signature_verified")

	// Linux: the privileged binary swap is performed by the root helper, not the
	// agent's own sudo. The server signed an agent-self capability token over this
	// binary's hash; the helper re-verifies and installs. Other platforms keep the
	// legacy direct path below until a platform executor exists.
	if runtime.GOOS == "linux" {
		// Download the helper binary if the server provided a URL.
		var helperBinaryPath string
		if helperURL, ok := params["helper_download_url"].(string); ok && helperURL != "" {
			var dlErr error
			helperBinaryPath, dlErr = downloadUpdatePackage(apiClient, helperURL)
			if dlErr != nil {
				log.Printf("[WARNING] [agent] [upgrade] helper_download_failed error=%v — agent will update, helper stays as-is", dlErr)
			} else {
				defer os.Remove(helperBinaryPath)
				// Verify helper checksum if provided
				if helperChecksum, ok := params["helper_checksum"].(string); ok && helperChecksum != "" {
					actual, err := computeSHA256(helperBinaryPath)
					if err != nil {
						log.Printf("[WARNING] [agent] [upgrade] helper_checksum_compute_failed error=%v", err)
						helperBinaryPath = ""
					} else if actual != helperChecksum {
						log.Printf("[WARNING] [agent] [upgrade] helper_checksum_mismatch expected=%s got=%s", helperChecksum, actual)
						helperBinaryPath = ""
					}
				}
			}
		}
		return installAgentViaHelper(tempBinaryPath, helperBinaryPath, params, commandID, version, updateStartTime)
	}

	currentBinaryPath, err := getCurrentBinaryPath()
	if err != nil {
		return fmt.Errorf("failed to determine current binary path: %w", err)
	}

	backupPath := currentBinaryPath + ".bak"
	var updateSuccess bool = false

	if err := createBackup(currentBinaryPath, backupPath); err != nil {
		log.Printf("[ERROR] [agent] [upgrade] backup_failed error=%v", err)
	} else {
		defer func() {
			if updateSuccess {
				// Binary swap and restart dispatch succeeded. systemd's SIGTERM
				// is imminent and may abort this defer before completion. .bak
				// stays on disk: the new binary cleans it up after first
				// successful check-in attestation (cleanupPostUpdateBackup),
				// or the operator restores from it if the new binary fails to
				// start. Completion is owned by server-side
				// timeout.reconcileAgentUpdates.
				return
			}
			log.Printf("[INFO] [agent] [upgrade] rollback_start reason=pre_restart_error")
			if restoreErr := restoreFromBackup(backupPath, currentBinaryPath); restoreErr != nil {
				log.Printf("[ERROR] [agent] [upgrade] rollback_failed error=%v", restoreErr)
			} else {
				log.Printf("[INFO] [agent] [upgrade] rollback_success")
			}
		}()
	}

	log.Printf("[INFO] [agent] [upgrade] install_start")
	if err := installNewBinary(tempBinaryPath, currentBinaryPath); err != nil {
		return fmt.Errorf("failed to install new binary: %w", err)
	}

	// Past the point of no return on disk. The watchdog-and-final-log-report
	// pattern used to live here; it could not survive systemd's SIGTERM and
	// has been removed. Server-side reconcileAgentUpdates closes the command
	// when the new binary reports its version on next check-in.
	updateSuccess = true
	log.Printf("[INFO] [agent] [upgrade] install_complete duration_seconds=%d", int(time.Since(updateStartTime).Seconds()))

	// Binary is committed on disk — even if the restart dispatch below fails, the
	// next boot runs the new binary, so the marker stays valid either way.
	if err := WriteUpgradeAttestation(commandID, version); err != nil {
		log.Printf("[WARNING] [agent] [upgrade] attestation_marker_write_failed error=%v — upgrade proceeds unattested", err)
	}

	log.Printf("[INFO] [agent] [upgrade] restart_dispatch")
	if err := restartAgentService(); err != nil {
		// Binary is swapped on disk, but restart failed. The current process
		// keeps running on the OLD code; .bak remains for manual restore on
		// next manual boot.
		return fmt.Errorf("failed to restart agent: %w", err)
	}

	return nil
}

// installAgentViaHelper performs the Linux agent self-upgrade through the
// privileged helper. The agent stages the verified binary on the real filesystem
// (the helper's transient-unit mount namespace cannot see the agent's PrivateTmp)
// and hands the helper the server-signed agent-self token. The helper verifies the
// token signature and the staged binary's hash, backs up, installs, chmods, and
// restarts the agent — the agent holds no sudo for cp/chmod/systemctl.
func installAgentViaHelper(tempBinaryPath, helperBinaryPath string, params map[string]interface{}, commandID, targetVersion string, startTime time.Time) error {
	// Must match the helper's DEFAULT_AGENT_UPGRADE_SOURCE.
	upgradeStagingPath := constants.GetAgentStagingPath("pending-upgrade.bin")
	if err := copyFile(tempBinaryPath, upgradeStagingPath); err != nil {
		return fmt.Errorf("failed to stage new binary for helper: %w", err)
	}

	// Stage the helper binary if we downloaded one. The helper will
	// self-update from this staged copy before installing the new agent.
	helperStagingPath := constants.GetAgentStagingPath("pending-helper.bin")
	hasHelper := false
	if helperBinaryPath != "" {
		if err := copyFile(helperBinaryPath, helperStagingPath); err != nil {
			log.Printf("[WARNING] [agent] [upgrade] helper_stage_failed error=%v — agent will update, helper stays as-is", err)
		} else {
			hasHelper = true
		}
	}

	// Clean up staged binaries on failure. On success the helper restarts
	// us (SIGTERM) before we reach this defer, so the files are already
	// replaced and this is a harmless no-op. The attestation marker is also
	// dropped on failure — no restart happened, so the next boot must not
	// attest against this command.
	success := false
	defer func() {
		if !success {
			for _, p := range []string{upgradeStagingPath, helperStagingPath} {
				if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
					log.Printf("[WARNING] [agent] [upgrade] staging_cleanup_failed path=%s error=%v", p, err)
				}
			}
			ClearUpgradeAttestation()
		}
	}()

	tokenJSON, ok := params["capability_token"].(string)
	if !ok || tokenJSON == "" {
		return fmt.Errorf("missing capability_token — server did not authorize the helper swap")
	}
	var token capability.Token
	if err := json.Unmarshal([]byte(tokenJSON), &token); err != nil {
		return fmt.Errorf("failed to parse capability_token: %w", err)
	}

	// The marker must be on disk before the helper runs: on success the helper
	// restarts this process mid-Execute, and the post-upgrade healthcheck in the
	// new binary (RunUpgradeAttestation) is what verifies the swap actually took.
	if err := WriteUpgradeAttestation(commandID, targetVersion); err != nil {
		log.Printf("[WARNING] [agent] [upgrade] attestation_marker_write_failed error=%v — upgrade proceeds unattested", err)
	}

	log.Printf("[INFO] [agent] [upgrade] invoking_helper token_id=%s has_helper=%v", token.TokenID, hasHelper)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Build args for the helper. --helper-file tells the helper where the
	// staged helper binary lives so it can self-update before installing
	// the new agent.
	var helperArgs []string
	if hasHelper {
		helperArgs = []string{"--helper-file", helperStagingPath}
	}

	result, err := supplychain.NewExecutor("").Execute(ctx, &token, helperArgs...)
	if err != nil {
		// On success the helper restarts this agent, which can SIGTERM us before
		// Execute returns. The new binary reporting its version on next check-in is
		// the authoritative success signal (server reconcileAgentUpdates closes the
		// command), so a transport error here is not necessarily a failed upgrade.
		return fmt.Errorf("helper invocation failed (or agent restarted mid-swap): %w", err)
	}
	if result.Decision != "executed" {
		return fmt.Errorf("helper refused agent upgrade: decision=%s reason=%s exit=%d",
			result.Decision, result.Reason, result.ExitCode)
	}
	success = true
	log.Printf("[INFO] [agent] [upgrade] helper_swap_complete token_id=%s duration_seconds=%d",
		token.TokenID, int(time.Since(startTime).Seconds()))
	return nil
}

// copyFile copies src to dst (truncating dst). No privilege required — used to
// place the verified binary on the real filesystem where the root helper can read
// it (the agent's own /var/lib/redflag/agent is writable and outside PrivateTmp).
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// --- Helper functions ---

// downloadUpdatePackage streams the new agent binary into a temp file using the
// agent's authenticated client. Caller owns the returned path (use defer os.Remove).
// The 500MB cap mirrors the prior implementation; an oversize response returns an
// error rather than a silently-truncated binary that would fail signature verify.
func downloadUpdatePackage(apiClient *client.Client, downloadURL string) (string, error) {
	tempFile, err := os.CreateTemp("", "redflag-update-*.bin")
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	tempPath := tempFile.Name()
	tempFile.Close() // DownloadAuthenticatedToFile re-opens via os.Create

	const maxBinarySize = 500 * 1024 * 1024
	if _, err := apiClient.DownloadAuthenticatedToFile(downloadURL, tempPath, maxBinarySize); err != nil {
		os.Remove(tempPath)
		return "", fmt.Errorf("failed to download: %w", err)
	}
	return tempPath, nil
}

func computeSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to compute hash: %w", err)
	}

	return hex.EncodeToString(hash.Sum(nil)), nil
}

func getCurrentBinaryPath() (string, error) {
	execPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("failed to get executable path: %w", err)
	}
	return execPath, nil
}

// No post-update backup sweep exists by design. On Linux the helper owns the
// binary swap and writes <binary>.bak as root into the root-owned install dir
// (helper/src/main.rs install_staged_agent_binary). The agent runs unprivileged
// and its only sudo grant is the systemd-run helper invocation — it cannot, and
// must not, remove that file directly. Each upgrade's fs::copy truncates .bak in
// place, so it is always a single slot holding exactly the version before the one
// now running: the correct rollback target, maintained without the agent touching
// it. Discarding it after first check-in (the prior behavior) only threw away
// rollback while the new binary was still unproven.

// createBackup copies the current binary to dst as the rollback slot. Pure file
// I/O, no privilege escalation: this runs only on non-Linux platforms (Windows,
// macOS), where the agent process owns its own install directory. Linux never
// reaches here — the privileged helper owns the swap and writes .bak itself.
func createBackup(src, dst string) error {
	if err := copyFile(src, dst); err != nil {
		return fmt.Errorf("failed to create backup: %w", err)
	}
	if err := os.Chmod(dst, 0o755); err != nil {
		return fmt.Errorf("failed to set backup permissions: %w", err)
	}
	return nil
}

func restoreFromBackup(backup, target string) error {
	// The live target may be a running, locked binary. On Windows it can't be
	// overwritten in place but can be renamed out of the way first; on POSIX it
	// can be unlinked while the old image keeps running off its open inode.
	if runtime.GOOS == "windows" {
		aside := target + ".old"
		_ = os.Remove(aside)
		_ = os.Rename(target, aside)
	} else if _, err := os.Stat(target); err == nil {
		if err := os.Remove(target); err != nil {
			return fmt.Errorf("failed to remove current binary: %w", err)
		}
	}
	return createBackup(backup, target)
}

// installNewBinary stages the verified binary beside the current one and swaps it
// into place with pure file I/O — no sudo, no helper. Reached only on non-Linux
// platforms (Linux swaps through the helper). The agent service account has write
// access to its own install directory on both Windows and macOS.
func installNewBinary(src, dst string) error {
	staged := dst + ".new"
	if err := copyFile(src, staged); err != nil {
		return fmt.Errorf("failed to stage new binary: %w", err)
	}
	if err := os.Chmod(staged, 0o755); err != nil {
		os.Remove(staged)
		return fmt.Errorf("failed to set binary permissions: %w", err)
	}

	if runtime.GOOS == "windows" {
		// A running .exe can't be overwritten, but Windows permits renaming it.
		// Move the live binary aside, then move the staged binary into the real
		// path. The aside copy is locked until this process exits; it self-clears
		// on the next upgrade (the os.Remove above) — a single stale slot at rest.
		aside := dst + ".old"
		_ = os.Remove(aside) // rename won't clobber an existing target on Windows
		if err := os.Rename(dst, aside); err != nil {
			os.Remove(staged)
			return fmt.Errorf("failed to move running binary aside: %w", err)
		}
		if err := os.Rename(staged, dst); err != nil {
			os.Rename(aside, dst) // undo: restore the original
			return fmt.Errorf("failed to install new binary: %w", err)
		}
		return nil
	}

	// POSIX: atomically replace the directory entry. The running process keeps its
	// already-open image until it restarts.
	if err := os.Rename(staged, dst); err != nil {
		os.Remove(staged)
		return fmt.Errorf("failed to install new binary: %w", err)
	}
	return nil
}

// restartAgentService cycles the agent's own service after the on-disk binary swap.
// Only reached on non-Linux platforms: Linux self-upgrades run end to end through
// the privileged helper (installAgentViaHelper), which restarts the unit itself.
func restartAgentService() error {
	switch runtime.GOOS {
	case "windows":
		return dispatchWindowsRestart("RedFlagAgent")
	default:
		return fmt.Errorf("self-restart not supported on %s; restart the service manually", runtime.GOOS)
	}
}

// --- Signature verification ---

func verifyBinarySignature(binaryPath, signatureHex string) error {
	publicKey, err := getServerPublicKey()
	if err != nil {
		return fmt.Errorf("failed to get server public key: %w", err)
	}

	content, err := os.ReadFile(binaryPath)
	if err != nil {
		return fmt.Errorf("failed to read binary: %w", err)
	}

	signatureBytes, err := hex.DecodeString(signatureHex)
	if err != nil {
		return fmt.Errorf("failed to decode signature: %w", err)
	}

	if len(signatureBytes) != ed25519.SignatureSize {
		return fmt.Errorf("invalid signature length: expected %d bytes, got %d", ed25519.SignatureSize, len(signatureBytes))
	}

	valid := ed25519.Verify(ed25519.PublicKey(publicKey), content, signatureBytes)
	if !valid {
		return fmt.Errorf("signature verification failed: invalid signature")
	}

	return nil
}

func getServerPublicKey() ([]byte, error) {
	publicKey, err := loadCachedPublicKeyDirect()
	if err != nil {
		return nil, fmt.Errorf("failed to load server public key: %w (hint: key is fetched at agent startup)", err)
	}

	return publicKey, nil
}

func loadCachedPublicKeyDirect() ([]byte, error) {
	keyPath := constants.GetServerPublicKeyPath()

	data, err := os.ReadFile(keyPath)
	if err != nil {
		return nil, fmt.Errorf("public key not found at %s: %w", keyPath, err)
	}

	if len(data) != 32 {
		return nil, fmt.Errorf("invalid public key size: expected 32 bytes, got %d", len(data))
	}

	return data, nil
}

func validateNonce(nonceUUIDStr, nonceTimestampStr, nonceSignature string, maxAge time.Duration) error {
	nonceTimestamp, err := time.Parse(time.RFC3339, nonceTimestampStr)
	if err != nil {
		return fmt.Errorf("invalid nonce timestamp format: %w", err)
	}

	age := time.Since(nonceTimestamp)
	if age > maxAge {
		return fmt.Errorf("nonce expired: age %v > %v", age, maxAge)
	}

	if age < 0 {
		return fmt.Errorf("nonce timestamp in the future: %v", nonceTimestamp)
	}

	publicKey, err := getServerPublicKey()
	if err != nil {
		return fmt.Errorf("failed to get server public key: %w", err)
	}

	nonceData := fmt.Sprintf("%s:%d", nonceUUIDStr, nonceTimestamp.Unix())

	signatureBytes, err := hex.DecodeString(nonceSignature)
	if err != nil {
		return fmt.Errorf("invalid nonce signature format: %w", err)
	}

	if len(signatureBytes) != ed25519.SignatureSize {
		return fmt.Errorf("invalid nonce signature length: expected %d bytes, got %d",
			ed25519.SignatureSize, len(signatureBytes))
	}

	valid := ed25519.Verify(ed25519.PublicKey(publicKey), []byte(nonceData), signatureBytes)
	if !valid {
		return fmt.Errorf("invalid nonce signature")
	}

	return nil
}
