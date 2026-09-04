package services

import "fmt"

// Event renderer — single source of operator-facing verbiage. Dashboards must
// never compose strings from raw enum names like `verify_command` or stitch
// `${result} System event: ${action}`. When the UI needs to display an
// event, it reads the `narrative` field that handlers populate using these
// functions.
//
// To add a new event narrative: extend the switch with the (type, subtype) or
// (action, result) pair you want to render. The default branch falls back to
// the existing free-form message or — failing that — the raw enum tokens, so
// nothing breaks silently when a renderer entry is missing.

// RenderSystemEvent returns a single-line operator narrative for a
// system_events row. eventType + "." + eventSubtype form the key; metadata
// supplies template values. message is the free-form field the producer
// already wrote (the reconciler, for instance, writes a thorough message);
// when present and the (type, subtype) has no dedicated renderer, message
// passes through unchanged.
func RenderSystemEvent(eventType, eventSubtype, message string, metadata map[string]interface{}) string {
	switch eventType + "." + eventSubtype {
	case "agent_update.initiated":
		oldV := metaString(metadata, "old_version")
		newV := metaString(metadata, "new_version")
		platform := metaString(metadata, "platform")
		if platform != "" {
			return fmt.Sprintf("Update queued: %s → %s (%s)", orUnknown(oldV), orUnknown(newV), platform)
		}
		return fmt.Sprintf("Update queued: %s → %s", orUnknown(oldV), orUnknown(newV))

	case "agent_update.succeeded":
		newV := metaString(metadata, "new_version")
		return fmt.Sprintf("Update succeeded — agent now running %s", orUnknown(newV))

	case "agent_update.timed_out":
		// Reconciler writes a complete operator-facing message including the
		// remediation hint. Pass it through.
		if message != "" {
			return message
		}
		return "Agent update timed out without reporting a new version"

	case "agent_update.failed":
		if reason := metaString(metadata, "failure_reason"); reason != "" {
			return "Update failed: " + reason
		}
		return "Update failed"

	case "command_failed.signature_invalid":
		return "Agent rejected a command because the signature did not validate against the server's signing key"

	case "agent_registration.success":
		return "Agent registered with the server"

	case "agent_registration.failed":
		if reason := metaString(metadata, "reason"); reason != "" {
			return "Agent registration failed: " + reason
		}
		return "Agent registration failed"
	}

	if message != "" {
		return message
	}
	return fmt.Sprintf("%s · %s", eventType, eventSubtype)
}

// RenderUpdateLog returns a single-line operator narrative for an update_logs
// row (agent-reported command outcome). action+result form the key; stderr
// is used only when no dedicated renderer matches and we want at least some
// detail in the fallback.
func RenderUpdateLog(action, result, stderr string) string {
	switch action {
	case "verify_command":
		if result == "failed" {
			return "Agent rejected command — signature did not validate against the server's signing key"
		}
		if result == "success" {
			return "Agent verified command signature"
		}

	case "update_agent":
		switch result {
		case "started":
			return "Agent binary update initiated"
		case "success":
			return "Agent applied binary update"
		case "failed":
			return "Agent failed to apply binary update"
		case "dry_run_success":
			return "Agent update dry run succeeded"
		case "partial":
			return "Agent update completed with non-fatal errors"
		}

	case "install_updates", "install":
		switch result {
		case "success":
			return "Package install completed"
		case "failed", "failure":
			return "Package install failed"
		case "partial_failure", "partial":
			return "Package install partially failed — some packages did not install"
		}

	case "dry_run_update":
		switch result {
		case "success":
			return "Dry run succeeded — dependencies confirmed installable"
		case "failed", "dry_run_failed":
			return "Dry run failed — dependency check did not pass"
		}

	case "confirm_dependencies":
		switch result {
		case "success":
			return "Agent confirmed dependencies for the pending install"
		case "failed":
			return "Agent could not confirm dependencies for the pending install"
		}

	case "rollback_update":
		switch result {
		case "success":
			return "Rollback completed — previous version restored"
		case "failed":
			return "Rollback failed — manual recovery required"
		}

	case "reboot":
		switch result {
		case "success":
			return "Agent host rebooted"
		case "failed":
			return "Agent host reboot failed"
		}

	case "enable_heartbeat":
		if result == "success" {
			return "Agent entered rapid-polling mode"
		}

	case "disable_heartbeat":
		if result == "success" {
			return "Agent exited rapid-polling mode"
		}
	}

	if stderr != "" {
		return fmt.Sprintf("%s · %s · %s", action, result, stderr)
	}
	return fmt.Sprintf("%s · %s", action, result)
}

func metaString(m map[string]interface{}, key string) string {
	if m == nil {
		return ""
	}
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func orUnknown(s string) string {
	if s == "" {
		return "unknown"
	}
	return s
}
