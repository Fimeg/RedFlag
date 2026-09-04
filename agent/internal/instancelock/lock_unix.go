//go:build linux || darwin || freebsd

package instancelock

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// acquireLock opens (or creates) the lockfile and takes an exclusive
// flock. The lock is released when the process exits (the kernel
// closes the fd, which releases the flock).
func acquireLock() (func(), error) {
	path := LockPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("instancelock: mkdir %s: %w", dir, err)
	}

	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0644)
	if err != nil {
		return nil, fmt.Errorf("instancelock: open %s: %w", path, err)
	}

	fd := int(f.Fd())
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return nil, fmt.Errorf("instancelock: %s is locked by another process: %w", path, err)
	}

	// Write our PID so operators can see who holds the lock.
	_, _ = f.WriteAt([]byte(fmt.Sprintf("%d\n", os.Getpid())), 0)
	_ = f.Truncate(64)

	release := func() {
		_ = f.Close()
		// Don't remove the file — leaving it is harmless and prevents a
		// TOCTOU race where a fresh open might get an unlocked fd before
		// we flock it.
	}

	return release, nil
}
