//go:build windows

package instancelock

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/windows"
)

var (
	// mu guards handle — the OS mutex handle must be closed exactly once,
	// but it must outlive the caller's release function (it's process-wide).
	mu     sync.Mutex
	handle windows.Handle
)

// lockName is the well-known kernel-object name. The Global\ prefix makes
// it visible across all sessions (including the service session), which is
// necessary because the agent may run both as a service and as a console
// process under different sessions on the same host.
const lockName = `Global\RedFlagAgent_v1`

// acquireLock opens a named kernel mutex and attempts to acquire it
// without blocking. If the mutex is held by another process, we fail
// immediately. If it doesn't exist, CreateMutex creates it for us.
//
// Unlike the Unix flock (which lives on an fd scoped to the file system
// path), a Windows mutex is purely a kernel object — there is no file
// to write a PID or leak. The name is the key: any process on the system
// (service, console, WSL bridge) that opens the same name gets the same
// mutex object.
func acquireLock() (func(), error) {
	mu.Lock()
	defer mu.Unlock()

	// If we already hold the lock in this process (shouldn't happen in
	// normal use, but guard it), don't re-acquire.
	if handle != 0 {
		return noopRelease, nil
	}

	name, err := windows.UTF16PtrFromString(lockName)
	if err != nil {
		return nil, fmt.Errorf("instancelock: utf16: %w", err)
	}

	// CreateMutex opens or creates the named mutex. It does NOT set the
	// initial ownership — we do that with WaitForSingleObject below.
	h, err := windows.CreateMutex(nil, false, name)
	if err != nil {
		return nil, fmt.Errorf("instancelock: CreateMutex: %w", err)
	}

	// Attempt to acquire with zero timeout (non-blocking).
	// WAIT_OBJECT_0  (0)   = we got it.
	// WAIT_ABANDONED (128) = previous owner died holding it — it's ours.
	// WAIT_TIMEOUT   (258) = someone else has it.
	switch waitResult, _ := windows.WaitForSingleObject(h, 0); waitResult {
	case 0, windows.WAIT_ABANDONED:
		handle = h
	case 258: // WAIT_TIMEOUT
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("instancelock: another agent process is already running on this host (locked mutex: %s)", lockName)
	default:
		_ = windows.CloseHandle(h)
		return nil, fmt.Errorf("instancelock: WaitForSingleObject failed on %s", lockName)
	}

	// Release is called on graceful shutdown. If the process crashes or
	// is killed, the kernel releases the mutex automatically — unlike
	// Unix where flock is tied to the fd and the fd closes on exit, a
	// Windows mutex held by a dead thread transitions to "abandoned"
	// and the next waiter acquires it cleanly.
	release := func() {
		mu.Lock()
		defer mu.Unlock()
		if handle != 0 {
			_ = windows.ReleaseMutex(handle)
			_ = windows.CloseHandle(handle)
			handle = 0
		}
	}

	// Also create a per-user lockfile under APPDATA to catch the case
	// where the same user runs two console-mode agents without going
	// through the SCM. The kernel mutex covers cross-session; the file
	// lock covers same-user-duplicates where both instances open the
	// same file.
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		lockFilePath := filepath.Join(localAppData, "RedFlag", "agent.lock")

		// Best-effort: if we can't write it, the kernel mutex still
		// protects us across all sessions.
		_ = os.MkdirAll(filepath.Dir(lockFilePath), 0755)
		if f, fErr := os.OpenFile(lockFilePath, os.O_RDWR|os.O_CREATE, 0644); fErr == nil {
			if lErr := lockFile(f); lErr == nil {
				// Extend release to also close the file.
				prevRelease := release
				release = func() {
					_ = f.Close()
					prevRelease()
				}
			} else {
				_ = f.Close()
			}
		}
	}

	return release, nil
}

// lockFile takes an advisory lock on an *os.File using LockFileEx (the
// Windows equivalent of flock). This is belt-and-suspenders with the
// kernel mutex — both must be acquired for the lock to count.
func lockFile(f *os.File) error {
	ol := &windows.Overlapped{}
	return windows.LockFileEx(
		windows.Handle(f.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, ol,
	)
}
