# Windows Updates Scanner

**Windows Update API integration for detecting and managing Windows updates.**

---

## Component Details

| Property | Value |
|----------|-------|
| Package | `windowsupdate` (Go) — https://github.com/ceshihao/windowsupdate |
| Platform | Windows |
| Execution time | ~30 seconds per scan |
| Output format | JSON array of update objects |
| Failure modes | WUA service unavailable, network timeout, API errors |

---

## Implementation

**File:** `agent/pkg/windowsupdate/client.go`

```go
func (c *Client) ScanUpdates(ctx context.Context) ([]windowsupdate.Update, error) {
    // 1. Initialize Windows Update API
    client := windowsupdate.NewClient()
    client.SetTimeout(30 * time.Second)

    // 2. Query for updates
    updates, err := client.Update()
    if err != nil {
        logSecurityEvent("[reliability] [agent] [windows] WUA query failed:", err)
        return nil, err
    }

    // 3. Filter ghost packages
    updates = filterGhostPackages(updates)

    // 4. Return filtered updates
    return updates, nil
}
```

---

## Integration Points

### 1. Command Dispatch

The Windows scanner is invoked via command execution flow:

```go
// agent/internal/orchestrator/system_scanner.go
case "scan_windows":
    updates, err := windowsupdate.ScanWindowsUpdates(ctx, agentID)
    if err != nil {
        return err
    }
    report := &SystemEvent{
        AgentID: agentID,
        EventType: EventTypeAgentScan,
        ScanType: "windows",
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
Agent Poll → Server creates "scan_windows" command
    ↓
Agent receives command
    ↓
ScanWindowsUpdates() queries Windows Update API
    ↓
Filter ghost packages (known issue: Windows occasionally reports stale updates)
    ↓
Return updates array
    ↓
Agent reports scan results to server
    ↓
Server displays updates in dashboard (SystemEvents table)
```

---

## Known Issues

### Ghost Packages

**Problem:** Windows Update API sometimes reports packages that are already installed or no longer available.

**Detection:** Windows Update API doesn't provide an easy way to filter these. Workaround: maintain a cache of previously seen update KB numbers and filter out duplicates.

**Status:** Partial fix — filtered in `filterGhostPackages()` but may not catch all cases.

**Cross-references:**
- `testing/02-windows-ghost.md` (ghost package test coverage)

---

### Update Reappearance

**Problem:** Some Windows Updates may reappear after installation (known Windows Update quirk).

**Status:** Known issue — logged but not automatically handled. Requires manual intervention or future fix.

---

## Footer: Assumptions & Connections

**Assumption:** Windows update scanning is a periodic operation that runs independently of command execution.

**Connection:** Circuit breaker (`verification/04-replay-protection.md`) prevents Windows scanner from blocking other subsystems.

**Connection:** System events (`flows/04-heartbeat.md`) published to history table for audit trail.

**Connection:** Windows scanner (`scanners/01-windows-updates.md`) lives in `agent/pkg/windowsupdate/` package.

---

*Last reviewed: 2026-05-26*
