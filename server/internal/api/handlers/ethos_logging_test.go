package handlers_test

// ethos_logging_test.go — Pre-fix tests for fmt.Printf used as logging in handlers.
// D-2: docker_reports.go and metrics.go use fmt.Printf for warnings.
// EXCLUDES: setup.go (legitimate CLI wizard output).

import (
	"os"
	"strings"
	"testing"
)

func TestHandlerFilesUseFmtPrintfForLogging(t *testing.T) {
	// D-2: handlers use fmt.Printf for warning messages.
	// EXCLUDES setup.go which is legitimate CLI output.
	files := []string{"docker_reports.go", "metrics.go"}
	total := 0

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Logf("[WARNING] [server] [handlers] could not read %s: %v", f, err)
			continue
		}
		count := strings.Count(string(content), "fmt.Printf")
		total += count
	}

	if total > 0 {
		t.Errorf("[ERROR] [server] [handlers] D-2 NOT FIXED: %d fmt.Printf in handler logs", total)
	}

	t.Log("[INFO] [server] [handlers] D-2 FIXED: no fmt.Printf in handler logs")
}

func TestHandlerFilesUseStructuredLogging(t *testing.T) {
	files := []string{"docker_reports.go", "metrics.go"}

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		count := strings.Count(string(content), "fmt.Printf")
		if count > 0 {
			t.Errorf("[ERROR] [server] [handlers] %d fmt.Printf calls in %s.\n"+
				"D-2: use log.Printf with [TAG] [server] [handlers] format.", count, f)
		}
	}
}
