package pingutil

import (
	"context"
	"net"
	"time"
)

type PingConfig struct {
	Ctx                       context.Context
	defaultTcpPort            string
	timeout                   time.Duration
	linkAddressResolveTimeout time.Duration
	proxies                   []string

	// for test
	pingNativeHandler func(ip string, timeout time.Duration) *PingResult
	tcpDialHandler    func(ctx context.Context, addr string, proxies ...string) (net.Conn, error)
	forceTcpPing      bool
}

func NewPingConfig() *PingConfig {
	return &PingConfig{
		Ctx:                       context.Background(),
		timeout:                   5 * time.Second,
		defaultTcpPort:            "22,80,443",
		linkAddressResolveTimeout: 2 * time.Second,
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

func WithTimeout(timeout time.Duration) PingConfigOpt {
	return func(cfg *PingConfig) {
		cfg.timeout = timeout
	}
}

func WithLinkResolveTimeout(timeout time.Duration) PingConfigOpt {
	return func(cfg *PingConfig) {
		cfg.linkAddressResolveTimeout = timeout
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
