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

type SynConfig struct {
	isLoopback bool
	// 发包必须的几个字段
	Iface     *net.Interface
	GatewayIP net.IP
	SourceIP  net.IP

	// Fetch Gateway Hardware Address TimeoutSeconds
	FetchGatewayHardwareAddressTimeout time.Duration
}

func NewDefaultConfig(extra ...SynConfigOption) (*SynConfig, error) {
	options, err := CreateConfigOptionsByTargetNetworkOrDomain("8.8.8.8", 5*time.Second)
	if err != nil {
		return nil, err
	}

	return NewConfig(append(options, extra...)...)
}

func NewConfig(options ...SynConfigOption) (*SynConfig, error) {
	config := &SynConfig{
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

type SynConfigOption func(config *SynConfig)

func WithLoopback(b bool) SynConfigOption {
	return func(config *SynConfig) {
		config.isLoopback = b
	}
}

func WithNetInterface(iface *net.Interface) SynConfigOption {
	return func(config *SynConfig) {
		config.Iface = iface
	}
}

func WithGatewayIP(ip net.IP) SynConfigOption {
	return func(config *SynConfig) {
		config.GatewayIP = ip
	}
}

func WithDefaultSourceIP(ip net.IP) SynConfigOption {
	return func(config *SynConfig) {
		config.SourceIP = ip
	}
}

func CreateConfigOptionsByIfaceName(ifaceName string) ([]SynConfigOption, error) {
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
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}

	// 获取网关和默认源地址
	var ifaceIp net.IP
	for _, addr := range addrs { // 获取网卡的ip地址，作为默认源地址使用，有限ipv4
		ip := addr.(*net.IPNet).IP
		if utils.IsIPv6(ip.String()) {
			ifaceIp = ip
		}
		if utils.IsIPv4(ip.String()) {
			ifaceIp = ip
			break
		}
	}
	if ifaceIp == nil {
		return nil, errors.Errorf("iface: %s has no addrs", iface.Name)
	}

	var opts = []SynConfigOption{
		WithNetInterface(iface),
		//WithGatewayIP(gIp),
		WithDefaultSourceIP(ifaceIp),
	}
	return opts, nil
}

func CreateConfigOptionsByTargetNetworkOrDomain(targetRaw string, duration time.Duration) ([]SynConfigOption, error) {
	target := utils.ExtractHost(targetRaw)
	iface, gIp, sIp, err := netutil.Route(duration, target)
	if err != nil {
		return nil, errors.Errorf("route to %s failed: %s", target, err)
	}

	var opts = []SynConfigOption{
		WithDefaultSourceIP(sIp),
		WithGatewayIP(gIp),
		WithNetInterface(iface),
	}
	return opts, nil
}

func WithIntervalMilliseconds(interval int) SynConfigOption {
	return func(config *SynConfig) {
	}
}

func WithPacketsPerSeconds(count int) SynConfigOption {
	return func(config *SynConfig) {
	}
}

func WithFetchGatewayHardwareAddressTimeout(timeout time.Duration) SynConfigOption {
	return func(config *SynConfig) {
		config.FetchGatewayHardwareAddressTimeout = timeout
	}
}
