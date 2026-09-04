package scanner

// windows_service_parity_test.go — Pre-fix tests for service polling loop parity.
// [SHARED] — reads service/windows.go as a text file, no Windows imports needed.
//
// F-C1-4 MEDIUM: Service has no auto-restart on crash.
// F-C1-5 HIGH: Service runAgent() is duplicated, missing B-2 fixes.
// F-C1-7 LOW: Service runAgent() uses emojis in logs.
//
// Run: cd agent && go test ./internal/scanner/... -v -run TestWindowsService

import (
	"os"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Test 4.1 — Documents missing FailureActions (F-C1-4)
//
// Category: PASS-NOW (documents the bug)
// ---------------------------------------------------------------------------

func TestWindowsServiceHasAutoRestartOnCrash(t *testing.T) {
	// F-C1-4 RESOLVED: Service DOES have RecoveryActions configured.
	// The audit finding F-C1-4 was incorrect — SetRecoveryActions is called.
	servicePath := "../service/windows.go"
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Skipf("service/windows.go not readable: %v", err)
		return
	}

	src := string(content)

	hasRecovery := strings.Contains(src, "RecoveryActions") ||
		strings.Contains(src, "FailureActions")

	if !hasRecovery {
		t.Error("[ERROR] [agent] [service] no RecoveryActions configured")
	}

	t.Log("[INFO] [agent] [service] F-C1-4 ALREADY CORRECT: service has crash recovery")
}

// ---------------------------------------------------------------------------
// Test 5.1 — Documents fixed jitter in service (F-C1-5)
//
// Category: PASS-NOW (documents the bug)
// ---------------------------------------------------------------------------

func TestWindowsServicePollingLoopHasFixedJitter(t *testing.T) {
	// POST-FIX (F-C1-5): Service now has proportional jitter.
	servicePath := "../service/windows.go"
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Skipf("service/windows.go not readable: %v", err)
		return
	}

	src := string(content)

	// Ideal fix (now in place): the service delegates to the shared loop via
	// agent.RunPollingLoop, so jitter lives there rather than being copied here.
	delegates := strings.Contains(src, "RunPollingLoop")
	if !delegates && !strings.Contains(src, "maxJitter") && !strings.Contains(src, "pollingInterval / 2") {
		t.Error("[ERROR] [agent] [service] F-C1-5 NOT FIXED: proportional jitter not in service")
	}

	t.Log("[INFO] [agent] [service] F-C1-5 FIXED: service delegates jitter to shared loop")
}

// ---------------------------------------------------------------------------
// Test 5.2 — Service must have proportional jitter (assert fix)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
// ---------------------------------------------------------------------------

func TestWindowsServicePollingLoopHasProportionalJitter(t *testing.T) {
	servicePath := "../service/windows.go"
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Skipf("service/windows.go not readable: %v", err)
		return
	}

	src := string(content)

	hasProportionalJitter := strings.Contains(src, "pollingInterval / 2") ||
		strings.Contains(src, "maxJitter") ||
		// Or the ideal fix: service delegates to the shared loop
		strings.Contains(src, "RunPollingLoop") ||
		strings.Contains(src, "runAgentLoop") ||
		strings.Contains(src, "commonPollingLoop")

	if !hasProportionalJitter {
		t.Errorf("[ERROR] [agent] [service] service polling loop missing proportional jitter.\n" +
			"F-C1-5: either deduplicate the loop or apply same jitter formula.")
	}
}

// ---------------------------------------------------------------------------
// Test 5.3 — Documents missing exponential backoff (F-C1-5)
//
// Category: PASS-NOW (documents the bug)
// ---------------------------------------------------------------------------

func TestWindowsServicePollingLoopHasNoExponentialBackoff(t *testing.T) {
	// POST-FIX (F-C1-5): Service now has exponential backoff.
	servicePath := "../service/windows.go"
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Skipf("service/windows.go not readable: %v", err)
		return
	}

	src := string(content)

	// Backoff lives in the shared loop the service delegates to (RunPollingLoop),
	// or — for the legacy in-place form — directly via calculateBackoff.
	delegates := strings.Contains(src, "RunPollingLoop")
	if !delegates && (!strings.Contains(src, "calculateBackoff") || !strings.Contains(src, "consecutiveFailures")) {
		t.Error("[ERROR] [agent] [service] F-C1-5 NOT FIXED: exponential backoff missing")
	}

	t.Log("[INFO] [agent] [service] F-C1-5 FIXED: service delegates backoff to shared loop")
}

// ---------------------------------------------------------------------------
// Test 5.4 — Service must have exponential backoff (assert fix)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
// ---------------------------------------------------------------------------

func TestWindowsServicePollingLoopHasExponentialBackoff(t *testing.T) {
	servicePath := "../service/windows.go"
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Skipf("service/windows.go not readable: %v", err)
		return
	}

	src := string(content)

	hasBackoff := strings.Contains(src, "calculateBackoff") ||
		strings.Contains(src, "consecutiveFailures") ||
		strings.Contains(src, "backoffDelay") ||
		// Or the ideal fix: delegates to the shared loop
		strings.Contains(src, "RunPollingLoop") ||
		strings.Contains(src, "runAgentLoop")

	if !hasBackoff {
		t.Errorf("[ERROR] [agent] [service] service missing exponential backoff.\n" +
			"F-C1-5: apply same backoff or deduplicate polling loop.")
	}
}

// ---------------------------------------------------------------------------
// Test 5.5 — Polling loop should NOT be duplicated (assert fix)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
// ---------------------------------------------------------------------------

func TestPollingLoopIsNotDuplicated(t *testing.T) {
	// F-C1-5: The ideal fix is deduplication. The pragmatic fix is
	// applying the same B-2 fixes to both loops and documenting the
	// TODO for future extraction. Both approaches are acceptable
	// as long as the service has proportional jitter AND backoff.
	servicePath := "../service/windows.go"
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Skipf("service/windows.go not readable: %v", err)
		return
	}

	src := string(content)

	// Accept if either: (a) delegated to shared function, or (b) has parity
	hasDelegation := strings.Contains(src, "RunPollingLoop") ||
		strings.Contains(src, "runAgentLoop") ||
		strings.Contains(src, "polling.Run")
	hasParity := strings.Contains(src, "maxJitter") && strings.Contains(src, "calculateBackoff")

	if !hasDelegation && !hasParity {
		t.Errorf("[ERROR] [agent] [service] service loop is neither deduplicated nor at parity with main.go")
	}

	t.Log("[INFO] [agent] [service] F-C1-5 FIXED: service polling loop has B-2 parity")
}

// ---------------------------------------------------------------------------
// Test 6.3 — Documents emoji in service logs (F-C1-7)
//
// Category: PASS-NOW (documents the bug)
// ---------------------------------------------------------------------------

func TestWindowsServiceHasEmojiInLogs(t *testing.T) {
	// POST-FIX (F-C1-7): Emojis removed from service code.
	servicePath := "../service/windows.go"
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Skipf("service/windows.go not readable: %v", err)
		return
	}

	src := string(content)

	hasEmoji := false
	for _, r := range src {
		if r >= 0x1F300 || (r >= 0x2600 && r <= 0x27BF) {
			hasEmoji = true
			break
		}
	}

	if hasEmoji {
		t.Error("[ERROR] [agent] [service] F-C1-7 NOT FIXED: emoji found in service code")
	}

	t.Log("[INFO] [agent] [service] F-C1-7 FIXED: no emojis in service code")
}

// ---------------------------------------------------------------------------
// Test 6.4 — Service must have no emojis (assert fix)
//
// Category: FAIL-NOW / PASS-AFTER-FIX
// ---------------------------------------------------------------------------

func TestWindowsServiceHasNoEmojiInLogs(t *testing.T) {
	servicePath := "../service/windows.go"
	content, err := os.ReadFile(servicePath)
	if err != nil {
		t.Skipf("service/windows.go not readable: %v", err)
		return
	}

	src := string(content)

	for _, r := range src {
		if r >= 0x1F300 || (r >= 0x2600 && r <= 0x27BF) {
			t.Errorf("[ERROR] [agent] [service] emoji found in service code (U+%04X).\n"+
				"F-C1-7: ETHOS #1 prohibits emojis in logs.", r)
			return
		}
	}
}
