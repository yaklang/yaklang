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
	outputFile          string
	outputFilePrefix    string
	waiting             time.Duration
	tcpProbeTimeout     time.Duration
	tcpProbeConcurrency int
	initFilterPorts     string
	initFilterHosts     string
	netInterface        string // net interface name
	shuffle             bool   // 是否打乱扫描顺序

	rateLimitDelayMs  float64
	rateLimitDelayGap int // 每隔多少数据包 delay 一次？
	// appendOutput keeps an existing output file when a later route group runs.
	appendOutput bool

	RuntimeId string

	Ctx context.Context

	excludeHosts *hostsparser.HostsParser

	excludePorts *utils.PortsFilter

	maxOpenPorts       int // 单个 IP 允许的最大开放端口数
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
		// 1 packet per millisecond is 1000 packets per second. rateLimit's
		// second argument raises that count; the capture path does not burst.
		rateLimitDelayMs:                   1,
		rateLimitDelayGap:                  1,
		shuffle:                            true,
		FetchGatewayHardwareAddressTimeout: 3 * time.Second,
		Ctx:                                context.Background(),
	}
}

type SynxConfigOption func(config *SynxConfig)

// maxOpenPorts syn scan 的配置选项，设置单个 IP 允许的最大开放端口数
// 参数:
//   - max: 最大开放端口数
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.maxOpenPorts(100) // 单个 IP 最多开放 100 个端口
//
// )
// die(err)
// ```
func WithMaxOpenPorts(max int) SynxConfigOption {
	return func(config *SynxConfig) {
		if max <= 0 {
			max = 65535
		}
		// A host can expose TCP and UDP. Values above that are not a uint16
		// wrap; they stay as written up to the cap below.
		if max > 65535*2 {
			max = 65535 * 2
		}
		config.maxOpenPorts = max
	}
}

// shuffle syn scan 的配置选项，设置是否打乱扫描顺序
// 参数:
//   - s: 是否打乱扫描顺序
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.shuffle(true) // 打乱扫描顺序
//
// )
// die(err)
// ```
func WithShuffle(s bool) SynxConfigOption {
	return func(config *SynxConfig) {
		config.shuffle = s
	}
}

// outputFile syn scan 的配置选项，设置本次扫描结果保存到指定的文件
// 参数:
//   - file: 文件路径
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.outputFile("/tmp/open_ports.txt")
//
// )
// die(err)
// ```
func WithOutputFile(file string) SynxConfigOption {
	return func(config *SynxConfig) {
		config.outputFile = file
	}
}

// outputPrefix syn scan 的配置选项，设置本次扫描结果保存到文件时添加自定义前缀，比如 tcp:// https:// http:// 等，需要配合 outputFile 使用
// 参数:
//   - prefix: 保存到文件时添加的前缀，例如 tcp://
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	 synscan.outputFile("./open_ports.txt"),
//		synscan.outputPrefix("tcp://")
//
// )
// die(err)
// ```
func WithOutputFilePrefix(prefix string) SynxConfigOption {
	return func(config *SynxConfig) {
		config.outputFilePrefix = prefix
	}
}

// wait syn scan 的配置选项，设置 UDP 应答等待时间，并作为 TCP 探测的总时间预算。TCP 的重试间隔会按这个预算拉开，高延迟链路上的 SYN-ACK 不会被固定的 2 秒上限截掉。已经设置过 tcpTimeout 时，这里不再覆盖它。
// 参数:
//   - sec: 等待时间，单位为秒
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.wait(5) // 等待 5 秒
//
// )
// die(err)
// ```
func WithWaiting(sec float64) SynxConfigOption {
	return func(config *SynxConfig) {
		config.waiting = utils.FloatSecondDuration(sec)
		if config.waiting <= 0 {
			config.waiting = 5 * time.Second
		}
		if config.tcpProbeTimeout <= 0 {
			config.tcpProbeTimeout = config.waiting
		}
	}
}

// initPortFilter syn scan 的配置选项，设置本次扫描的端口过滤器，只展示这些端口的扫描结果
// 参数:
//   - ports: 端口，支持逗号、- 分割
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// res, err = synscan.Scan("192.168.3.1", "1-65535",
//
//	synscan.initPortFilter("1-100,200-300")
//
// )
// die(err)
// ```
func WithInitFilterPorts(ports string) SynxConfigOption {
	return func(config *SynxConfig) {
		config.initFilterPorts = ports
	}
}

// initHostFilter syn scan 的配置选项，设置本次扫描的主机过滤器，只展示这些主机的扫描结果
// 参数:
//   - hosts: 主机，支持逗号、CIDR、- 分割
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// res, err = synscan.Scan("192.168.1.1/24", "1-65535",
//
//	synscan.initHostFilter("192.168.1.1,192.168.1.2")
//
// )
// die(err)
// ```
func WithInitFilterHosts(hosts string) SynxConfigOption {
	return func(config *SynxConfig) {
		config.initFilterHosts = hosts
	}
}

// rateLimit syn scan 的配置选项，设置 syn 扫描的速率
// 参数:
//   - ms: 延迟多少毫秒
//   - count: 每隔多少个数据包延迟一次
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.rateLimit(1, 2000) // 每隔 2000 个数据包延迟 1 毫秒
//
// )
// die(err)
// ```
func WithRateLimit(ms, count int) SynxConfigOption {
	return func(config *SynxConfig) {
		config.rateLimitDelayMs = float64(ms)
		config.rateLimitDelayGap = count
	}
}

// concurrent syn scan 的配置选项，设置 syn 扫描的发包速率，和 rateLimit 基本相同
// 参数:
//   - count: 并发数
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.concurrent(1000) // 并发 1000
//
// )
// die(err)
// ```
func WithConcurrent(count int) SynxConfigOption {
	return func(config *SynxConfig) {
		if count <= 0 {
			count = 1000
		}
		// Outstanding ProbeSYN calls. The same value is the session admission
		// limit; the delay below is the sustained SYN rate passed to netstackvm.
		config.tcpProbeConcurrency = count
		config.rateLimitDelayMs = float64(time.Second) / float64(count) / float64(time.Millisecond)
		// Gap 1 keeps the rate equal to one packet per delay. A larger gap is
		// the rateLimit(ms, count) packet count, not a burst.
		config.rateLimitDelayGap = 1
		log.Debugf("rate limit delay ms: %v(ms)", config.rateLimitDelayMs)
		log.Debugf("rate limit delay gap: %v", config.rateLimitDelayGap)
	}
}

// excludeHosts syn scan 的配置选项，设置本次扫描排除的主机
// 参数:
//   - hosts: 主机，支持逗号分割、CIDR、- 的格式
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// res, err = synscan.Scan("192.168.1.1/24", "1-65535",
//
//	synscan.excludeHosts("192.168.1.1,192.168.1.3-10,192.168.1.1/26")
//
// )
// die(err)
// ```
func WithExcludeHosts(hosts string) SynxConfigOption {
	return func(config *SynxConfig) {
		if hosts == "" {
			return
		}
		config.excludeHosts = hostsparser.NewHostsParser(context.Background(), hosts)
	}
}

// excludePorts syn scan 的配置选项，设置本次扫描排除的端口
// 参数:
//   - ports: 端口，支持 1-65535、1,2,3、1-100,200-300 格式
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.excludePorts("1-100,200-300") // 排除 1-100 和 200-300 端口
//
// )
// die(err)
// ```
func WithExcludePorts(ports string) SynxConfigOption {
	return func(config *SynxConfig) {
		if ports == "" {
			return
		}
		config.excludePorts = utils.NewPortsFilter(ports)
	}
}

// callback syn scan 的配置选项，设置一个回调函数，每发现一个端口就会调用一次
// 参数:
//   - callback: 回调函数，入参为单个 SYN 扫描结果
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.callback(func(i){
//	   db.SavePortFromResult(i) // 将结果保存到数据库
//	})
//
// )
// die(err)
// ```
func WithCallback(callback func(result *synscan.SynScanResult)) SynxConfigOption {
	return func(config *SynxConfig) {
		config.callback = callback
	}
}

// submitTaskCallback syn scan 的配置选项，设置一个回调函数，每提交一个探测数据包的时候，这个回调会执行一次
// 参数:
//   - callback: 回调函数，入参为本次提交探测的目标字符串
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// res, err = synscan.Scan("127.0.0.1", "1-65535",
//
//	synscan.submitTaskCallback(func(i){
//	   println(i) // 打印要探测的目标
//	})
//
// )
// die(err)
// ```
func WithSubmitTaskCallback(callback func(i string)) SynxConfigOption {
	return func(config *SynxConfig) {
		config.submitTaskCallback = callback
	}
}

// iface syn scan 的配置选项，设置 syn 扫描的网卡
// 参数:
//   - iface: 网卡名称
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// res, err = synscan.Scan("192.168.1.1/24", "1-65535",
//
//	synscan.iface("eth0") // 使用 eth0 网卡
//
// )
// die(err)
// ```
func WithIface(iface string) SynxConfigOption {
	return func(config *SynxConfig) {
		config.netInterface = iface
	}
}

func WithRuntimeId(id string) SynxConfigOption {
	return func(config *SynxConfig) {
		config.RuntimeId = id
	}
}

// tcpTimeout sets the total TCP probe budget in seconds, including retries.
// Non-positive values keep the built-in retry policy (about 3.5s of response waits).
func WithTCPProbeTimeoutSeconds(sec float64) SynxConfigOption {
	return func(config *SynxConfig) {
		config.tcpProbeTimeout = utils.FloatSecondDuration(sec)
		if config.tcpProbeTimeout <= 0 {
			config.tcpProbeTimeout = defaultTCPProbeTimeout
		}
	}
}

// context 设置 SYN 扫描使用的 context，用于取消或超时控制
// 在 yak 中通过 synscan.context 调用；cancel 时 SYN 发包/结果投递循环会立即短路退出
// 参数:
//   - ctx: 上下文对象
//
// 返回值:
//   - 一个 synscan.Scan 可接收的配置选项
//
// Example:
// ```
// // 该示例为示意性用法：使用可取消的 context 控制 SYN 扫描
// ctx, cancel = context.WithCancel(context.Background())
// defer cancel()
// res = synscan.Scan("192.168.1.1", "80,443", synscan.context(ctx))~
//
//	for result = range res {
//	    println(result.String())
//	}
//
// ```
func WithCtx(ctx context.Context) SynxConfigOption {
	return func(config *SynxConfig) {
		config.Ctx = ctx
	}

}

// WithTCPProbeTimeout sets each TCP target's independent total deadline, covering
// admission, sending and retries. Non-positive values use the 15 second default.
func WithTCPProbeTimeout(timeout time.Duration) SynxConfigOption {
	return func(config *SynxConfig) { config.tcpProbeTimeout = timeout }
}

// WithTCPProbeConcurrency sets how many TCP probes may be in flight. The
// session admission limit and the worker count both use this value.
// Non-positive values keep the default of 256. Yak's synscan.concurrent sets it too.
func WithTCPProbeConcurrency(concurrency int) SynxConfigOption {
	return func(config *SynxConfig) { config.tcpProbeConcurrency = concurrency }
}
