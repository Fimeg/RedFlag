package queries_test

// ethos_logging_test.go — Pre-fix tests for fmt.Printf used as logging.
// D-2: Database query files use fmt.Printf for warnings and cleanup.

import (
	"os"
	"strings"
	"testing"
)

func TestQueriesUseFmtPrintfForLogging(t *testing.T) {
	// D-2: Database query files use fmt.Printf for warning and cleanup
	// messages. These should use log.Printf with ETHOS format.
	files := []string{"docker.go", "metrics.go", "updates.go"}
	total := 0

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			t.Logf("[WARNING] [server] [database] could not read %s: %v", f, err)
			continue
		}
		count := strings.Count(string(content), "fmt.Printf")
		count += strings.Count(string(content), "fmt.Println")
		total += count
	}

	if total > 0 {
		t.Errorf("[ERROR] [server] [database] D-2 NOT FIXED: %d fmt.Printf calls in query files", total)
	}

	t.Log("[INFO] [server] [database] D-2 FIXED: no fmt.Printf in query files")
}

func TestQueriesUseStructuredLogging(t *testing.T) {
	files := []string{"docker.go", "metrics.go", "updates.go"}

	for _, f := range files {
		content, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		count := strings.Count(string(content), "fmt.Printf")
		count += strings.Count(string(content), "fmt.Println")
		if count > 0 {
			t.Errorf("[ERROR] [server] [database] %d fmt.Printf calls in %s.\n"+
				"D-2: use log.Printf with [TAG] [server] [database] format.", count, f)
		}
	}
}
