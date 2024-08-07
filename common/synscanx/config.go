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

	maxOpenPorts       uint16 // 单个 IP 允许的最大开放端口数
	targetsCount       uint16
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

func TargetCount(count int) SynxConfigOption {
	return func(config *SynxConfig) {
		config.targetsCount = uint16(count)
	}
}

func WithMaxOpenPorts(max int) SynxConfigOption {
	return func(config *SynxConfig) {
		if max <= 0 {
			max = 65535
		}
		config.maxOpenPorts = uint16(max)
	}
}

// shuffle syn scan 的配置选项，设置是否打乱扫描顺序
// @param {bool} s 是否打乱
// @return {scanOpt} 返回配置选项
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
// @param {string} file 文件路径
// @return {scanOpt} 返回配置选项
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
// @param {string} prefix 前缀
// @return {scanOpt} 返回配置选项
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

// wait syn scan 的配置选项，设置等待扫描目标回包的最大时间
// @param {float64} sec 等待时间，单位秒
// @return {scanOpt} 返回配置选项
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
	}
}

// initPortFilter syn scan 的配置选项，设置本次扫描的端口过滤器，只展示这些端口的扫描结果
// @param {string} f 端口，支持逗号、-分割
// @return {scanOpt} 返回配置选项
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
// @param {string} f 主机，支持逗号、CIDR、-分割
// @return {scanOpt} 返回配置选项
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
// @param {int} ms 延迟多少毫秒
// @param {int} count 每隔多少个数据包延迟一次
// @return {scanOpt} 返回配置选项
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
// @param {int} count 并发数
// @return {scanOpt} 返回配置选项
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
		config.rateLimitDelayMs = float64(time.Second) / float64(count) / float64(time.Millisecond)
		config.rateLimitDelayGap = count / 10
		log.Infof("rate limit delay ms: %v(ms)", config.rateLimitDelayMs)
		log.Infof("rate limit delay gap: %v", config.rateLimitDelayGap)
	}
}

// excludeHosts syn scan 的配置选项，设置本次扫描排除的主机
// @param {string} hosts 主机，支持逗号分割、CIDR、-的格式
// @return {scanOpt} 返回配置选项
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
// @param {string} ports 端口，支持 1-65535、1,2,3、1-100,200-300 格式
// @return {scanOpt} 返回配置选项
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
// @param {func(i *synscan.SynScanResult)} i 回调函数
// @return {scanOpt} 返回配置选项
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
// @param {func(string)} i 回调函数
// @return {scanOpt} 返回配置选项
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
// @param {string} iface 网卡名称
// @return {scanOpt} 返回配置选项
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
