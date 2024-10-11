package pingutil

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"time"
)

type PingConfig struct {
	Ctx 		  context.Context
	defaultTcpPort string
	timeout        time.Duration
	proxies        []string

	// for test
	pingNativeHandler func(ip string, timeout time.Duration) *PingResult
	tcpDialHandler    func(ctx context.Context, addr string, proxies ...string) (net.Conn, error)
	forceTcpPing      bool
}

func NewPingConfig() *PingConfig {
	return &PingConfig{
		Ctx :           context.Background(),
		timeout:        5 * time.Second,
		defaultTcpPort: "22,80,443",
	}
}

type PingConfigOpt func(*PingConfig)

func WithPingContext(ctx context.Context) PingConfigOpt {
	return func(cfg *PingConfig) {
		cfg.Ctx = ctx
	}
}

func WithForceTcpPing() PingConfigOpt {
	return func(cfg *PingConfig) {
		cfg.forceTcpPing = true
	}
}

func WithPingNativeHandler(f func(ip string, timeout time.Duration) *PingResult) PingConfigOpt {
	return func(cfg *PingConfig) {
		cfg.pingNativeHandler = f
	}
}

func WithTcpDialHandler(f func(ctx context.Context, addr string, proxies ...string) (net.Conn, error)) PingConfigOpt {
	return func(cfg *PingConfig) {
		cfg.tcpDialHandler = f
	}
}

func WithTimeout(timeout any) PingConfigOpt {
	return func(cfg *PingConfig) {
		switch v := timeout.(type) {
		case float64:
			cfg.timeout = utils.FloatSecondDuration(v)
		case int:
			cfg.timeout = utils.FloatSecondDuration(float64(v))
		case time.Duration:
			cfg.timeout = v
		default:
			cfg.timeout = 5 * time.Second
		}
	}
}

func WithProxies(proxies ...string) PingConfigOpt {
	return func(cfg *PingConfig) {
		cfg.proxies = proxies
	}
}

func WithDefaultTcpPort(port string) PingConfigOpt {
	return func(cfg *PingConfig) {
		cfg.defaultTcpPort = port
	}
}
