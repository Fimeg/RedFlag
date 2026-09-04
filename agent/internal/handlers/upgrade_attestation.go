package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/acknowledgment"
	"github.com/Fimeg/RedFlag/agent/internal/client"
	"github.com/Fimeg/RedFlag/agent/internal/config"
	"github.com/Fimeg/RedFlag/agent/internal/constants"
	"github.com/Fimeg/RedFlag/agent/internal/version"
)

// UpgradeAttestation is the marker the old binary writes immediately before the
// self-upgrade restart. The new binary reads it at startup and attests whether
// the process now running is actually the version the swap installed. Without
// this, a swap that silently failed (or was rolled back from .bak) leaves the
// old binary checking in normally and the server waiting out the full
// stuck-update timeout before anyone notices.
type UpgradeAttestation struct {
	CommandID   string    `json:"command_id"`
	FromVersion string    `json:"from_version"`
	ToVersion   string    `json:"to_version"`
	InitiatedAt time.Time `json:"initiated_at"`
}

// attestationMaxAge bounds marker retries. Past this the server's
// reconcileAgentUpdates sweep has long since timed the command out, so a
// report would only 409; drop the marker instead of retrying forever.
const attestationMaxAge = 24 * time.Hour

func upgradeAttestationPath() string {
	return filepath.Join(constants.GetAgentStateDir(), "upgrade-attestation.json")
}

// WriteUpgradeAttestation persists the pre-restart marker. Called by the
// upgrade handler after the binary swap is committed (or handed to the helper)
// and before the service restart.
func WriteUpgradeAttestation(commandID, toVersion string) error {
	att := UpgradeAttestation{
		CommandID:   commandID,
		FromVersion: version.Version,
		ToVersion:   toVersion,
		InitiatedAt: time.Now().UTC(),
	}
	data, err := json.Marshal(att)
	if err != nil {
		return fmt.Errorf("marshal upgrade attestation: %w", err)
	}
	if err := os.WriteFile(upgradeAttestationPath(), data, 0o600); err != nil {
		return fmt.Errorf("write upgrade attestation: %w", err)
	}
	return nil
}

// ClearUpgradeAttestation removes the marker. Used on upgrade paths that fail
// before the restart is dispatched — the next boot is not a post-upgrade boot.
func ClearUpgradeAttestation() {
	if err := os.Remove(upgradeAttestationPath()); err != nil && !os.IsNotExist(err) {
		log.Printf("[WARNING] [agent] [upgrade] attestation_marker_remove_failed error=%v", err)
	}
}

// RunUpgradeAttestation is the post-upgrade healthcheck, called once at agent
// startup before the polling loop. If a marker exists, the running version is
// checked against the upgrade target:
//
//   - running >= target: the swap worked. Local log only — success closure is
//     deliberately owned by the server (version check-in confirm in
//     ReportMetrics + the reconcileAgentUpdates sweep), not this report.
//   - running < target: the swap failed or was rolled back. Report a failed
//     update_agent log under the original command_id — the server marks the
//     command failed, clears is_updating immediately, and journals a system
//     event, instead of the operator waiting out the stuck-update timeout.
func RunUpgradeAttestation(apiClient *client.Client, cfg *config.Config, ackTracker *acknowledgment.Tracker) {
	data, err := os.ReadFile(upgradeAttestationPath())
	if err != nil {
		if !os.IsNotExist(err) {
			log.Printf("[WARNING] [agent] [upgrade] attestation_marker_read_failed error=%v", err)
		}
		return
	}
	var att UpgradeAttestation
	if err := json.Unmarshal(data, &att); err != nil {
		log.Printf("[WARNING] [agent] [upgrade] attestation_marker_corrupt error=%v", err)
		ClearUpgradeAttestation()
		return
	}

	if versionAtLeast(version.Version, att.ToVersion) {
		log.Printf("[INFO] [agent] [upgrade] post_upgrade_attestation_ok running=%s target=%s command_id=%s",
			version.Version, att.ToVersion, att.CommandID)
		ClearUpgradeAttestation()
		return
	}

	log.Printf("[CRITICAL] [agent] [upgrade] post_upgrade_attestation_failed running=%s target=%s from=%s command_id=%s",
		version.Version, att.ToVersion, att.FromVersion, att.CommandID)

	expired := time.Since(att.InitiatedAt) > attestationMaxAge

	report := client.LogReport{
		CommandID: att.CommandID,
		Action:    "update_agent",
		Result:    "failed",
		Stderr: fmt.Sprintf(
			"post-upgrade attestation failed: running version %s, expected %s (was %s before the swap) — binary swap failed or was rolled back",
			version.Version, att.ToVersion, att.FromVersion),
		ExitCode: 1,
		Metadata: map[string]string{
			"subsystem_label": "Agent Update",
			"subsystem":       "agent",
			"target_version":  att.ToVersion,
			"running_version": version.Version,
			"attested_at":     time.Now().UTC().Format(time.RFC3339),
		},
	}
	if err := ReportLogWithAck(apiClient, cfg, ackTracker, report); err != nil {
		// 409 = the command is already finalized server-side (timeout sweep or an
		// earlier report won the race) — the marker has nothing left to say.
		if strings.Contains(err.Error(), "409") {
			log.Printf("[INFO] [agent] [upgrade] attestation_already_finalized command_id=%s", att.CommandID)
			ClearUpgradeAttestation()
			return
		}
		if expired {
			log.Printf("[ERROR] [agent] [upgrade] attestation_report_failed error=%v — marker expired (>%v), dropping", err, attestationMaxAge)
			ClearUpgradeAttestation()
			return
		}
		log.Printf("[ERROR] [agent] [upgrade] attestation_report_failed error=%v — marker retained for retry on next start", err)
		return
	}
	ClearUpgradeAttestation()
}

// versionAtLeast reports whether running >= target, comparing dotted numeric
// versions (leading "v" tolerated). Mirrors the server's IsNewerOrEqualVersion
// check-in confirm: a running version past the target still proves the swap
// took. Non-numeric segments fall back to string comparison.
func versionAtLeast(running, target string) bool {
	r := strings.Split(strings.TrimPrefix(running, "v"), ".")
	t := strings.Split(strings.TrimPrefix(target, "v"), ".")
	for i := 0; i < len(r) || i < len(t); i++ {
		var rs, ts string
		if i < len(r) {
			rs = r[i]
		}
		if i < len(t) {
			ts = t[i]
		}
		rn, rErr := strconv.Atoi(rs)
		tn, tErr := strconv.Atoi(ts)
		if rErr != nil || tErr != nil {
			if rs == ts {
				continue
			}
			return rs > ts
		}
		if rn != tn {
			return rn > tn
		}
	}
	return true
}
