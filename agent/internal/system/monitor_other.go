//go:build !linux

package system

import "fmt"

func readRawMonitorSample() (*rawMonitorSample, error) {
	return nil, fmt.Errorf("%w on this platform", ErrMonitorUnavailable)
}
