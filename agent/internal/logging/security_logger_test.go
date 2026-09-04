package logging

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Fimeg/RedFlag/agent/internal/config"
)

func TestSecurityLoggerGetBatchDoesNotClearUntilSuccessfulSend(t *testing.T) {
	sl, err := NewSecurityLogger(&config.Config{}, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer sl.Close()

	if err := sl.Log(&SecurityEvent{Level: "WARNING", EventType: "TEST_EVENT", Message: "first"}); err != nil {
		t.Fatal(err)
	}

	first := sl.GetBatch()
	if len(first) != 1 {
		t.Fatalf("first batch len = %d, want 1", len(first))
	}
	second := sl.GetBatch()
	if len(second) != 1 {
		t.Fatalf("second batch len = %d, want retained event before ClearBatch", len(second))
	}

	sl.ClearBatch(len(first))
	if got := sl.GetBatch(); len(got) != 0 {
		t.Fatalf("batch len after ClearBatch = %d, want 0", len(got))
	}
}

func TestSecurityLoggerFlushDoesNotDrainServerBufferOrDuplicateFileWrites(t *testing.T) {
	dir := t.TempDir()
	sl, err := NewSecurityLogger(&config.Config{}, dir)
	if err != nil {
		t.Fatal(err)
	}
	defer sl.Close()

	if err := sl.Log(&SecurityEvent{Level: "WARNING", EventType: "TEST_EVENT", Message: "first"}); err != nil {
		t.Fatal(err)
	}
	sl.flushBuffer()
	sl.flushBuffer()

	if got := sl.GetBatch(); len(got) != 1 {
		t.Fatalf("batch len after file flush = %d, want 1", len(got))
	}

	body, err := os.ReadFile(filepath.Join(dir, "security.log"))
	if err != nil {
		t.Fatal(err)
	}
	if count := strings.Count(string(body), "TEST_EVENT"); count != 1 {
		t.Fatalf("file event count = %d, want 1\n%s", count, string(body))
	}
}
