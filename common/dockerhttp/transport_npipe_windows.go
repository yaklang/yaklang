//go:build windows

package dockerhttp

import (
	"context"
	"net"
	"strings"

	"github.com/Microsoft/go-winio"
)

// Reuse the repository's existing Windows pipe transport for cancellable I/O
// and deadlines instead of maintaining raw overlapped syscall handling.
func npipeDialContext(addr string) (func(context.Context, string, string) (net.Conn, error), error) {
	path := strings.ReplaceAll(addr, "/", `\`)
	return func(ctx context.Context, _, _ string) (net.Conn, error) {
		return winio.DialPipeContext(ctx, path)
	}, nil
}
