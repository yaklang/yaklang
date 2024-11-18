package tools

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/netx"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"github.com/yaklang/yaklang/common/utils/network"
	"github.com/yaklang/yaklang/common/utils/pingutil"
	"strings"
	"sync"
	"time"
)

type _pingConfig struct {
	Ctx                context.Context
	RuntimeId          string
	dnsTimeout         time.Duration
	linkResolveTimeout time.Duration
	timeout            time.Duration
	dnsServers         []string
	scanCClass         bool
	concurrent         int
	skipped            bool
	tcpPingPort        string
	proxies            []string
	excludeHostsFilter *hostsparser.HostsParser
	_cancel            func()
	_onResult          func(result *pingutil.PingResult)
}

func NewDefaultPingConfig() *_pingConfig {
	return &_pingConfig{
		Ctx:        context.Background(),
		dnsTimeout: 5 * time.Second,
		timeout:    5 * time.Second,
		scanCClass: false,
		concurrent: 50,
		tcpPingPort: "22,80,443",
	}
}

type PingConfigOpt func(config *_pingConfig)

func WithPingCtx(ctx context.Context) PingConfigOpt {
	return func(cfg *_pingConfig) {
		cfg.Ctx = ctx
	}
}

func WithPingRuntimeId(id string) PingConfigOpt {
	return func(cfg *_pingConfig) {
		cfg.RuntimeId = id
	}
}

func _pingConfigOpt_skipped(i bool) PingConfigOpt {
	return func(config *_pingConfig) {
		config.skipped = i
	}
}

func _pingConfigOpt_tcpPingPorts(i string) PingConfigOpt {
	return func(c *_pingConfig) {
		c.tcpPingPort = i
	}
}

func _pingConfigOpt_proxy(i ...string) PingConfigOpt {
	return func(config *_pingConfig) {
		if len(utils.StringArrayFilterEmpty(i)) <= 0 {
			return
		}
		config.proxies = i
	}
}

func _pingConfigOpt_withDNSTimeout(i float64) PingConfigOpt {
	return func(config *_pingConfig) {
		config.dnsTimeout = utils.FloatSecondDuration(i)
	}
}

func _pingConfigOpt_withTimeout(i float64) PingConfigOpt {
	return func(config *_pingConfig) {
		config.timeout = utils.FloatSecondDuration(i)
	}
}

func _pingConfigOpt_LinkResolveTimeout(i float64) PingConfigOpt {
	return func(config *_pingConfig) {
		config.linkResolveTimeout = utils.FloatSecondDuration(i)
	}
}

func _pingConfigOpt_dnsServers(i ...string) PingConfigOpt {
	return func(config *_pingConfig) {
		config.dnsServers = i
	}
}

func _pingConfigOpt_concurrent(i int) PingConfigOpt {
	return func(config *_pingConfig) {
		config.concurrent = i
	}
}

func _pingConfigOpt_scanCClass(i bool) PingConfigOpt {
	return func(config *_pingConfig) {
		config.scanCClass = i
	}
}

func _pingConfigOpt_cancel(f func()) PingConfigOpt {
	return func(config *_pingConfig) {
		config._cancel = f
	}
}

func _pingScan(target string, opts ...PingConfigOpt) chan *pingutil.PingResult {
	config := NewDefaultPingConfig()

	for _, r := range opts {
		r(config)
	}

	if config.scanCClass {
		target = network.ParseStringToCClassHosts(target)
	}

	ctx, cancel := context.WithCancel(config.Ctx)
	config._cancel = cancel
	opts = append(opts, _pingConfigOpt_cancel(config._cancel))

	resultChan := make(chan *pingutil.PingResult)
	taskChan := make(chan string)

	go func() {
		defer close(resultChan)

		var wg sync.WaitGroup
		for i := 0; i < config.concurrent; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for hostRaw := range taskChan {
					host := utils.ExtractHost(hostRaw)
					targetHost := host

					if config.skipped || config.IsFiltered(targetHost) {
						select {
						case <-ctx.Done():
							return
						case resultChan <- &pingutil.PingResult{
							IP:     hostRaw,
							Ok:     true,
							Reason: "skipped",
						}:
						}
						continue
					}

					result := _ping(targetHost, opts...)
					if ctx.Err() != nil {
						return
					}
					if config._onResult != nil {
						config._onResult(result)
					}
					if result != nil {
						if result.IP != hostRaw {
							result.IP = hostRaw
						}
						select {
						case <-ctx.Done():
							return
						case resultChan <- result:
						}

					}
				}
			}()
		}

		for _, hostRaw := range utils.ParseStringToHosts(target) {
			select {
			case <-ctx.Done():
				log.Infof("ping scan canceled")
				goto EXIT
			case taskChan <- hostRaw:
			}
		}

	EXIT:
		close(taskChan)
		wg.Wait()
	}()

	return resultChan
}

func _ping(target string, opts ...PingConfigOpt) *pingutil.PingResult {
	config := NewDefaultPingConfig()
	for _, r := range opts {
		r(config)
	}

	pingOpts := []pingutil.PingConfigOpt{
		pingutil.WithPingContext(config.Ctx),
		pingutil.WithDefaultTcpPort(config.tcpPingPort),
		pingutil.WithTimeout(config.timeout),
		pingutil.WithProxies(config.proxies...),
		pingutil.WithLinkResolveTimeout(config.linkResolveTimeout),
	}

	if utils.IsIPv4(target) || utils.IsIPv6(target) {
		return pingutil.PingAuto(target, pingOpts...)
	} else if strings.HasPrefix(
		target, "https://") || strings.HasPrefix(
		target, "http://") {
		result, _, err := utils.ParseStringToHostPort(target)
		if err != nil {
			return &pingutil.PingResult{
				IP:     target,
				Ok:     false,
				RTT:    0,
				Reason: utils.Errorf("parse host[%s] from url failed: %s", target, err.Error()).Error(),
			}
		}
		return _ping(result, opts...)
	} else {
		result := netx.LookupFirst(target, netx.WithTimeout(config.dnsTimeout), netx.WithDNSServers(config.dnsServers...))
		if result != "" && (utils.IsIPv4(result) || utils.IsIPv6(result)) {
			return pingutil.PingAuto(target, pingOpts...)
		}
		return &pingutil.PingResult{
			IP:     target,
			Ok:     false,
			RTT:    0,
			Reason: utils.Errorf("parse/dns [%s] to ip failed", target).Error(),
		}
	}
}

func _pingConfigOpt_onResult(i func(result *pingutil.PingResult)) PingConfigOpt {
	return func(config *_pingConfig) {
		config._onResult = i
	}
}

func _pingConfigOpt_excludeHosts(host string) PingConfigOpt {
	return func(config *_pingConfig) {
		if host == "" {
			return
		}
		config.excludeHostsFilter = hostsparser.NewHostsParser(context.Background(), host)
	}
}

func (c *_pingConfig) IsFiltered(host string) bool {
	if c == nil {
		return false
	}

	if c.excludeHostsFilter != nil {
		if c.excludeHostsFilter.Contains(host) {
			return true
		}
	}

	return false
}

var PingExports = map[string]interface{}{
	"Scan":         _pingScan,
	"Ping":         _ping,
	"excludeHosts": _pingConfigOpt_excludeHosts,
	"onResult":     _pingConfigOpt_onResult,
	"dnsTimeout":   _pingConfigOpt_withDNSTimeout,
	"timeout":      _pingConfigOpt_withTimeout,
	"dnsServers":   _pingConfigOpt_dnsServers,
	"scanCClass":   _pingConfigOpt_scanCClass,
	"skip":         _pingConfigOpt_skipped,
	"concurrent":   _pingConfigOpt_concurrent,
	"tcpPingPorts": _pingConfigOpt_tcpPingPorts,
	"proxy":        _pingConfigOpt_proxy,
}
