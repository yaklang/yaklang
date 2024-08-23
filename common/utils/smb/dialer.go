package smb

import (
	"context"
	"net"
)

type DialerContext interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}
