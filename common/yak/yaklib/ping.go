package yaklib

import (
	"context"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
	"github.com/yaklang/yaklang/common/utils/pingutil"
	"github.com/yaklang/yaklang/common/yakdns"
	"strings"
	"time"
)

type _pingConfig struct {
	dnsTimeout         time.Duration
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

type _pingConfigOpt func(config *_pingConfig)

func _pingConfigOpt_skipped(i bool) _pingConfigOpt {
	return func(config *_pingConfig) {
		config.skipped = i
	}
}

func _pingConfigOpt_tcpPingPorts(i string) _pingConfigOpt {
	return func(c *_pingConfig) {
		c.tcpPingPort = i
	}
}

func _pingConfigOpt_proxy(i ...string) _pingConfigOpt {
	return func(config *_pingConfig) {
		if len(utils.StringArrayFilterEmpty(i)) <= 0 {
			return
		}
		config.proxies = i
	}
}

func _pingConfigOpt_withDNSTimeout(i float64) _pingConfigOpt {
	return func(config *_pingConfig) {
		config.dnsTimeout = utils.FloatSecondDuration(i)
	}
}

func _pingConfigOpt_withTimeout(i float64) _pingConfigOpt {
	return func(config *_pingConfig) {
		config.timeout = utils.FloatSecondDuration(i)
	}
}

func _pingConfigOpt_dnsServers(i ...string) _pingConfigOpt {
	return func(config *_pingConfig) {
		config.dnsServers = i
	}
}

func _pingConfigOpt_concurrent(i int) _pingConfigOpt {
	return func(config *_pingConfig) {
		config.concurrent = i
	}
}

func _pingConfigOpt_scanCClass(i bool) _pingConfigOpt {
	return func(config *_pingConfig) {
		config.scanCClass = i
	}
}

func _pingConfigOpt_cancel(f func()) _pingConfigOpt {
	return func(config *_pingConfig) {
		config._cancel = f
	}
}

func _pingScan(target string, opts ..._pingConfigOpt) chan *pingutil.PingResult {
	config := &_pingConfig{
		dnsTimeout: 5 * time.Second,
		timeout:    10 * time.Second,
		scanCClass: false,
		concurrent: 50,
	}

	for _, r := range opts {
		r(config)
	}

	if config.scanCClass {
		target = utils.ParseStringToCClassHosts(target)
	}

	ctx, cancel := context.WithCancel(context.Background())
	config._cancel = cancel
	opts = append(opts, _pingConfigOpt_cancel(config._cancel))

	var resultChan = make(chan *pingutil.PingResult)
	go func() {
		defer close(resultChan)

		swg := utils.NewSizedWaitGroup(config.concurrent)
		for _, hostRaw := range utils.ParseStringToHosts(target) {
			hostOrigin := hostRaw
			host := utils.ExtractHost(hostRaw)
			err := swg.AddWithContext(ctx)
			if err != nil {
				log.Error("cancel pingscan from context")
				return
			}
			targetHost := host
			go func() {
				defer swg.Done()

				if config.skipped || config.IsFiltered(targetHost) {
					resultChan <- &pingutil.PingResult{
						IP:     hostOrigin,
						Ok:     true,
						Reason: "skipped",
					}
					return
				}

				result := _ping(targetHost, opts...)
				//if utils.MatchAnyOfRegexp(result.Reason, "(?i)operation not permitted") {
				//	// 权限不足
				//	cancel()
				//}
				if config._onResult != nil {
					config._onResult(result)
				}
				if result != nil {
					if result.IP != hostOrigin {
						result.IP = hostOrigin
					}
					resultChan <- result
				}
			}()
		}
		swg.Wait()
	}()

	return resultChan
}

func _ping(target string, opts ..._pingConfigOpt) *pingutil.PingResult {
	config := &_pingConfig{
		dnsTimeout: time.Second * 5,
		timeout:    10 * time.Second,
	}
	for _, r := range opts {
		r(config)
	}

	if utils.IsIPv4(target) || utils.IsIPv6(target) {
		return pingutil.PingAuto(target, config.tcpPingPort, config.timeout)
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
		result := yakdns.LookupFirst(target, yakdns.WithTimeout(config.dnsTimeout), yakdns.WithDNSServers(config.dnsServers...))
		if result != "" && (utils.IsIPv4(result) || utils.IsIPv6(result)) {
			return pingutil.PingAuto(result, config.tcpPingPort, config.timeout)
		}
		return &pingutil.PingResult{
			IP:     target,
			Ok:     false,
			RTT:    0,
			Reason: utils.Errorf("parse/dns [%s] to ip failed", target).Error(),
		}
	}
}

func _pingConfigOpt_onResult(i func(result *pingutil.PingResult)) _pingConfigOpt {
	return func(config *_pingConfig) {
		config._onResult = i
	}
}

func _pingConfigOpt_excludeHosts(host string) _pingConfigOpt {
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
	"Scan": _pingScan,
	"Ping": _ping,

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
