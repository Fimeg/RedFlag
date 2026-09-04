//go:build windows
// +build windows

package scanner

// WindowsUpdateScanner is an alias for WindowsUpdateScannerWUA on Windows
// This aliases to the WUA implementation on Windows builds
type WindowsUpdateScanner = WindowsUpdateScannerWUA

// NewWindowsUpdateScanner returns the WUA-based scanner on Windows
func NewWindowsUpdateScanner() *WindowsUpdateScanner {
	return NewWindowsUpdateScannerWUA()
}