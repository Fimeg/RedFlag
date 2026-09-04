//go:build windows

package localapi

import (
	"fmt"
	"net"

	winio "github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
)

func defaultGroupName() string {
	return DefaultWindowsGroupName
}

func defaultUnixSocketPath() string {
	return ""
}

func listen(opts Options) (net.Listener, string, error) {
	pipeName := windowsPipeName(opts)
	group := groupName(opts)

	groupSID, err := lookupGroupSID(group)
	if err != nil {
		return nil, "", formatGroupMissing(group, err)
	}

	cfg := &winio.PipeConfig{
		// LocalSystem and Administrators get full access; the RedFlag local UI
		// group gets read/write transport rights so it can issue HTTP GETs and
		// receive responses over the pipe.
		SecurityDescriptor: fmt.Sprintf("D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GRGW;;;%s)", groupSID),
	}
	ln, err := winio.ListenPipe(pipeName, cfg)
	if err != nil {
		return nil, "", fmt.Errorf("localapi: listen on named pipe %q: %w", pipeName, err)
	}
	return ln, pipeName, nil
}

func lookupGroupSID(groupName string) (string, error) {
	sid, _, accountType, err := windows.LookupSID("", groupName)
	if err != nil {
		return "", err
	}
	switch accountType {
	case windows.SidTypeGroup, windows.SidTypeAlias, windows.SidTypeWellKnownGroup:
		return sid.String(), nil
	default:
		return "", fmt.Errorf("account %q has unexpected SID type %d", groupName, accountType)
	}
}
