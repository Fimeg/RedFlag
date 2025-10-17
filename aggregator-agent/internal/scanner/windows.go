//go:build !windows
// +build !windows

package scanner

import "github.com/aggregator-project/aggregator-agent/internal/client"

// WindowsUpdateScanner stub for non-Windows platforms
type WindowsUpdateScanner struct{}

// NewWindowsUpdateScanner creates a stub Windows scanner for non-Windows platforms
func NewWindowsUpdateScanner() *WindowsUpdateScanner {
	return &WindowsUpdateScanner{}
}

// IsAvailable always returns false on non-Windows platforms
func (s *WindowsUpdateScanner) IsAvailable() bool {
	return false
}

// Scan always returns no updates on non-Windows platforms
func (s *WindowsUpdateScanner) Scan() ([]client.UpdateReportItem, error) {
	return []client.UpdateReportItem{}, nil
}



