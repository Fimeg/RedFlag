package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/agent/internal/localapi"
)

// HandleLocalStatusCommand displays live agent status through the local IPC API.
// It intentionally runs before config loading, so membership in the local access
// group is enough to inspect local state without reading protected config files.
func HandleLocalStatusCommand(exportFormat string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	snapshot, err := localapi.FetchSnapshot(ctx, localapi.ClientOptions{})
	if err != nil {
		return err
	}

	if exportFormat == "json" {
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(snapshot)
	}

	printLocalStatus(snapshot)
	return nil
}

func printLocalStatus(snapshot *localapi.Snapshot) {
	identity := snapshot.Identity
	status := snapshot.Status

	fmt.Println("==================================================================")
	fmt.Println("RedFlag Local Agent Status")
	fmt.Println("==================================================================")
	fmt.Printf("Agent ID: %s\n", identity.AgentID)
	fmt.Printf("Server: %s\n", identity.ServerURL)
	if identity.Hostname != "" {
		fmt.Printf("Hostname: %s\n", identity.Hostname)
	}
	if identity.OSType != "" {
		fmt.Printf("OS Type: %s\n", identity.OSType)
	}
	if identity.DisplayName != "" {
		fmt.Printf("Display Name: %s\n", identity.DisplayName)
	}
	if len(identity.Tags) > 0 {
		fmt.Printf("Tags: %s\n", strings.Join(identity.Tags, ", "))
	}
	fmt.Printf("Version: %s\n", identity.AgentVersion)
	fmt.Printf("Registered: %t\n", identity.Registered)
	fmt.Println()

	fmt.Printf("Agent Status: %s\n", fallback(status.AgentStatus, "unknown"))
	if !status.LastCheckIn.IsZero() {
		fmt.Printf("Last Check-in: %s\n", status.LastCheckIn.Format(time.RFC3339))
	}
	if !status.LastScan.IsZero() {
		fmt.Printf("Last Scan: %s\n", status.LastScan.Format(time.RFC3339))
	}
	fmt.Printf("Updates Available: %d\n", status.UpdateCount)
	fmt.Printf("Summary Total: %d\n", status.Summary.Total)

	if len(status.Summary.ByEcosystem) > 0 {
		fmt.Printf("By Ecosystem: %s\n", formatCounts(status.Summary.ByEcosystem))
	}
	if len(status.Summary.BySeverity) > 0 {
		fmt.Printf("By Severity: %s\n", formatCounts(status.Summary.BySeverity))
	}

	if len(status.Scanners) > 0 {
		fmt.Println()
		fmt.Println("Scanners:")
		names := make([]string, 0, len(status.Scanners))
		for name := range status.Scanners {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			scanner := status.Scanners[name]
			line := fmt.Sprintf("  - %s: %s (%d updates)", scanner.Name, fallback(scanner.Status, "unknown"), scanner.UpdateCount)
			if scanner.LastError != "" {
				line += fmt.Sprintf(" error=%q", scanner.LastError)
			}
			fmt.Println(line)
		}
	}

	fmt.Println("==================================================================")
}

func formatCounts(counts map[string]int) string {
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, counts[key]))
	}
	return strings.Join(parts, ", ")
}

func fallback(value, replacement string) string {
	if value == "" {
		return replacement
	}
	return value
}
