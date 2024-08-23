package pingutil

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"net"
	"time"
)

type PingConfig struct {
	defaultTcpPort string
	timeout        time.Duration
	proxies        []string

	// for test
	pingNativeHandler func(ip string, timeout time.Duration) *PingResult
	tcpDialHandler    func(ctx context.Context, addr string, proxies ...string) (net.Conn, error)
}

func NewPingConfig() *PingConfig {
	return &PingConfig{
		timeout:        5 * time.Second,
		defaultTcpPort: "22,80,443",
	}
}

func WithPingNativeHandler(f func(ip string, timeout time.Duration) *PingResult) func(*PingConfig) {
	return func(cfg *PingConfig) {
		cfg.pingNativeHandler = f
	}
}

func WithTcpDialHandler(f func(ctx context.Context, addr string, proxies ...string) (net.Conn, error)) func(*PingConfig) {
	return func(cfg *PingConfig) {
		cfg.tcpDialHandler = f
	}
}

func WithTimeout(timeout float64) func(*PingConfig) {
	return func(cfg *PingConfig) {
		cfg.timeout = utils.FloatSecondDuration(timeout)
	}
}

func WithProxies(proxies ...string) func(*PingConfig) {
	return func(cfg *PingConfig) {
		cfg.proxies = proxies
	}
}

func WithDefaultTcpPort(port string) func(*PingConfig) {
	return func(cfg *PingConfig) {
		cfg.defaultTcpPort = port
	}
}
