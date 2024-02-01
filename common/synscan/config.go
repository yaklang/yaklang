package synscan

import (
	"github.com/pkg/errors"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/pcapx/pcaputil"
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

	for _, option := range options {
		option(config)
	}

	if config.Iface == nil {
		return nil, errors.New("config default net.Interface failed: empty iface")
	}
	return config, nil
}

type ConfigOption func(config *Config)

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

func CreateConfigOptionsByIfaceName(ifaceName string) ([]ConfigOption, error) {
	var iface *net.Interface
	var err error
	// 支持 net interface name 和 pcap dev name
	iface, err = net.InterfaceByName(ifaceName)
	if err != nil {
		iface, err = pcaputil.PcapIfaceNameToNetInterface(ifaceName)
		if err != nil {
			return nil, errors.Errorf("get iface failed: %s", err)
		}
	}
	log.Infof("use net interface: %v", iface.Name)

	//route, err := netroute.New()
	//if err != nil {
	//	return nil, errors.Errorf("create route failed: %s", err)
	//}
	//log.Debugf("start to find route for %s in %v", "ip", runtime.GOOS)
	//_, gateway, srcIP, err := route.Route(net.IPv4(0, 0, 0, 0))
	//if err != nil {
	//	return nil, errors.Errorf("route to %s failed: %s", "ip", err)
	//}
	var opts = []ConfigOption{
		WithNetInterface(iface),
		//WithGatewayIP(gateway),
		//WithDefaultSourceIP(srcIP),
	}
	return opts, nil
}

func CreateConfigOptionsByTargetNetworkOrDomain(targetRaw string, duration time.Duration) ([]ConfigOption, error) {
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
