package bruteutils

import (
	"context"
	"crypto/tls"
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

// DialContext 拨号并执行协议握手前的准备。
//
// 历史缺陷（已修复）：旧实现 TLS 优先——对明文服务先发送 TLS ClientHello
// 再回退重连。DialX 的 TLS 重试会对目标连续多次发送握手字节，明文服务
// （Redis/Mock 等）收到垃圾数据后进入不可用状态，导致后续所有认证尝试
// 间歇性失败（CI 的 TestGRPCMUSTPASS_Brute 即此问题）。
// 现在默认纯 TCP：本包内所有调用方（redis/telnet/rtsp/rdp/vnc/ssh/smtp/
// imap/ftp）都是先发协议明文握手，TLS-first 从来就是错误行为。
func (d *NetXDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	return d.DialTCPContext(ctx, network, address)
}

// DialTLSContext 显式 TLS 拨号（需要 TLS 的调用方使用，不隐式回退）。
// 证书校验按模块约定跳过（弱口令探测场景）。
func (d *NetXDialer) DialTLSContext(ctx context.Context, network, address string) (net.Conn, error) {
	conn, err := netx.DialTLSTimeout(defaultTimeout, address, &tls.Config{InsecureSkipVerify: true}) // #nosec G402 -- 弱口令探测约定
	if err != nil {
		return nil, utils.Wrap(dialError, err.Error())
	}
	return conn, nil
}
