package bruteutils

import (
	"context"
	"crypto/tls"
	"net"
	"time"

	"github.com/yaklang/yaklang/common/log"
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
	// tls first（证书校验按模块约定跳过）
	conn, err := netx.DialTLSTimeout(defaultTimeout, address, &tls.Config{InsecureSkipVerify: true}) // #nosec G402 -- 弱口令探测约定
	if err == nil {
		return conn, nil
	}
	// 显式记录 TLS→明文回退：不再静默降级（新探针在结果中记录传输方式，
	// 旧协议路径在此打日志，保证凭证发送方式可审计）。
	log.Debugf("brute dialer: TLS dial %s failed (%v), falling back to plaintext TCP", address, err)
	return d.DialTCPContext(ctx, network, address)
}
