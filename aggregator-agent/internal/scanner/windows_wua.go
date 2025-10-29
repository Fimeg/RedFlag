//go:build windows
// +build windows

package scanner

import (
	"fmt"
	"runtime"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/aggregator-agent/internal/client"
	"github.com/Fimeg/RedFlag/aggregator-agent/pkg/windowsupdate"
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

	// Use MSRC severity if available (more accurate than category-based detection)
	severity := s.mapMsrcSeverity(update.MsrcSeverity)
	if severity == "" {
		severity = s.determineSeverityFromCategories(update)
	}

	// Get version information with improved parsing
	currentVersion, availableVersion := s.parseVersionFromTitle(title)

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

	// Add MSRC severity if available
	if update.MsrcSeverity != "" {
		metadata["msrc_severity"] = update.MsrcSeverity
	}

	// Add security bulletin IDs (includes CVEs)
	if len(update.SecurityBulletinIDs) > 0 {
		metadata["security_bulletins"] = update.SecurityBulletinIDs
		// Extract CVEs from security bulletins
		cveList := make([]string, 0)
		for _, bulletin := range update.SecurityBulletinIDs {
			if strings.HasPrefix(bulletin, "CVE-") {
				cveList = append(cveList, bulletin)
			}
		}
		if len(cveList) > 0 {
			metadata["cve_list"] = cveList
		}
	}

	// Add deployment information
	if update.LastDeploymentChangeTime != nil {
		metadata["last_deployment_change"] = update.LastDeploymentChangeTime.Format(time.RFC3339)
		metadata["discovered_at"] = update.LastDeploymentChangeTime.Format(time.RFC3339)
	}

	// Add deadline if present
	if update.Deadline != nil {
		metadata["deadline"] = update.Deadline.Format(time.RFC3339)
	}

	// Add flags
	if update.IsMandatory {
		metadata["is_mandatory"] = true
	}
	if update.IsBeta {
		metadata["is_beta"] = true
	}
	if update.IsDownloaded {
		metadata["is_downloaded"] = true
	}

	// Add more info URLs
	if len(update.MoreInfoUrls) > 0 {
		metadata["more_info_urls"] = update.MoreInfoUrls
	}

	// Add release notes
	if update.ReleaseNotes != "" {
		metadata["release_notes"] = update.ReleaseNotes
	}

	// Add support URL
	if update.SupportUrl != "" {
		metadata["support_url"] = update.SupportUrl
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
		CurrentVersion:     currentVersion,
		AvailableVersion:   availableVersion,
		Severity:           severity,
		RepositorySource:   "Microsoft Update",
		Metadata:           metadata,
	}

	// Add KB articles to CVE list field if present
	if len(kbArticles) > 0 {
		updateItem.KBID = strings.Join(kbArticles, ", ")
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

// mapMsrcSeverity maps Microsoft's MSRC severity ratings to our severity levels
func (s *WindowsUpdateScannerWUA) mapMsrcSeverity(msrcSeverity string) string {
	switch strings.ToLower(strings.TrimSpace(msrcSeverity)) {
	case "critical":
		return "critical"
	case "important":
		return "critical"
	case "moderate":
		return "moderate"
	case "low":
		return "low"
	case "unspecified", "":
		return ""
	default:
		return ""
	}
}

// parseVersionFromTitle attempts to extract current and available version from update title
// Examples:
//   "Intel Corporation - Display - 26.20.100.7584" -> ("Unknown", "26.20.100.7584")
//   "2024-01 Cumulative Update for Windows 11 Version 22H2 (KB5034123)" -> ("Unknown", "KB5034123")
func (s *WindowsUpdateScannerWUA) parseVersionFromTitle(title string) (currentVersion, availableVersion string) {
	currentVersion = "Unknown"
	availableVersion = "Unknown"

	// Pattern 1: Version at the end after last dash (common for drivers)
	// Example: "Intel Corporation - Display - 26.20.100.7584"
	if strings.Contains(title, " - ") {
		parts := strings.Split(title, " - ")
		lastPart := strings.TrimSpace(parts[len(parts)-1])

		// Check if last part looks like a version (contains dots and digits)
		if strings.Contains(lastPart, ".") && s.containsDigits(lastPart) {
			availableVersion = lastPart
			return
		}
	}

	// Pattern 2: KB article in parentheses
	// Example: "2024-01 Cumulative Update (KB5034123)"
	if strings.Contains(title, "(KB") && strings.Contains(title, ")") {
		start := strings.Index(title, "(KB")
		end := strings.Index(title[start:], ")")
		if end > 0 {
			kbNumber := title[start+1 : start+end]
			availableVersion = kbNumber
			return
		}
	}

	// Pattern 3: Date-based versioning
	// Example: "2024-01 Security Update"
	if strings.Contains(title, "202") { // Year pattern
		words := strings.Fields(title)
		for _, word := range words {
			// Look for YYYY-MM pattern
			if len(word) == 7 && word[4] == '-' && s.containsDigits(word[:4]) && s.containsDigits(word[5:]) {
				availableVersion = word
				return
			}
		}
	}

	// Pattern 4: Version keyword followed by number
	// Example: "Feature Update to Windows 11, version 23H2"
	lowerTitle := strings.ToLower(title)
	if strings.Contains(lowerTitle, "version ") {
		idx := strings.Index(lowerTitle, "version ")
		afterVersion := title[idx+8:]
		words := strings.Fields(afterVersion)
		if len(words) > 0 {
			// Take the first word after "version"
			versionStr := strings.TrimRight(words[0], ",.")
			availableVersion = versionStr
			return
		}
	}

	return
}

// containsDigits checks if a string contains any digits
func (s *WindowsUpdateScannerWUA) containsDigits(str string) bool {
	for _, char := range str {
		if char >= '0' && char <= '9' {
			return true
		}
	}
	return false
}