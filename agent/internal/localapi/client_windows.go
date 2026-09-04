//go:build windows

package localapi

import (
	"context"
	"net"

	winio "github.com/Microsoft/go-winio"
)

func platformDialLocalContext(opts ClientOptions) func(context.Context, string, string) (net.Conn, error) {
	pipeName := opts.WindowsPipeName
	if pipeName == "" {
		pipeName = DefaultWindowsPipeName
	}

	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		return winio.DialPipeContext(ctx, pipeName)
	}
}
