package finscan

import (
	"github.com/gopacket/gopacket/layers"
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/netutil"
	"net"
	"time"
)

type Config struct {
	// 发包必须的几个字段
	Iface     *net.Interface
	GatewayIP net.IP
	SourceIP  net.IP
	TcpSetter func(tcp *layers.TCP)
	TcpFilter func(tcp *layers.TCP) bool
	// Fetch Gateway Hardware Address TimeoutSeconds
	FetchGatewayHardwareAddressTimeout time.Duration
}

func NewDefaultConfig(extra ...ConfigOption) (*Config, error) {
	options, err := CreateConfigOptionsByTargetNetworkOrDomain("8.8.8.8", 5*time.Second)
	if err != nil {
		return nil, err
	}

	return NewConfig(append(options, extra...)...)
}

func NewConfig(options ...ConfigOption) (*Config, error) {
	config := &Config{
		FetchGatewayHardwareAddressTimeout: 5 * time.Second,
	}
	// from https://nmap.org/book/scan-methods-null-fin-xmas-scan.html
	// When scanning systems compliant with this RFC text, any packet not containing SYN, RST, or ACK bits will result
	// in a returned RST if the port is closed and no response at all if the port is open. As long as none of those three
	// bits are included, any combination of the other three (FIN, PSH, and URG) are OK.
	WithTcpSetter(func(tcp *layers.TCP) {
		tcp.FIN = true
		tcp.SYN = false
	})(config)
	WithTcpFilter(func(tcp *layers.TCP) bool {
		if tcp.RST && tcp.ACK && !tcp.SYN {
			return true
		}
		return false
	})(config)

	for _, option := range options {
		option(config)
	}

	if config.Iface == nil {
		return nil, errors.New("config default net.Interface failed: empty iface")
	}
	return config, nil
}

type ConfigOption func(config *Config)

func WithTcpSetter(setter func(tcp *layers.TCP)) ConfigOption {
	return func(config *Config) {
		config.TcpSetter = setter
	}
}
func WithTcpFilter(filter func(tcp *layers.TCP) bool) ConfigOption {
	return func(config *Config) {
		config.TcpFilter = filter
	}
}
func WithNetInterface(iface *net.Interface) ConfigOption {
	return func(config *Config) {
		config.Iface = iface
	}
}

func WithGatewayIP(ip net.IP) ConfigOption {
	return func(config *Config) {
		config.GatewayIP = ip
	}
}

func WithDefaultSourceIP(ip net.IP) ConfigOption {
	return func(config *Config) {
		config.SourceIP = ip
	}
}

func CreateConfigOptionsByTargetNetworkOrDomain(
	targetRaw string, duration time.Duration,
) (
	[]ConfigOption, error,
) {
	target := utils.ExtractHost(targetRaw)
	iface, gIp, sIp, err := netutil.Route(duration, target)
	if err != nil {
		return nil, errors.Errorf("route to %s failed: %s", target, err)
	}

	var opts = []ConfigOption{
		WithDefaultSourceIP(sIp),
		WithGatewayIP(gIp),
		WithNetInterface(iface),
	}
	return opts, nil
}

func WithIntervalMilliseconds(interval int) ConfigOption {
	return func(config *Config) {
	}
}

func WithPacketsPerSeconds(count int) ConfigOption {
	return func(config *Config) {
	}
}

func WithFetchGatewayHardwareAddressTimeout(timeout time.Duration) ConfigOption {
	return func(config *Config) {
		config.FetchGatewayHardwareAddressTimeout = timeout
	}
}
