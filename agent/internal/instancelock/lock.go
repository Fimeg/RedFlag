// Package instancelock prevents multiple agent processes from running
// concurrently on the same host by holding an exclusive lock on a
// well-known path (Unix: flock on /var/lib/.../agent.lock; Windows:
// named kernel mutex Global\RedFlagAgent_v1 + per-user file lock).
// The lock is released when the process exits (the OS cleans up).
// If the lock cannot be acquired, the instance should exit immediately
// rather than share config.json and renewal state.
package instancelock

import (
	"path/filepath"
)

// LockPath returns the well-known path for the Unix instance lockfile.
// Windows uses a named kernel mutex instead; this function is only called
// from lock_unix.go and is referenced here so the package compiles on
// Windows.
func LockPath() string {
	return filepath.Join("/var/lib/redflag/agent/state", "agent.lock")
}

// Acquire attempts to acquire an exclusive instance lock. It returns a
// release function and nil on success, or an error if another agent
// process is already running on this host.
//
// The release function must be called on graceful shutdown.
// On process crash the OS releases the lock automatically:
//   - Unix: the kernel closes the flock fd on process exit.
//   - Windows: the kernel transitions the mutex to "abandoned" state;
//     a crashed owner's mutex is acquired cleanly by the next waiter.
func Acquire() (release func(), err error) {
	return acquireLock()
}

// noopRelease is a safe no-op for platforms that don't need cleanup.
func noopRelease() {}

