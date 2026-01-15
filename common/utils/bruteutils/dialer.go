package bruteutils

import (
	"context"
	"net"
	"time"

	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
)

const defaultTimeout = 10 * time.Second

type NetXDialer struct{}

var defaultDialer = &NetXDialer{}

func (d *NetXDialer) Dial(network, address string) (net.Conn, error) {
	return d.DialContext(context.Background(), network, address)
}

func (d *NetXDialer) DialTCPContext(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := netx.DialContext(ctx, addr)
	if err != nil {
		return nil, utils.Wrap(dialError, err.Error())
	}
	return conn, nil
}

func (d *NetXDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.DialTCPContext(ctx, network, address)
}
