package installer

import (
	"fmt"
	"log"
	"runtime"
	"strings"
	"time"

	"github.com/Fimeg/RedFlag/agent/pkg/windowsupdate"
	"github.com/go-ole/go-ole"
	"github.com/scjalliance/comshim"
)

// Windows Update Agent operation result codes.
// https://learn.microsoft.com/en-us/windows/win32/api/wuapi/ne-wuapi-operationresultcode
const (
	orcSucceeded           int32 = 2
	orcSucceededWithErrors int32 = 3
)

// WindowsUpdateInstaller installs Windows updates through the Windows Update
// Agent COM API (Microsoft.Update.Session) via go-ole — the same binding the WUA
// scanner uses. No shelling out: wuauclt's /updatenow was removed on Windows 10,
// and Install-WindowsUpdate needs the third-party PSWindowsUpdate module. The COM
// binding compiles cross-platform (go-ole ships non-Windows stubs); IsAvailable()
// gates execution to Windows at runtime.
type WindowsUpdateInstaller struct{}

// NewWindowsUpdateInstaller creates a new Windows Update installer
func NewWindowsUpdateInstaller() *WindowsUpdateInstaller {
	return &WindowsUpdateInstaller{}
}

// IsAvailable reports whether this installer can run on the current host.
func (i *WindowsUpdateInstaller) IsAvailable() bool {
	return runtime.GOOS == "windows"
}

// GetPackageType returns the package type this installer handles
func (i *WindowsUpdateInstaller) GetPackageType() string {
	return "windows_update"
}

// Install installs a specific Windows update by title.
func (i *WindowsUpdateInstaller) Install(packageName string) (*InstallResult, error) {
	return i.installUpdates([]string{packageName}, false)
}

// InstallMultiple installs multiple Windows updates by title.
func (i *WindowsUpdateInstaller) InstallMultiple(packageNames []string) (*InstallResult, error) {
	return i.installUpdates(packageNames, false)
}

// Upgrade installs every available Windows update.
func (i *WindowsUpdateInstaller) Upgrade() (*InstallResult, error) {
	return i.installUpdates(nil, true)
}

// UpdatePackage updates a specific Windows update (alias for Install).
func (i *WindowsUpdateInstaller) UpdatePackage(packageName string) (*InstallResult, error) {
	return i.Install(packageName)
}

// DryRun reports which updates would be installed for the given title without
// installing anything.
func (i *WindowsUpdateInstaller) DryRun(packageName, version string) (*InstallResult, error) {
	return i.installUpdates([]string{packageName}, true)
}

// VerifyHash is a no-op for Windows Update: the WUA validates update payloads
// against Microsoft's signed catalog itself, and individual updates expose no
// addressable download URL to hash. Fail-open by design.
func (i *WindowsUpdateInstaller) VerifyHash(packageName, version, expectedSHA256 string) error {
	log.Printf("[INFO] [agent] [installer] hash_verification_skipped package=%s reason=windows_update_uses_wua_verification", packageName)
	return nil
}

// installUpdates searches the Windows Update Agent for the requested updates and
// runs the download -> accept-EULA -> install lifecycle through the COM API.
// packageNames are matched against update titles (the scanner reports Title as the
// package name); a nil/empty slice means "all available updates" (upgrade).
func (i *WindowsUpdateInstaller) installUpdates(packageNames []string, isDryRun bool) (*InstallResult, error) {
	if !i.IsAvailable() {
		return nil, fmt.Errorf("Windows Update installer is only available on Windows")
	}

	startTime := time.Now()
	action := "install"
	if len(packageNames) == 0 {
		action = "upgrade"
	}

	result := &InstallResult{
		Success:           false,
		IsDryRun:          isDryRun,
		Action:            action,
		PackagesInstalled: []string{},
		Dependencies:      []string{},
	}

	// Initialize COM (mirror the WUA scanner's pattern).
	comshim.Add(1)
	defer comshim.Done()
	ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED|ole.COINIT_SPEED_OVER_MEMORY)
	defer ole.CoUninitialize()

	session, err := windowsupdate.NewUpdateSession()
	if err != nil {
		return i.fail(result, startTime, fmt.Errorf("create Windows Update session: %w", err))
	}

	searcher, err := session.CreateUpdateSearcher()
	if err != nil {
		return i.fail(result, startTime, fmt.Errorf("create update searcher: %w", err))
	}

	searchResult, err := searcher.Search("IsInstalled=0 AND IsHidden=0")
	if err != nil {
		return i.fail(result, startTime, fmt.Errorf("search for updates: %w", err))
	}

	selected := selectUpdates(searchResult.Updates, packageNames)
	if len(selected) == 0 {
		if len(packageNames) == 0 {
			// Nothing to upgrade — a clean no-op, not a failure.
			result.Success = true
			result.Stdout = "No applicable Windows updates available"
			result.DurationSeconds = int(time.Since(startTime).Seconds())
			return result, nil
		}
		return i.fail(result, startTime,
			fmt.Errorf("requested update(s) not found among available Windows updates: %v", packageNames))
	}

	if isDryRun {
		result.Success = true
		result.Stdout = formatSelected(selected)
		result.PackagesInstalled = updateTitles(selected) // what WOULD be installed
		result.DurationSeconds = int(time.Since(startTime).Seconds())
		return result, nil
	}

	// Accept EULAs where required before download/install.
	for _, u := range selected {
		if !u.EulaAccepted {
			if err := u.AcceptEula(); err != nil {
				return i.fail(result, startTime, fmt.Errorf("accept EULA for %q: %w", u.Title, err))
			}
		}
	}

	// Download any updates not already cached.
	downloader, err := session.CreateUpdateDownloader()
	if err != nil {
		return i.fail(result, startTime, fmt.Errorf("create update downloader: %w", err))
	}
	dlResult, err := downloader.Download(selected)
	if err != nil {
		return i.fail(result, startTime, fmt.Errorf("download updates: %w", err))
	}
	if !wuaSucceeded(dlResult.ResultCode) {
		return i.fail(result, startTime,
			fmt.Errorf("Windows Update download failed: ResultCode=%d HResult=0x%08X", dlResult.ResultCode, uint32(dlResult.HResult)))
	}

	// Install.
	inst, err := session.CreateUpdateInstaller()
	if err != nil {
		return i.fail(result, startTime, fmt.Errorf("create update installer: %w", err))
	}
	instResult, err := inst.Install(selected)
	if err != nil {
		return i.fail(result, startTime, fmt.Errorf("install updates: %w", err))
	}
	if !wuaSucceeded(instResult.ResultCode) {
		return i.fail(result, startTime,
			fmt.Errorf("Windows Update install failed: ResultCode=%d HResult=0x%08X", instResult.ResultCode, uint32(instResult.HResult)))
	}

	result.Success = true
	result.PackagesInstalled = updateTitles(selected)
	result.RebootRequired = instResult.RebootRequired
	result.Stdout = fmt.Sprintf("Installed %d Windows update(s): %s", len(selected), strings.Join(updateTitles(selected), "; "))
	result.DurationSeconds = int(time.Since(startTime).Seconds())

	log.Printf("[INFO] [agent] [installer] windows_update_installed packages=%v reboot_required=%v duration=%ds",
		result.PackagesInstalled, result.RebootRequired, result.DurationSeconds)

	return result, nil
}

// GetPendingUpdates returns the titles of updates the Windows Update Agent reports
// as applicable but not yet installed.
func (i *WindowsUpdateInstaller) GetPendingUpdates() ([]string, error) {
	if !i.IsAvailable() {
		return nil, fmt.Errorf("Windows Update installer is only available on Windows")
	}

	comshim.Add(1)
	defer comshim.Done()
	ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED|ole.COINIT_SPEED_OVER_MEMORY)
	defer ole.CoUninitialize()

	session, err := windowsupdate.NewUpdateSession()
	if err != nil {
		return nil, fmt.Errorf("create Windows Update session: %w", err)
	}
	searcher, err := session.CreateUpdateSearcher()
	if err != nil {
		return nil, fmt.Errorf("create update searcher: %w", err)
	}
	searchResult, err := searcher.Search("IsInstalled=0 AND IsHidden=0")
	if err != nil {
		return nil, fmt.Errorf("search for updates: %w", err)
	}
	return updateTitles(searchResult.Updates), nil
}

// fail finalizes a failed InstallResult: records the error, stamps duration, and
// returns it alongside the error so the caller reports ground truth (no fake success).
func (i *WindowsUpdateInstaller) fail(result *InstallResult, start time.Time, err error) (*InstallResult, error) {
	result.Success = false
	result.ErrorMessage = err.Error()
	result.Stderr = err.Error()
	result.ExitCode = 1
	result.DurationSeconds = int(time.Since(start).Seconds())
	return result, err
}

// selectUpdates picks the updates to act on. An empty names slice selects every
// available update (upgrade-all). Otherwise an update is selected when a requested
// name matches its title (exact, then case-insensitive contains) or one of its KB
// article IDs.
func selectUpdates(available []*windowsupdate.IUpdate, names []string) []*windowsupdate.IUpdate {
	if len(names) == 0 {
		return available
	}
	var selected []*windowsupdate.IUpdate
	for _, u := range available {
		if updateMatchesAny(u, names) {
			selected = append(selected, u)
		}
	}
	return selected
}

func updateMatchesAny(u *windowsupdate.IUpdate, names []string) bool {
	title := strings.TrimSpace(u.Title)
	lowerTitle := strings.ToLower(title)
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if title == name || strings.Contains(lowerTitle, strings.ToLower(name)) {
			return true
		}
		for _, kb := range u.KBArticleIDs {
			// KB IDs come back without the "KB" prefix; match either form.
			if strings.EqualFold(kb, name) || strings.EqualFold("KB"+kb, name) {
				return true
			}
		}
	}
	return false
}

func updateTitles(updates []*windowsupdate.IUpdate) []string {
	titles := make([]string, 0, len(updates))
	for _, u := range updates {
		titles = append(titles, u.Title)
	}
	return titles
}

func formatSelected(updates []*windowsupdate.IUpdate) string {
	var b strings.Builder
	b.WriteString("Dry run - the following Windows updates would be installed:\n")
	for _, u := range updates {
		fmt.Fprintf(&b, "  - %s", u.Title)
		if len(u.KBArticleIDs) > 0 {
			fmt.Fprintf(&b, " (KB%s)", strings.Join(u.KBArticleIDs, ", KB"))
		}
		b.WriteString("\n")
	}
	return b.String()
}

func wuaSucceeded(resultCode int32) bool {
	return resultCode == orcSucceeded || resultCode == orcSucceededWithErrors
}
