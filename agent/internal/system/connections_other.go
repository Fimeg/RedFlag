//go:build !linux

package system

import "fmt"

func getConnectionsSnapshot() (*ConnectionSnapshot, error) {
	return nil, fmt.Errorf("connection inventory not supported on this platform")
}
