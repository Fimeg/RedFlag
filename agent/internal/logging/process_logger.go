package logging

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"sync"

	"github.com/Fimeg/RedFlag/agent/internal/constants"
)

var (
	processLoggerMu   sync.Mutex
	processLoggerFile *os.File
)

// ConfigureProcessLogger routes the standard library logger to the primary
// Windows agent log. Linux keeps its existing service/journal behavior.
func ConfigureProcessLogger() error {
	if runtime.GOOS != "windows" {
		return nil
	}
	return ConfigureProcessLoggerFile(constants.GetAgentLogPath(), os.Stderr)
}

// ConfigureProcessLoggerFile wires log.Printf/log.Fatal output to both the
// console stream and a durable file. This preserves CLI feedback while making
// Windows service diagnostics available without Event Viewer.
func ConfigureProcessLoggerFile(logPath string, console io.Writer) error {
	if logPath == "" {
		return fmt.Errorf("agent log path is empty")
	}
	if console == nil {
		console = io.Discard
	}

	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0750); err != nil {
		return fmt.Errorf("failed to create agent log directory %q: %w", logDir, err)
	}

	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return fmt.Errorf("failed to open agent log file %q: %w", logPath, err)
	}

	processLoggerMu.Lock()
	defer processLoggerMu.Unlock()

	oldFile := processLoggerFile
	processLoggerFile = file
	log.SetFlags(log.LstdFlags | log.LUTC)
	log.SetOutput(io.MultiWriter(file, console))

	if oldFile != nil {
		_ = oldFile.Close()
	}
	log.Printf("[INFO] [agent] [logging] process_logger_initialized path=%s", logPath)
	return nil
}
