package synscanx

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/synscan"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"net"
	"time"
)

type SynxConfig struct {
	// options
	outputFile       string
	outputFilePrefix string
	waiting          time.Duration
	initFilterPorts  string
	initFilterHosts  string
	netInterface     string

	rateLimitDelayMs  float64
	rateLimitDelayGap int // 每隔多少数据包 delay 一次？

	excludeHosts *hostsparser.HostsParser
	ExcludePorts *filter.StringFilter

	callback           func(result *synscan.SynScanResult)
	submitTaskCallback func(i string)

	// 发包
	Iface     *net.Interface
	GatewayIP net.IP
	SourceIP  net.IP

	// Fetch Gateway Hardware Address TimeoutSeconds
	FetchGatewayHardwareAddressTimeout time.Duration
}



func (sc *SynxConfig) filtered(port int) bool {
	if sc.ExcludePorts != nil && port > 0 {
		if sc.ExcludePorts.Exist(fmt.Sprint(port)) {
			return true
		}
	}
	return false
}

func NewDefaultConfig() *SynxConfig {
	return &SynxConfig{
		waiting:           5 * time.Second,
		rateLimitDelayMs:  1,
		rateLimitDelayGap: 5,
		ExcludePorts:      filter.NewFilter(),
	}
}

type SynxConfigOption func(config *SynxConfig)

func WithOutputFile(file string) SynxConfigOption {
	return func(config *SynxConfig) {
		config.outputFile = file
	}
}

func WithOutputFilePrefix(prefix string) SynxConfigOption {
	return func(config *SynxConfig) {
		config.outputFilePrefix = prefix
	}
}

func WithWaiting(d time.Duration) SynxConfigOption {
	return func(config *SynxConfig) {
		config.waiting = d
	}
}

func WithInitFilterPorts(ports string) SynxConfigOption {
	return func(config *SynxConfig) {
		config.initFilterPorts = ports
	}
}

func WithInitFilterHosts(hosts string) SynxConfigOption {
	return func(config *SynxConfig) {
		config.initFilterHosts = hosts
	}
}

func WithNetInterface(iface string) SynxConfigOption {
	return func(config *SynxConfig) {
		config.netInterface = iface
	}
}

func WithRateLimitDelayMs(ms float64) SynxConfigOption {
	return func(config *SynxConfig) {
		config.rateLimitDelayMs = ms
	}
}

func WithRateLimitDelayGap(count int) SynxConfigOption {
	return func(config *SynxConfig) {
		config.rateLimitDelayGap = count
	}
}

func WithExcludeHosts(hosts string) SynxConfigOption {
	return func(config *SynxConfig) {
		if hosts == "" {
			return
		}
		config.excludeHosts = hostsparser.NewHostsParser(context.Background(), hosts)
	}
}

func WithExcludePorts(ports string) SynxConfigOption {
	return func(config *SynxConfig) {
		if ports == "" {
			return
		}
		config.ExcludePorts = filter.NewFilter()
		for _, port := range utils.ParseStringToPorts(ports) {
			config.ExcludePorts.Insert(fmt.Sprint(port))
		}
	}
}

func WithCallback(callback func(result *synscan.SynScanResult)) SynxConfigOption {
	return func(config *SynxConfig) {
		config.callback = callback
	}
}

func WithSubmitTaskCallback(callback func(i string)) SynxConfigOption {
	return func(config *SynxConfig) {
		config.submitTaskCallback = callback
	}
}
