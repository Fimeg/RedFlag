//go:build !windows

package localapi

import (
	"context"
	"net"
)

func platformDialLocalContext(opts ClientOptions) func(context.Context, string, string) (net.Conn, error) {
	path := opts.UnixSocketPath
	if path == "" {
		path = defaultUnixSocketPath()
	}

	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		var dialer net.Dialer
		return dialer.DialContext(ctx, "unix", path)
	}
}
