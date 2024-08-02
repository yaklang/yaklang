package synscanx

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
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
	netInterface     string // net interface name
	shuffle          bool   // 是否打乱扫描顺序

	rateLimitDelayMs  float64
	rateLimitDelayGap int // 每隔多少数据包 delay 一次？

	excludeHosts *hostsparser.HostsParser

	excludePorts *utils.PortsFilter

	maxOpenPorts uint16 // 单个 IP 允许的最大开放端口数

	callback           func(result *synscan.SynScanResult)
	submitTaskCallback func(i string)

	// 发包
	Iface     *net.Interface
	GatewayIP net.IP
	SourceIP  net.IP
	// 内网扫描时，目标机器的 MAC 地址来自 ARP
	// 外网扫描时，目标机器的 MAC 地址就是网关的 MAC 地址
	SourceMac, RemoteMac net.HardwareAddr

	// Fetch Gateway Hardware Address TimeoutSeconds
	FetchGatewayHardwareAddressTimeout time.Duration
}

func (s *SynxConfig) callCallback(r *synscan.SynScanResult) {
	if s == nil {
		return
	}

	if s.callback == nil {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("synscan callback failed: %s", err)
		}
	}()

	s.callback(r)
}

func (s *SynxConfig) callSubmitTaskCallback(r string) {
	if s == nil {
		return
	}

	if s.submitTaskCallback == nil {
		return
	}

	defer func() {
		if err := recover(); err != nil {
			log.Errorf("synscan callback failed: %s", err)
		}
	}()

	s.submitTaskCallback(r)
}

func NewDefaultConfig() *SynxConfig {
	return &SynxConfig{
		waiting: 5 * time.Second,
		// 这个限速器每秒可以允许最多 1000 个请求，短时间内可以允许突发的 150 个请求
		rateLimitDelayMs:                   1,
		rateLimitDelayGap:                  150,
		shuffle:                            true,
		FetchGatewayHardwareAddressTimeout: 3 * time.Second,
	}
}

type SynxConfigOption func(config *SynxConfig)

func WithMaxOpenPorts(max int) SynxConfigOption {
	return func(config *SynxConfig) {
		if max <= 0 {
			max = 65535
		}
		config.maxOpenPorts = uint16(max)
	}
}

func WithShuffle(s bool) SynxConfigOption {
	return func(config *SynxConfig) {
		config.shuffle = s
	}
}

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

func WithWaiting(sec float64) SynxConfigOption {
	return func(config *SynxConfig) {
		config.waiting = utils.FloatSecondDuration(sec)
		if config.waiting <= 0 {
			config.waiting = 5 * time.Second
		}
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

func WithRateLimit(ms, count int) SynxConfigOption {
	return func(config *SynxConfig) {
		config.rateLimitDelayMs = float64(ms)
		config.rateLimitDelayGap = count
	}
}

func WithConcurrent(count int) SynxConfigOption {
	return func(config *SynxConfig) {
		if count <= 0 {
			count = 1000
		}
		config.rateLimitDelayMs = float64(time.Second) / float64(count) / float64(time.Millisecond)
		config.rateLimitDelayGap = count / 10
		log.Infof("rate limit delay ms: %v(ms)", config.rateLimitDelayMs)
		log.Infof("rate limit delay gap: %v", config.rateLimitDelayGap)
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
		config.excludePorts = utils.NewPortsFilter(ports)
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

func WithIface(iface string) SynxConfigOption {
	return func(config *SynxConfig) {
		config.netInterface = iface
	}
}
