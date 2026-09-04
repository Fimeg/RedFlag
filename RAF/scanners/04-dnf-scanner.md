# DNF Scanner

**DNF package manager scanning for Fedora/RHEL-based Linux agents.**

---

## Component Details

| Property | Value |
|----------|-------|
| Method | `dnf check-update --refresh` |
| Platform | Linux (Fedora, RHEL, CentOS) |
| Execution time | ~15 seconds per scan |
| Output format | JSON array of package objects with version info |
| Failure modes | DNF lock held, network timeout, permission denied |

---

## Implementation

**File:** `agent/internal/scanner/dnf.go`

```go
func ScanDNF(ctx context.Context) ([]DNFUpdate, error) {
    // 1. Run dnf check-update --refresh
    cmd := exec.CommandContext(ctx, "dnf", "check-update", "--refresh")
    output, err := cmd.CombinedOutput()
    if err != nil {
        // Check if it's a no-update case (exit code 100)
        if exitErr, ok := err.(*exec.ExitError); ok {
            if exitErr.ExitCode() == 100 {
                return []DNFUpdate{}, nil
            }
        }
        logSecurityEvent("[reliability] [agent] [dnf] dnf check-update failed:", err)
        return nil, err
    }

    // 2. Parse output
    var updates []DNFUpdate
    lines := strings.Split(string(output), "\n")
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if line == "" || strings.HasPrefix(line, "Last metadata expiration check") {
            continue
        }

        // Parse "package-name.old_version.new_version"
        parts := strings.Fields(line)
        if len(parts) < 3 {
            continue
        }

        pkg := parts[0]
        oldVersion := parts[1]
        newVersion := parts[2]

        updates = append(updates, DNFUpdate{
            Package:   pkg,
            OldVersion: oldVersion,
            NewVersion: newVersion,
        })
    }

    return updates, nil
}
```

---

## Integration Points

### 1. Command Dispatch

The DNF scanner is invoked via command execution flow:

```go
// agent/internal/orchestrator/system_scanner.go
case "scan_dnf":
    updates, err := dnf.ScanDNF(ctx)
    if err != nil {
        return err
    }
    report := &SystemEvent{
        AgentID: agentID,
        EventType: EventTypeAgentScan,
        ScanType: "dnf",
        Data: updates,
    }
    return reportSystemEvent(report)
```

### 2. Circuit Breaker

- **Failure threshold:** 5 failures in 60 seconds
- **Open duration:** 300 seconds (5 minutes)
- **Half-open attempts:** 3 consecutive successes to recover

**Cross-references:**
- `core/01-ethos.md` (principle #3: Assume Failure)
- `verification/04-replay-protection.md` (circuit breaker integration)

---

## Systemd Integration

The DNF scanner requires two paths writable under `ProtectSystem=strict`:

- **`/var/log`** — dnf5 writes `/var/log/dnf5.log`
- **`/var/cache`** — dnf5 creates temp files at `/var/cache/libdnf5/`

**Files to update when locking down a new agent:**
- Live systemd unit: `/etc/systemd/system/redflag-agent.service` → add to `ReadWritePaths`
- Installer template: `agent/internal/installer/sudoers.go:CreateSystemdService()` → same change

**Cross-references:**
- `core/01-ethos.md` (principle #1: Errors are History — log writes must succeed)
- `agent/internal/installer/sudoers.go` (systemd service template)

---

## Footer: Assumptions & Connections

**Assumption:** DNF scanning is a periodic operation that runs independently of command execution.

**Connection:** Circuit breaker (`verification/04-replay-protection.md`) prevents DNF scanner from blocking other subsystems.

**Connection:** System events (`flows/04-heartbeat.md`) published to history table for audit trail.

**Connection:** DNF scanner (`scanners/04-dnf-scanner.md`) lives in `agent/internal/scanner/dnf.go`.

---

*Last reviewed: 2026-05-26*
