//go:build !linux

package system

import "fmt"

func getServicesSnapshot() (*ServiceSnapshot, error) {
	return nil, fmt.Errorf("service inventory not supported on this platform")
}
