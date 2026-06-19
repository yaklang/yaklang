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
		Ctx:         context.Background(),
		dnsTimeout:  5 * time.Second,
		timeout:     5 * time.Second,
		scanCClass:  false,
		concurrent:  50,
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

// skip 设置是否跳过实际存活探测，直接将目标标记为存活(常用于已知目标存活的场景)
// 在 yak 中通过 ping.skip 调用
// 参数:
//   - i: 是否跳过探测
//
// 返回值:
//   - 一个 ping.Scan/ping.Ping 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：跳过存活探测
// res = ping.Scan("192.168.1.1/24", ping.skip(true))
// ```
func _pingConfigOpt_skipped(i bool) PingConfigOpt {
	return func(config *_pingConfig) {
		config.skipped = i
	}
}

// tcpPingPorts 设置 TCP Ping 使用的端口列表(当 ICMP 不可用时回退到 TCP 探测)
// 在 yak 中通过 ping.tcpPingPorts 调用
// 参数:
//   - i: 逗号分隔的端口列表，如 "22,80,443"
//
// 返回值:
//   - 一个 ping.Scan/ping.Ping 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：指定 TCP Ping 端口
// res = ping.Scan("192.168.1.1/24", ping.tcpPingPorts("80,443"))
// ```
func _pingConfigOpt_tcpPingPorts(i string) PingConfigOpt {
	return func(c *_pingConfig) {
		c.tcpPingPort = i
	}
}

// proxy 设置探测时使用的代理地址列表
// 在 yak 中通过 ping.proxy 调用
// 参数:
//   - i: 一个或多个代理地址
//
// 返回值:
//   - 一个 ping.Scan/ping.Ping 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：通过代理探测
// res = ping.Scan("192.168.1.1/24", ping.proxy("socks5://127.0.0.1:1080"))
// ```
func _pingConfigOpt_proxy(i ...string) PingConfigOpt {
	return func(config *_pingConfig) {
		if len(utils.StringArrayFilterEmpty(i)) <= 0 {
			return
		}
		config.proxies = i
	}
}

// dnsTimeout 设置域名解析(DNS)的超时时间(秒)
// 在 yak 中通过 ping.dnsTimeout 调用
// 参数:
//   - i: 超时时间(秒)
//
// 返回值:
//   - 一个 ping.Scan/ping.Ping 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置 DNS 解析超时 3 秒
// res = ping.Scan("example.com", ping.dnsTimeout(3))
// ```
func _pingConfigOpt_withDNSTimeout(i float64) PingConfigOpt {
	return func(config *_pingConfig) {
		config.dnsTimeout = utils.FloatSecondDuration(i)
	}
}

// timeout 设置单次存活探测的超时时间(秒)
// 在 yak 中通过 ping.timeout 调用
// 参数:
//   - i: 超时时间(秒)
//
// 返回值:
//   - 一个 ping.Scan/ping.Ping 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：设置探测超时 5 秒
// res = ping.Scan("192.168.1.1/24", ping.timeout(5))
// ```
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

// dnsServers 设置进行域名解析时使用的自定义 DNS 服务器列表
// 在 yak 中通过 ping.dnsServers 调用
// 参数:
//   - i: 一个或多个 DNS 服务器地址
//
// 返回值:
//   - 一个 ping.Scan/ping.Ping 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用自定义 DNS 服务器
// res = ping.Scan("example.com", ping.dnsServers("8.8.8.8", "1.1.1.1"))
// ```
func _pingConfigOpt_dnsServers(i ...string) PingConfigOpt {
	return func(config *_pingConfig) {
		config.dnsServers = i
	}
}

// concurrent 设置存活探测的并发数量(默认 50)
// 在 yak 中通过 ping.concurrent 调用
// 参数:
//   - i: 并发数量
//
// 返回值:
//   - 一个 ping.Scan/ping.Ping 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：以 100 并发探测
// res = ping.Scan("192.168.1.1/24", ping.concurrent(100))
// ```
func _pingConfigOpt_concurrent(i int) PingConfigOpt {
	return func(config *_pingConfig) {
		config.concurrent = i
	}
}

// scanCClass 设置是否将目标扩展为其所在的整个 C 段(/24)再进行探测
// 在 yak 中通过 ping.scanCClass 调用
// 参数:
//   - i: 是否扩展为 C 段
//
// 返回值:
//   - 一个 ping.Scan/ping.Ping 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：探测目标所在 C 段
// res = ping.Scan("192.168.1.1", ping.scanCClass(true))
// ```
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

// Scan 对一个或一批目标执行存活探测(ping)，以 channel 形式流式返回每个目标的存活结果
// 在 yak 中通过 ping.Scan 调用，target 支持 IP、域名、CIDR、逗号分隔或范围等多种写法
// 参数:
//   - target: 探测目标，如 "192.168.1.1"、"192.168.1.0/24"、"a.com,b.com"
//   - opts: 可选配置项，如 ping.timeout、ping.concurrent、ping.tcpPingPorts 等
//
// 返回值:
//   - 一个只读 channel，逐条产出 *pingutil.PingResult 探测结果
//
// Example:
// ```
// // 该示例为示意性用法：探测 C 段存活主机
// res = ping.Scan("192.168.1.1/24", ping.timeout(5), ping.concurrent(50))
//
//	for result = range res {
//	    if result.Ok {
//	        println("alive:", result.IP)
//	    }
//	}
//
// ```
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

// Ping 对单个目标执行存活探测，自动在 ICMP 与 TCP Ping 之间选择，返回探测结果
// 在 yak 中通过 ping.Ping 调用，target 支持 IP、域名或 http(s) URL
// 参数:
//   - target: 单个探测目标
//   - opts: 可选配置项，如 ping.timeout、ping.tcpPingPorts、ping.proxy 等
//
// 返回值:
//   - 探测结果对象，包含目标 IP、是否存活、RTT 与原因等
//
// Example:
// ```
// // 该示例为示意性用法：探测单个目标是否存活
// result = ping.Ping("127.0.0.1", ping.timeout(5))
// println(result.IP, result.Ok)
// ```
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

// onResult 设置每得到一个探测结果时触发的回调函数
// 在 yak 中通过 ping.onResult 调用
// 参数:
//   - i: 接收单个探测结果的回调函数
//
// 返回值:
//   - 一个 ping.Scan/ping.Ping 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：通过回调处理每个结果
//
//	res = ping.Scan("192.168.1.1/24", ping.onResult(func(result) {
//	    println(result.IP, result.Ok)
//	}))
//
// ```
func _pingConfigOpt_onResult(i func(result *pingutil.PingResult)) PingConfigOpt {
	return func(config *_pingConfig) {
		config._onResult = i
	}
}

// excludeHosts 设置探测时需要排除的主机(支持 IP、CIDR、范围等写法)
// 在 yak 中通过 ping.excludeHosts 调用
// 参数:
//   - host: 需要排除的主机表达式
//
// 返回值:
//   - 一个 ping.Scan/ping.Ping 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：排除网关地址
// res = ping.Scan("192.168.1.1/24", ping.excludeHosts("192.168.1.1"))
// ```
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
