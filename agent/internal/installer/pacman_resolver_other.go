//go:build !linux

package installer

import "fmt"

func ResolvePacmanClosure(packageName, version string) (*PacmanResolution, error) {
	return nil, fmt.Errorf("pacman resolution is only available on Linux")
}
