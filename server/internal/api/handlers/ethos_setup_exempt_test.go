package handlers_test

// ethos_setup_exempt_test.go — Exemption documentation for setup wizard output.
// D-2: setup.go fmt.Printf is EXEMPT from ETHOS #1.
// The setup wizard is user-facing CLI output, not background log statements.
// Do NOT modify setup.go fmt.Printf in D-2 fix pass.

import (
	"os"
	"strings"
	"testing"
)

func TestSetupHandlerIsExemptFromEthos(t *testing.T) {
	// D-2: setup.go fmt.Printf is EXEMPT.
	// The setup wizard is user-facing CLI output.
	// ETHOS #1 applies to background log statements.
	content, err := os.ReadFile("setup.go")
	if err != nil {
		t.Fatalf("failed to read setup.go: %v", err)
	}

	hasFmtPrint := strings.Contains(string(content), "fmt.Printf") ||
		strings.Contains(string(content), "fmt.Println")

	if !hasFmtPrint {
		t.Log("[INFO] [server] [handlers] setup.go has no fmt.Printf (may have been restructured)")
	} else {
		t.Log("[INFO] [server] [handlers] setup.go uses fmt.Printf (EXEMPT — CLI wizard output)")
	}

	t.Log("[INFO] [server] [handlers] EXEMPTION: setup.go fmt.Printf is intentional CLI output")
}
