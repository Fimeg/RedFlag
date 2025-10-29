package display

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/aggregator-agent/internal/client"
)

// Color codes for terminal output
const (
	ColorReset  = "\033[0m"
	ColorRed    = "\033[31m"
	ColorGreen  = "\033[32m"
	ColorYellow = "\033[33m"
	ColorBlue   = "\033[34m"
	ColorPurple = "\033[35m"
	ColorCyan   = "\033[36m"
	ColorWhite  = "\033[37m"
	ColorBold   = "\033[1m"
)

// SeverityColors maps severity levels to colors
var SeverityColors = map[string]string{
	"critical": ColorRed,
	"high":     ColorRed,
	"medium":   ColorYellow,
	"moderate": ColorYellow,
	"low":      ColorGreen,
	"info":     ColorBlue,
}

// PrintScanResults displays scan results in a pretty format
func PrintScanResults(updates []client.UpdateReportItem, exportFormat string) error {
	// Handle export formats
	if exportFormat != "" {
		return exportResults(updates, exportFormat)
	}

	// Count updates by type
	aptCount := 0
	dockerCount := 0
	otherCount := 0

	for _, update := range updates {
		switch update.PackageType {
		case "apt":
			aptCount++
		case "docker":
			dockerCount++
		default:
			otherCount++
		}
	}

	// Header
	fmt.Printf("%s🚩 RedFlag Update Scan Results%s\n", ColorBold+ColorRed, ColorReset)
	fmt.Printf("%s%sScan completed: %s%s\n", ColorBold, ColorCyan, time.Now().Format("2006-01-02 15:04:05"), ColorReset)
	fmt.Println()

	// Summary
	if len(updates) == 0 {
		fmt.Printf("%s✅ No updates available - system is up to date!%s\n", ColorBold+ColorGreen, ColorReset)
		return nil
	}

	fmt.Printf("%s📊 Summary:%s\n", ColorBold+ColorBlue, ColorReset)
	fmt.Printf("  Total updates: %s%d%s\n", ColorBold+ColorYellow, len(updates), ColorReset)

	if aptCount > 0 {
		fmt.Printf("  APT packages: %s%d%s\n", ColorBold+ColorCyan, aptCount, ColorReset)
	}
	if dockerCount > 0 {
		fmt.Printf("  Docker images: %s%d%s\n", ColorBold+ColorCyan, dockerCount, ColorReset)
	}
	if otherCount > 0 {
		fmt.Printf("  Other: %s%d%s\n", ColorBold+ColorCyan, otherCount, ColorReset)
	}
	fmt.Println()

	// Group by package type
	if aptCount > 0 {
		printAPTUpdates(updates)
	}

	if dockerCount > 0 {
		printDockerUpdates(updates)
	}

	if otherCount > 0 {
		printOtherUpdates(updates)
	}

	// Footer
	fmt.Println()
	fmt.Printf("%s💡 Tip: Use --list-updates for detailed information or --export=json for automation%s\n", ColorBold+ColorYellow, ColorReset)

	return nil
}

// printAPTUpdates displays APT package updates
func printAPTUpdates(updates []client.UpdateReportItem) {
	fmt.Printf("%s📦 APT Package Updates%s\n", ColorBold+ColorBlue, ColorReset)
	fmt.Println(strings.Repeat("─", 50))

	for _, update := range updates {
		if update.PackageType != "apt" {
			continue
		}

		severityColor := getSeverityColor(update.Severity)
		packageIcon := getPackageIcon(update.Severity)

		fmt.Printf("%s %s%s%s\n", packageIcon, ColorBold, update.PackageName, ColorReset)
		fmt.Printf("  Version: %s→%s\n",
			getVersionColor(update.CurrentVersion),
			getVersionColor(update.AvailableVersion))

		if update.Severity != "" {
			fmt.Printf("  Severity: %s%s%s\n", severityColor, update.Severity, ColorReset)
		}

		if update.PackageDescription != "" {
			fmt.Printf("  Description: %s\n", truncateString(update.PackageDescription, 60))
		}

		if len(update.CVEList) > 0 {
			fmt.Printf("  CVEs: %s\n", strings.Join(update.CVEList, ", "))
		}

		if update.RepositorySource != "" {
			fmt.Printf("  Source: %s\n", update.RepositorySource)
		}

		if update.SizeBytes > 0 {
			fmt.Printf("  Size: %s\n", formatBytes(update.SizeBytes))
		}

		fmt.Println()
	}
}

// printDockerUpdates displays Docker image updates
func printDockerUpdates(updates []client.UpdateReportItem) {
	fmt.Printf("%s🐳 Docker Image Updates%s\n", ColorBold+ColorBlue, ColorReset)
	fmt.Println(strings.Repeat("─", 50))

	for _, update := range updates {
		if update.PackageType != "docker" {
			continue
		}

		severityColor := getSeverityColor(update.Severity)
		imageIcon := "🐳"

		fmt.Printf("%s %s%s%s\n", imageIcon, ColorBold, update.PackageName, ColorReset)

		if update.Severity != "" {
			fmt.Printf("  Severity: %s%s%s\n", severityColor, update.Severity, ColorReset)
		}

		// Show digest comparison if available
		if update.CurrentVersion != "" && update.AvailableVersion != "" {
			fmt.Printf("  Digest: %s→%s\n",
				truncateString(update.CurrentVersion, 12),
				truncateString(update.AvailableVersion, 12))
		}

		if update.PackageDescription != "" {
			fmt.Printf("  Description: %s\n", truncateString(update.PackageDescription, 60))
		}

		if len(update.CVEList) > 0 {
			fmt.Printf("  CVEs: %s\n", strings.Join(update.CVEList, ", "))
		}

		fmt.Println()
	}
}

// printOtherUpdates displays updates from other package managers
func printOtherUpdates(updates []client.UpdateReportItem) {
	fmt.Printf("%s📋 Other Updates%s\n", ColorBold+ColorBlue, ColorReset)
	fmt.Println(strings.Repeat("─", 50))

	for _, update := range updates {
		if update.PackageType == "apt" || update.PackageType == "docker" {
			continue
		}

		severityColor := getSeverityColor(update.Severity)
		packageIcon := "📦"

		fmt.Printf("%s %s%s%s (%s)\n", packageIcon, ColorBold, update.PackageName, ColorReset, update.PackageType)
		fmt.Printf("  Version: %s→%s\n",
			getVersionColor(update.CurrentVersion),
			getVersionColor(update.AvailableVersion))

		if update.Severity != "" {
			fmt.Printf("  Severity: %s%s%s\n", severityColor, update.Severity, ColorReset)
		}

		if update.PackageDescription != "" {
			fmt.Printf("  Description: %s\n", truncateString(update.PackageDescription, 60))
		}

		fmt.Println()
	}
}

// PrintDetailedUpdates shows full details for all updates
func PrintDetailedUpdates(updates []client.UpdateReportItem, exportFormat string) error {
	// Handle export formats
	if exportFormat != "" {
		return exportResults(updates, exportFormat)
	}

	fmt.Printf("%s🔍 Detailed Update Information%s\n", ColorBold+ColorPurple, ColorReset)
	fmt.Printf("%sGenerated: %s%s\n\n", ColorCyan, time.Now().Format("2006-01-02 15:04:05"), ColorReset)

	if len(updates) == 0 {
		fmt.Printf("%s✅ No updates available%s\n", ColorBold+ColorGreen, ColorReset)
		return nil
	}

	for i, update := range updates {
		fmt.Printf("%sUpdate #%d%s\n", ColorBold+ColorYellow, i+1, ColorReset)
		fmt.Println(strings.Repeat("═", 60))

		fmt.Printf("%sPackage:%s %s\n", ColorBold, ColorReset, update.PackageName)
		fmt.Printf("%sType:%s %s\n", ColorBold, ColorReset, update.PackageType)
		fmt.Printf("%sCurrent Version:%s %s\n", ColorBold, ColorReset, update.CurrentVersion)
		fmt.Printf("%sAvailable Version:%s %s\n", ColorBold, ColorReset, update.AvailableVersion)

		if update.Severity != "" {
			severityColor := getSeverityColor(update.Severity)
			fmt.Printf("%sSeverity:%s %s%s%s\n", ColorBold, ColorReset, severityColor, update.Severity, ColorReset)
		}

		if update.PackageDescription != "" {
			fmt.Printf("%sDescription:%s %s\n", ColorBold, ColorReset, update.PackageDescription)
		}

		if len(update.CVEList) > 0 {
			fmt.Printf("%sCVE List:%s %s\n", ColorBold, ColorReset, strings.Join(update.CVEList, ", "))
		}

		if update.KBID != "" {
			fmt.Printf("%sKB Article:%s %s\n", ColorBold, ColorReset, update.KBID)
		}

		if update.RepositorySource != "" {
			fmt.Printf("%sRepository:%s %s\n", ColorBold, ColorReset, update.RepositorySource)
		}

		if update.SizeBytes > 0 {
			fmt.Printf("%sSize:%s %s\n", ColorBold, ColorReset, formatBytes(update.SizeBytes))
		}

		if len(update.Metadata) > 0 {
			fmt.Printf("%sMetadata:%s\n", ColorBold, ColorReset)
			for key, value := range update.Metadata {
				fmt.Printf("  %s: %v\n", key, value)
			}
		}

		fmt.Println()
	}

	return nil
}

// PrintAgentStatus displays agent status information
func PrintAgentStatus(agentID string, serverURL string, lastCheckIn time.Time, lastScan time.Time, updateCount int, agentStatus string) {
	fmt.Printf("%s🚩 RedFlag Agent Status%s\n", ColorBold+ColorRed, ColorReset)
	fmt.Println(strings.Repeat("─", 40))

	fmt.Printf("%sAgent ID:%s %s\n", ColorBold, ColorReset, agentID)
	fmt.Printf("%sServer:%s %s\n", ColorBold, ColorReset, serverURL)
	fmt.Printf("%sStatus:%s %s%s%s\n", ColorBold, ColorReset, getSeverityColor(agentStatus), agentStatus, ColorReset)

	if !lastCheckIn.IsZero() {
		fmt.Printf("%sLast Check-in:%s %s\n", ColorBold, ColorReset, formatTimeSince(lastCheckIn))
	} else {
		fmt.Printf("%sLast Check-in:%s %sNever%s\n", ColorBold, ColorReset, ColorYellow, ColorReset)
	}

	if !lastScan.IsZero() {
		fmt.Printf("%sLast Scan:%s %s\n", ColorBold, ColorReset, formatTimeSince(lastScan))
		fmt.Printf("%sUpdates Found:%s %s%d%s\n", ColorBold, ColorReset, ColorYellow, updateCount, ColorReset)
	} else {
		fmt.Printf("%sLast Scan:%s %sNever%s\n", ColorBold, ColorReset, ColorYellow, ColorReset)
	}

	fmt.Println()
}

// Helper functions

func getSeverityColor(severity string) string {
	if color, ok := SeverityColors[severity]; ok {
		return color
	}
	return ColorWhite
}

func getPackageIcon(severity string) string {
	switch strings.ToLower(severity) {
	case "critical", "high":
		return "🔴"
	case "medium", "moderate":
		return "🟡"
	case "low":
		return "🟢"
	default:
		return "🔵"
	}
}

func getVersionColor(version string) string {
	if version == "" {
		return ColorRed + "unknown" + ColorReset
	}
	return ColorCyan + version + ColorReset
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

func formatTimeSince(t time.Time) string {
	duration := time.Since(t)
	if duration < time.Minute {
		return fmt.Sprintf("%d seconds ago", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%d minutes ago", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(duration.Hours()))
	} else {
		return fmt.Sprintf("%d days ago", int(duration.Hours()/24))
	}
}

func exportResults(updates []client.UpdateReportItem, format string) error {
	switch strings.ToLower(format) {
	case "json":
		encoder := json.NewEncoder(os.Stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(updates)

	case "csv":
		return exportCSV(updates)

	default:
		return fmt.Errorf("unsupported export format: %s (supported: json, csv)", format)
	}
}

func exportCSV(updates []client.UpdateReportItem) error {
	// Print CSV header
	fmt.Println("PackageType,PackageName,CurrentVersion,AvailableVersion,Severity,CVEList,Description,SizeBytes")

	// Print each update as CSV row
	for _, update := range updates {
		cveList := strings.Join(update.CVEList, ";")
		description := strings.ReplaceAll(update.PackageDescription, ",", ";")
		description = strings.ReplaceAll(description, "\n", " ")

		fmt.Printf("%s,%s,%s,%s,%s,%s,%s,%d\n",
			update.PackageType,
			update.PackageName,
			update.CurrentVersion,
			update.AvailableVersion,
			update.Severity,
			cveList,
			description,
			update.SizeBytes,
		)
	}

	return nil
}