# APT Scanner

**APT package manager scanning for Debian/Ubuntu-based Linux agents.**

---

## Component Details

| Property | Value |
|----------|-------|
| Method | `apt list --upgradable -o APT::Get::List-Cleanup=false` |
| Platform | Linux (Debian, Ubuntu) |
| Execution time | ~10 seconds per scan |
| Output format | JSON array of package objects with version info |
| Failure modes | APT lock held, network timeout, permission denied |

---

## Implementation

**File:** `agent/internal/scanner/apt.go`

```go
func ScanAPT(ctx context.Context) ([]APTUpdate, error) {
    // 1. Run apt list --upgradable
    cmd := exec.CommandContext(ctx, "bash", "-c",
        "apt list --upgradable -o APT::Get::List-Cleanup=false 2>&1")
    output, err := cmd.CombinedOutput()
    if err != nil {
        logSecurityEvent("[reliability] [agent] [apt] apt list failed:", err)
        return nil, err
    }

    // 2. Parse output (format: "package/old_version -> new_version")
    var updates []APTUpdate
    lines := strings.Split(string(output), "\n")
    for _, line := range lines {
        line = strings.TrimSpace(line)
        if line == "" || strings.HasPrefix(line, "Listing...") {
            continue
        }

        // Parse "package/old_version -> new_version"
        parts := strings.Split(line, "->")
        if len(parts) != 2 {
            continue
        }

        pkgInfo := strings.Split(strings.TrimSpace(parts[0]), "/")
        if len(pkgInfo) != 2 {
            continue
        }

        updates = append(updates, APTUpdate{
            Package:   pkgInfo[0],
            OldVersion: pkgInfo[1],
            NewVersion: strings.TrimSpace(parts[1]),
        })
    }

    return updates, nil
}
```

---

## Integration Points

### 1. Command Dispatch

The APT scanner is invoked via command execution flow:

```go
// agent/internal/orchestrator/system_scanner.go
case "scan_apt":
    updates, err := apt.ScanAPT(ctx)
    if err != nil {
        return err
    }
    report := &SystemEvent{
        AgentID: agentID,
        EventType: EventTypeAgentScan,
        ScanType: "apt",
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

## Data Flow

```
Agent Poll → Server creates "scan_apt" command
    ↓
Agent receives command
    ↓
ScanAPT() runs apt list --upgradable
    ↓
Parse output and extract package/version info
    ↓
Return updates array
    ↓
Agent reports scan results to server
    ↓
Server displays updates in dashboard
```

---

## Footer: Assumptions & Connections

**Assumption:** APT scanning is a periodic operation that runs independently of command execution.

**Connection:** Circuit breaker (`verification/04-replay-protection.md`) prevents APT scanner from blocking other subsystems.

**Connection:** System events (`flows/04-heartbeat.md`) published to history table for audit trail.

**Connection:** APT scanner (`scanners/03-apt-scanner.md`) lives in `agent/internal/scanner/apt.go`.

---

*Last reviewed: 2026-05-26*
