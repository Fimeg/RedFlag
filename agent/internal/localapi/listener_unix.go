//go:build !windows

package localapi

import (
	"fmt"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"strconv"

	"github.com/Fimeg/RedFlag/agent/internal/constants"
)

const unixSocketFile = "redflag-agent.sock"

func defaultGroupName() string {
	return DefaultUnixGroupName
}

func defaultUnixSocketPath() string {
	return filepath.Join(constants.GetBaseDir(), constants.AgentDir, "localapi", unixSocketFile)
}

func listen(opts Options) (net.Listener, string, error) {
	path := unixSocketPath(opts)
	group := groupName(opts)

	gid, err := lookupGroupID(group)
	if err != nil {
		return nil, "", formatGroupMissing(group, err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, "", fmt.Errorf("localapi: create socket directory %q: %w", dir, err)
	}
	if err := os.Chown(dir, -1, gid); err != nil {
		return nil, "", fmt.Errorf("localapi: chown socket directory %q to group %q: %w", dir, group, err)
	}
	if err := os.Chmod(dir, 0o750); err != nil {
		return nil, "", fmt.Errorf("localapi: chmod socket directory %q: %w", dir, err)
	}

	if err := removeStaleSocket(path); err != nil {
		return nil, "", err
	}

	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, "", fmt.Errorf("localapi: listen on unix socket %q: %w", path, err)
	}

	if err := os.Chown(path, -1, gid); err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return nil, "", fmt.Errorf("localapi: chown socket %q to group %q: %w", path, group, err)
	}
	if err := os.Chmod(path, 0o660); err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return nil, "", fmt.Errorf("localapi: chmod socket %q: %w", path, err)
	}

	return &cleanupListener{Listener: ln, path: path}, path, nil
}

func lookupGroupID(groupName string) (int, error) {
	group, err := user.LookupGroup(groupName)
	if err != nil {
		return 0, err
	}
	gid, err := strconv.Atoi(group.Gid)
	if err != nil {
		return 0, fmt.Errorf("parse gid %q: %w", group.Gid, err)
	}
	return gid, nil
}

func removeStaleSocket(path string) error {
	info, err := os.Lstat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("localapi: stat socket path %q: %w", path, err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("localapi: refusing to replace non-socket path %q", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("localapi: remove stale socket %q: %w", path, err)
	}
	return nil
}

type cleanupListener struct {
	net.Listener
	path string
}

func (l *cleanupListener) Close() error {
	err := l.Listener.Close()
	if removeErr := os.Remove(l.path); err == nil && removeErr != nil && !os.IsNotExist(removeErr) {
		err = removeErr
	}
	return err
}
