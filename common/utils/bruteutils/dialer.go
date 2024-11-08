package bruteutils

import (
	"context"
	"github.com/yaklang/yaklang/common/gmsm/gmtls"
	"net"
	"time"

	"github.com/yaklang/yaklang/common/netx"
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
		return nil, err
	}
	return conn, nil
}

func (d *NetXDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var (
		conn net.Conn
		err  error
	)
	// tls first
	conn, err = netx.DialTLSTimeout(defaultTimeout, address, &gmtls.Config{InsecureSkipVerify: true})
	if err == nil {
		return conn, nil
	}

	return d.DialTCPContext(ctx, network, address)
}
