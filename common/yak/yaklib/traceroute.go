package yaklib

import (
	"github.com/yaklang/yaklang/common/utils/pingutil"
)

var TracerouteExports = map[string]interface{}{
	"Diagnostic": func(host string, opts ...pingutil.TracerouteConfigOption) (chan *pingutil.TracerouteResponse, error) {
		return pingutil.Traceroute(host, opts...)
	},
	"ctx": pingutil.WithCtx,
	"timeout": func(timeout float64) pingutil.TracerouteConfigOption {
		return func(cfg *pingutil.TracerouteConfig) {
			pingutil.WithReadTimeout(timeout)(cfg)
			pingutil.WithWriteTimeout(timeout)(cfg)
		}
	},
	"hops":     pingutil.WithMaxHops,
	"protocol": pingutil.WithProtocol,
	"retry":    pingutil.WithRetryTimes,
	"localIp":  pingutil.WithLocalAddr,
	"udpPort":  pingutil.WithUdpPort,
	"firstTTL": pingutil.WithFirstTTL,
}
