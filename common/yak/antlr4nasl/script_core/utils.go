package script_core

import (
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/pingutil"
	"time"
)

func Ping(ctx *ExecContext) {
	ok := false
	defer func() {
		if ok {
			ctx.Kbs.SetKB("Host/dead", 0)
			ctx.Kbs.SetKB("Host/State", "up")
		} else {
			//ping检测不存活 或排除打印机设备时会标注为dead
			ctx.Kbs.SetKB("Host/dead", 1)
			ctx.Kbs.SetKB("Host/State", "down")
		}
	}()
	target := ctx.Host
	tcpPingPort := "80,443,22,3389"
	timeout := 3 * time.Second
	dnsTimeout := 3 * time.Second
	proxies := ctx.Proxies
	if utils.IsIPv4(target) || utils.IsIPv6(target) {
		ok = pingutil.PingAuto(target, pingutil.WithDefaultTcpPort(tcpPingPort), pingutil.WithTimeout(timeout), pingutil.WithProxies(proxies...)).Ok
	} else {
		result := netx.LookupFirst(target, netx.WithTimeout(dnsTimeout))
		if result != "" && (utils.IsIPv4(result) || utils.IsIPv6(result)) {
			ok = pingutil.PingAuto(result, pingutil.WithDefaultTcpPort(tcpPingPort), pingutil.WithTimeout(timeout), pingutil.WithProxies(proxies...)).Ok
		}
	}
}
