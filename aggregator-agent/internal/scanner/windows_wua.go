//go:build windows
// +build windows

package scanner

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/aggregator-project/aggregator-agent/internal/client"
	"github.com/aggregator-project/aggregator-agent/pkg/windowsupdate"
	"github.com/go-ole/go-ole"
	"github.com/scjalliance/comshim"
)

// WindowsUpdateScannerWUA scans for Windows updates using the Windows Update Agent (WUA) API
type WindowsUpdateScannerWUA struct{}

// NewWindowsUpdateScannerWUA creates a new Windows Update scanner using WUA API
func NewWindowsUpdateScannerWUA() *WindowsUpdateScannerWUA {
	return &WindowsUpdateScannerWUA{}
}

// IsAvailable checks if WUA scanner is available on this system
func (s *WindowsUpdateScannerWUA) IsAvailable() bool {
	// Only available on Windows
	return runtime.GOOS == "windows"
}

// Scan scans for available Windows updates using the Windows Update Agent API
func (s *WindowsUpdateScannerWUA) Scan() ([]client.UpdateReportItem, error) {
	if !s.IsAvailable() {
		return nil, fmt.Errorf("WUA scanner is only available on Windows")
	}

	// Initialize COM
	comshim.Add(1)
	defer comshim.Done()

	ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED|ole.COINIT_SPEED_OVER_MEMORY)
	defer ole.CoUninitialize()

	// Create update session
	session, err := windowsupdate.NewUpdateSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create Windows Update session: %w", err)
	}

	// Create update searcher
	searcher, err := session.CreateUpdateSearcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create update searcher: %w", err)
	}

	// Search for available updates (IsInstalled=0 means not installed)
	searchCriteria := "IsInstalled=0 AND IsHidden=0"
	result, err := searcher.Search(searchCriteria)
	if err != nil {
		return nil, fmt.Errorf("failed to search for updates: %w", err)
	}

	// Convert results to our format
	updates := s.convertWUAResult(result)
	return updates, nil
}

// convertWUAResult converts WUA search results to our UpdateReportItem format
func (s *WindowsUpdateScannerWUA) convertWUAResult(result *windowsupdate.ISearchResult) []client.UpdateReportItem {
	var updates []client.UpdateReportItem

	updatesCollection := result.Updates
	if updatesCollection == nil {
		return updates
	}

	for _, update := range updatesCollection {
		if update == nil {
			continue
		}

		updateItem := s.convertWUAUpdate(update)
		updates = append(updates, *updateItem)
	}

	return updates
}

// convertWUAUpdate converts a single WUA update to our UpdateReportItem format
func (s *WindowsUpdateScannerWUA) convertWUAUpdate(update *windowsupdate.IUpdate) *client.UpdateReportItem {
	// Get update information
	title := update.Title
	description := update.Description
	kbArticles := s.getKBArticles(update)
	updateIdentity := update.Identity

	// Determine severity from categories
	severity := s.determineSeverityFromCategories(update)

	// Get version information
	maxDownloadSize := update.MaxDownloadSize
	estimatedSize := s.getEstimatedSize(update)

	// Create metadata with WUA-specific information
	metadata := map[string]interface{}{
		"package_manager": "windows_update",
		"detected_via":    "wua_api",
		"kb_articles":     kbArticles,
		"update_identity": updateIdentity.UpdateID,
		"revision_number": updateIdentity.RevisionNumber,
		"search_criteria": "IsInstalled=0 AND IsHidden=0",
		"download_size":  maxDownloadSize,
		"estimated_size": estimatedSize,
		"api_source":     "windows_update_agent",
		"scan_timestamp":  time.Now().Format(time.RFC3339),
	}

	// Add categories if available
	categories := s.getCategories(update)
	if len(categories) > 0 {
		metadata["categories"] = categories
	}

	updateItem := &client.UpdateReportItem{
		PackageType:        "windows_update",
		PackageName:        title,
		PackageDescription: description,
		CurrentVersion:     "Not Installed",
		AvailableVersion:   s.getVersionInfo(update),
		Severity:           severity,
		RepositorySource:   "Microsoft Update",
		Metadata:           metadata,
	}

	// Add size information to description if available
	if maxDownloadSize > 0 {
		sizeStr := s.formatFileSize(uint64(maxDownloadSize))
		updateItem.PackageDescription += fmt.Sprintf(" (Size: %s)", sizeStr)
	}

	return updateItem
}

// getKBArticles extracts KB article IDs from an update
func (s *WindowsUpdateScannerWUA) getKBArticles(update *windowsupdate.IUpdate) []string {
	kbCollection := update.KBArticleIDs
	if kbCollection == nil {
		return []string{}
	}

	// kbCollection is already a slice of strings
	return kbCollection
}

// getCategories extracts update categories
func (s *WindowsUpdateScannerWUA) getCategories(update *windowsupdate.IUpdate) []string {
	var categories []string

	categoryCollection := update.Categories
	if categoryCollection == nil {
		return categories
	}

	for _, category := range categoryCollection {
		if category != nil {
			name := category.Name
			categories = append(categories, name)
		}
	}

	return categories
}

// determineSeverityFromCategories determines severity based on update categories
func (s *WindowsUpdateScannerWUA) determineSeverityFromCategories(update *windowsupdate.IUpdate) string {
	categories := s.getCategories(update)
	title := strings.ToUpper(update.Title)

	// Critical Security Updates
	for _, category := range categories {
		categoryUpper := strings.ToUpper(category)
		if strings.Contains(categoryUpper, "SECURITY") ||
		   strings.Contains(categoryUpper, "CRITICAL") ||
		   strings.Contains(categoryUpper, "IMPORTANT") {
			return "critical"
		}
	}

	// Check title for security keywords
	if strings.Contains(title, "SECURITY") ||
	   strings.Contains(title, "CRITICAL") ||
	   strings.Contains(title, "IMPORTANT") ||
	   strings.Contains(title, "PATCH TUESDAY") {
		return "critical"
	}

	// Driver Updates
	for _, category := range categories {
		if strings.Contains(strings.ToUpper(category), "DRIVERS") {
			return "moderate"
		}
	}

	// Definition Updates
	for _, category := range categories {
		if strings.Contains(strings.ToUpper(category), "DEFINITION") ||
		   strings.Contains(strings.ToUpper(category), "ANTIVIRUS") ||
		   strings.Contains(strings.ToUpper(category), "ANTIMALWARE") {
			return "high"
		}
	}

	return "moderate"
}

// categorizeUpdate determines the type of update
func (s *WindowsUpdateScannerWUA) categorizeUpdate(title string, categories []string) string {
	titleUpper := strings.ToUpper(title)

	// Security Updates
	for _, category := range categories {
		if strings.Contains(strings.ToUpper(category), "SECURITY") {
			return "security"
		}
	}

	if strings.Contains(titleUpper, "SECURITY") ||
	   strings.Contains(titleUpper, "PATCH") ||
	   strings.Contains(titleUpper, "VULNERABILITY") {
		return "security"
	}

	// Driver Updates
	for _, category := range categories {
		if strings.Contains(strings.ToUpper(category), "DRIVERS") {
			return "driver"
			}
	}

	if strings.Contains(titleUpper, "DRIVER") {
		return "driver"
	}

	// Definition Updates
	for _, category := range categories {
		if strings.Contains(strings.ToUpper(category), "DEFINITION") {
			return "definition"
		}
	}

	if strings.Contains(titleUpper, "DEFINITION") ||
	   strings.Contains(titleUpper, "ANTIVIRUS") ||
	   strings.Contains(titleUpper, "ANTIMALWARE") {
		return "definition"
	}

	// Feature Updates
	if strings.Contains(titleUpper, "FEATURE") ||
	   strings.Contains(titleUpper, "VERSION") ||
	   strings.Contains(titleUpper, "UPGRADE") {
		return "feature"
	}

	// Quality Updates
	if strings.Contains(titleUpper, "QUALITY") ||
	   strings.Contains(titleUpper, "CUMULATIVE") {
		return "quality"
	}

	return "system"
}

// getVersionInfo extracts version information from update
func (s *WindowsUpdateScannerWUA) getVersionInfo(update *windowsupdate.IUpdate) string {
	// Try to get version from title or description
	title := update.Title
	description := update.Description

	// Look for version patterns
	title = s.extractVersionFromText(title)
	if title != "" {
		return title
	}

	return s.extractVersionFromText(description)
}

// extractVersionFromText extracts version information from text
func (s *WindowsUpdateScannerWUA) extractVersionFromText(text string) string {
	// Common version patterns to look for
	patterns := []string{
		`\b\d+\.\d+\.\d+\b`,           // x.y.z
		`\b\d+\.\d+\b`,               // x.y
		`\bKB\d+\b`,                  // KB numbers
		`\b\d{8}\b`,                 // 8-digit Windows build numbers
	}

	for _, pattern := range patterns {
		// This is a simplified version - in production you'd use regex
		if strings.Contains(text, pattern) {
			// For now, return a simplified extraction
			if strings.Contains(text, "KB") {
				return s.extractKBNumber(text)
			}
		}
	}

	return "Unknown"
}

// extractKBNumber extracts KB numbers from text
func (s *WindowsUpdateScannerWUA) extractKBNumber(text string) string {
	words := strings.Fields(text)
	for _, word := range words {
		if strings.HasPrefix(word, "KB") && len(word) > 2 {
			return word
		}
	}
	return ""
}

// getEstimatedSize gets the estimated size of the update
func (s *WindowsUpdateScannerWUA) getEstimatedSize(update *windowsupdate.IUpdate) uint64 {
	maxSize := update.MaxDownloadSize
	if maxSize > 0 {
		return uint64(maxSize)
	}
	return 0
}

// formatFileSize formats bytes into human readable string
func (s *WindowsUpdateScannerWUA) formatFileSize(bytes uint64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := uint64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// GetUpdateDetails retrieves detailed information about a specific Windows update
func (s *WindowsUpdateScannerWUA) GetUpdateDetails(updateID string) (*client.UpdateReportItem, error) {
	// This would require implementing a search by ID functionality
	// For now, we don't implement this as it would require additional WUA API calls
	return nil, fmt.Errorf("GetUpdateDetails not yet implemented for WUA scanner")
}

// GetUpdateHistory retrieves update history
func (s *WindowsUpdateScannerWUA) GetUpdateHistory() ([]client.UpdateReportItem, error) {
	if !s.IsAvailable() {
		return nil, fmt.Errorf("WUA scanner is only available on Windows")
	}

	// Initialize COM
	comshim.Add(1)
	defer comshim.Done()

	ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED|ole.COINIT_SPEED_OVER_MEMORY)
	defer ole.CoUninitialize()

	// Create update session
	session, err := windowsupdate.NewUpdateSession()
	if err != nil {
		return nil, fmt.Errorf("failed to create Windows Update session: %w", err)
	}

	// Create update searcher
	searcher, err := session.CreateUpdateSearcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create update searcher: %w", err)
	}

	// Query update history
	historyEntries, err := searcher.QueryHistoryAll()
	if err != nil {
		return nil, fmt.Errorf("failed to query update history: %w", err)
	}

	// Convert history to our format
	return s.convertHistoryEntries(historyEntries), nil
}

// convertHistoryEntries converts update history entries to our UpdateReportItem format
func (s *WindowsUpdateScannerWUA) convertHistoryEntries(entries []*windowsupdate.IUpdateHistoryEntry) []client.UpdateReportItem {
	var updates []client.UpdateReportItem

	for _, entry := range entries {
		if entry == nil {
			continue
		}

		// Create a basic update report item from history entry
		updateItem := &client.UpdateReportItem{
			PackageType:        "windows_update_history",
			PackageName:        entry.Title,
			PackageDescription: entry.Description,
			CurrentVersion:     "Installed",
			AvailableVersion:   "History Entry",
			Severity:           s.determineSeverityFromHistoryEntry(entry),
			RepositorySource:   "Microsoft Update",
			Metadata: map[string]interface{}{
				"detected_via":    "wua_history",
				"api_source":     "windows_update_agent",
				"scan_timestamp":  time.Now().Format(time.RFC3339),
				"history_date":    entry.Date,
				"operation":       entry.Operation,
				"result_code":     entry.ResultCode,
				"hresult":         entry.HResult,
			},
		}

		updates = append(updates, *updateItem)
	}

	return updates
}

// determineSeverityFromHistoryEntry determines severity from history entry
func (s *WindowsUpdateScannerWUA) determineSeverityFromHistoryEntry(entry *windowsupdate.IUpdateHistoryEntry) string {
	title := strings.ToUpper(entry.Title)

	// Check title for security keywords
	if strings.Contains(title, "SECURITY") ||
	   strings.Contains(title, "CRITICAL") ||
	   strings.Contains(title, "IMPORTANT") {
		return "critical"
	}

	if strings.Contains(title, "DEFINITION") ||
	   strings.Contains(title, "ANTIVIRUS") ||
	   strings.Contains(title, "ANTIMALWARE") {
		return "high"
	}

	return "moderate"
}