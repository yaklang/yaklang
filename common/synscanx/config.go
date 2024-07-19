package synscanx

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/filter"
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
	shuffle          bool // 是否打乱扫描顺序

	rateLimitDelayMs  float64
	rateLimitDelayGap int // 每隔多少数据包 delay 一次？

	excludeHosts *hostsparser.HostsParser
	ExcludePorts *filter.StringFilter

	callback           func(result *synscan.SynScanResult)
	submitTaskCallback func(i string)

	// 发包
	Iface                *net.Interface
	GatewayIP            net.IP
	SourceIP             net.IP
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

func (s *SynxConfig) filtered(port int) bool {
	if s.ExcludePorts != nil && port > 0 {
		if s.ExcludePorts.Exist(fmt.Sprint(port)) {
			return true
		}
	}
	return false
}

func NewDefaultConfig() *SynxConfig {
	return &SynxConfig{
		waiting: 5 * time.Second,
		// 这个限速器每秒可以允许最多 1000 个请求，短时间内可以允许突发的 150 个请求
		rateLimitDelayMs:                   1,
		rateLimitDelayGap:                  150,
		ExcludePorts:                       filter.NewFilter(),
		FetchGatewayHardwareAddressTimeout: 3 * time.Second,
	}
}

type SynxConfigOption func(config *SynxConfig)

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

func WithIface(iface string) SynxConfigOption {
	return func(config *SynxConfig) {
		config.netInterface = iface
	}
}
