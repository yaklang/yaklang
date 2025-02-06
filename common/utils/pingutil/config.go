package pingutil

import (
	"context"
	"net"
	"time"
)

type PingConfig struct {
	Ctx            context.Context
	defaultTcpPort string
	timeout        time.Duration
	proxies        []string

	// for test
	pingNativeHandler func(ip string, timeout time.Duration) *PingResult
	tcpDialHandler    func(ctx context.Context, addr string, proxies ...string) (net.Conn, error)
	forceTcpPing      bool
	RetryCount        int           // 发包重试次数
	RetryInterval     time.Duration // 发包间隔
}

func NewPingConfig() *PingConfig {
	return &PingConfig{
		Ctx:            context.Background(),
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

func WithTimeout(timeout time.Duration) PingConfigOpt {
	return func(cfg *PingConfig) {
		cfg.timeout = timeout
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
