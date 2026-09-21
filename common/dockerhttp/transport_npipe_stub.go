//go:build !windows

package dockerhttp

import (
	"context"
	"fmt"
	"net"
)

// npipeDialContext is unavailable off Windows.
func npipeDialContext(addr string) (func(ctx context.Context, network, address string) (net.Conn, error), error) {
	return nil, fmt.Errorf("npipe protocol requires windows build (addr=%s); use unix or tcp", addr)
}
