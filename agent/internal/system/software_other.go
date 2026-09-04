//go:build !linux

package system

import (
	"fmt"
	"time"
)

func getSoftwareSnapshot() (*SoftwareSnapshot, error) {
	return &SoftwareSnapshot{Supported: false, CollectedAt: time.Now().UTC()}, nil
}

func getPackageDetail(packageType, identity string) (*PackageDetail, error) {
	return nil, fmt.Errorf("software detail not supported on this platform")
}

func findSoftwareOwner(path string) (*SoftwareOwner, error) {
	return nil, fmt.Errorf("software ownership not supported on this platform")
}
