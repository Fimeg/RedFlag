//go:build windows
// +build windows

package scanner

// WindowsUpdateScanner is an alias for WindowsUpdateScannerWUA on Windows
// This allows the WUA implementation to be used seamlessly
type WindowsUpdateScanner = WindowsUpdateScannerWUA

// NewWindowsUpdateScanner returns the WUA-based scanner on Windows
func NewWindowsUpdateScanner() *WindowsUpdateScanner {
	return NewWindowsUpdateScannerWUA()
}