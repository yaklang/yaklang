package bruteutils

import (
	"context"
	"net"

	"github.com/yaklang/yaklang/common/netx"
)


type NetXDialer struct {
}

var defaultDialer = &NetXDialer{}

func (d *NetXDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return netx.DialContext(ctx, address)
}