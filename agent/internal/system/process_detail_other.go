//go:build !linux
// +build !linux

package system

import "fmt"

func getFullProcessSnapshot() (*FullProcessSnapshot, error) {
	return nil, fmt.Errorf("process detail scanning not supported on this platform")
}

func getProcessDetail(pid int, caps ProcessCaps) (*FullProcess, error) {
	return nil, fmt.Errorf("process detail scanning not supported on this platform")
}
