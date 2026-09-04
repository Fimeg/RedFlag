package logging

import (
	"bytes"
	"errors"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfigureProcessLoggerFileWritesFileAndConsole(t *testing.T) {
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	t.Cleanup(func() {
		processLoggerMu.Lock()
		if processLoggerFile != nil {
			_ = processLoggerFile.Close()
			processLoggerFile = nil
		}
		processLoggerMu.Unlock()
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	})

	logPath := filepath.Join(t.TempDir(), "agent.log")
	var console bytes.Buffer

	if err := ConfigureProcessLoggerFile(logPath, &console); err != nil {
		t.Fatalf("ConfigureProcessLoggerFile() error = %v", err)
	}

	entry := "[INFO] [agent] [logging] process_logger_test"
	log.Print(entry)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), entry) {
		t.Fatalf("log file missing entry %q: %s", entry, data)
	}
	if !strings.Contains(console.String(), entry) {
		t.Fatalf("console output missing entry %q: %s", entry, console.String())
	}
}

type failingWriter struct{}

func (f failingWriter) Write([]byte) (int, error) {
	return 0, errors.New("invalid handle")
}

func TestConfigureProcessLoggerFileWritesFileWhenConsoleFails(t *testing.T) {
	previousOutput := log.Writer()
	previousFlags := log.Flags()
	t.Cleanup(func() {
		processLoggerMu.Lock()
		if processLoggerFile != nil {
			_ = processLoggerFile.Close()
			processLoggerFile = nil
		}
		processLoggerMu.Unlock()
		log.SetOutput(previousOutput)
		log.SetFlags(previousFlags)
	})

	logPath := filepath.Join(t.TempDir(), "agent.log")
	if err := ConfigureProcessLoggerFile(logPath, failingWriter{}); err != nil {
		t.Fatalf("ConfigureProcessLoggerFile() error = %v", err)
	}

	entry := "[INFO] [agent] [logging] service_console_unavailable"
	log.Print(entry)

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file: %v", err)
	}
	if !strings.Contains(string(data), entry) {
		t.Fatalf("log file missing entry %q after console write failure: %s", entry, data)
	}
}
