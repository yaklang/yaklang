package bruteutils

import (
	"context"
	"crypto/tls"
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

func (d *NetXDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	var (
		conn net.Conn
		err  error
	)
	// tls first
	conn, err = netx.DialTLSTimeout(defaultTimeout, address, &tls.Config{InsecureSkipVerify: true})
	if err == nil {
		return conn, nil
	}

	conn, err = netx.DialContext(ctx, address)
	if err != nil {
		return nil, err
	}
	return conn, nil
}
