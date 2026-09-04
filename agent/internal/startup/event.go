// Package startup provides startup event logging for the agent
// [TD-002] Phase 0 - Startup event tracking
package startup

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

// Event represents a startup event
type Event struct {
	Timestamp   time.Time `json:"timestamp"`
	Type        string    `json:"type"`
	Version     string    `json:"version"`
	PID         int       `json:"pid"`
	Hostname    string    `json:"hostname"`
	Success     bool      `json:"success"`
	Error       string    `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
}

const (
	EventTypeStartup    = "startup"
	EventTypeShutdown   = "shutdown"
	EventTypePanic      = "panic_recovery"
	EventTypeConfigLoad = "config_load"
)

// Logger handles startup event logging
type Logger struct {
	stateDir string
	version  string
}

// NewLogger creates a new startup event logger
func NewLogger(stateDir string, version string) *Logger {
	return &Logger{
		stateDir: stateDir,
		version:  version,
	}
}

// LogEvent logs a startup event to file
func (l *Logger) LogEvent(eventType string, success bool, err error, metadata map[string]interface{}) error {
	hostname, _ := os.Hostname()
	
	event := Event{
		Timestamp: time.Now().UTC(),
		Type:      eventType,
		Version:   l.version,
		PID:       os.Getpid(),
		Hostname:  hostname,
		Success:   success,
		Metadata:  metadata,
	}
	
	if err != nil {
		event.Error = err.Error()
	}
	
	// Log to stdout
	log.Printf("[STARTUP] [%s] success=%v version=%s pid=%d", eventType, success, l.version, event.PID)
	
	// Write to state file
	return l.writeEvent(event)
}

// writeEvent writes the event to the startup events file
func (l *Logger) writeEvent(event Event) error {
	eventsFile := filepath.Join(l.stateDir, "startup_events.jsonl")
	
	// Ensure directory exists
	if err := os.MkdirAll(l.stateDir, 0750); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}
	
	// Marshal event to JSON
	data, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}
	
	// Append to file
	f, err := os.OpenFile(eventsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0640)
	if err != nil {
		return fmt.Errorf("failed to open events file: %w", err)
	}
	defer f.Close()
	
	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("failed to write event: %w", err)
	}
	
	return nil
}

// GetLastStartup reads the last startup event
func (l *Logger) GetLastStartup() (*Event, error) {
	eventsFile := filepath.Join(l.stateDir, "startup_events.jsonl")
	
	data, err := os.ReadFile(eventsFile)
	if err != nil {
		return nil, err
	}
	
	// Parse last line
	lines := splitLines(data)
	if len(lines) == 0 {
		return nil, fmt.Errorf("no events found")
	}
	
	var event Event
	if err := json.Unmarshal(lines[len(lines)-1], &event); err != nil {
		return nil, err
	}
	
	return &event, nil
}

func splitLines(data []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i, b := range data {
		if b == '\n' {
			if i > start {
				lines = append(lines, data[start:i])
			}
			start = i + 1
		}
	}
	if start < len(data) {
		lines = append(lines, data[start:])
	}
	return lines
}
