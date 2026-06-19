package yaklib

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/dnsutil"
	"time"
)

type _dnsConfig struct {
	timeout    time.Duration
	dnsServers []string
}

type _dnsConfigOpt func(c *_dnsConfig)

// timeout 是一个 DNS 查询配置选项，用于设置查询的超时时间（单位：秒）
// 参数:
//   - d: 超时时间，单位为秒，支持小数
//
// 返回值:
//   - 一个 DNS 查询配置选项，作为可变参数传入查询函数
//
// Example:
// ```
// // 设置 3 秒超时进行 DNS 查询，此处仅作示意
// ip = dns.QueryIP("www.example.com", dns.timeout(3))
// println(ip)
// ```
func _dnsConfigOpt_WithTimeout(d float64) _dnsConfigOpt {
	return func(c *_dnsConfig) {
		c.timeout = utils.FloatSecondDuration(d)
	}
}

// dnsServers 是一个 DNS 查询配置选项，用于指定自定义的 DNS 服务器列表
// 参数:
//   - servers: 一个或多个 DNS 服务器地址（如 "8.8.8.8"）
//
// 返回值:
//   - 一个 DNS 查询配置选项，作为可变参数传入查询函数
//
// Example:
// ```
// // 指定使用 8.8.8.8 与 1.1.1.1 进行查询，此处仅作示意
// ip = dns.QueryIP("www.example.com", dns.dnsServers("8.8.8.8", "1.1.1.1"), dns.timeout(5))
// println(ip)
// ```
func _dnsConfigOpt_WithDNSServers(servers ...string) _dnsConfigOpt {
	return func(c *_dnsConfig) {
		c.dnsServers = servers
	}
}

// QueryIP 查询目标域名的 A 记录，返回解析到的第一个 IPv4 地址
// 参数:
//   - target: 要查询的域名
//   - opts: 可选配置，例如 dns.timeout、dns.dnsServers
//
// 返回值:
//   - 解析到的 IP 地址字符串；解析失败时返回空字符串
//
// Example:
// ```
// // 真实 DNS 查询，结果依赖网络与解析服务，此处仅作示意
// ip = dns.QueryIP("www.example.com", dns.timeout(5))
// println(ip)
// ```
func _dnsQueryIP(target string, opts ..._dnsConfigOpt) string {
	var config = &_dnsConfig{
		timeout: 5 * time.Second,
	}

	for _, o := range opts {
		o(config)
	}

	return dnsutil.QueryIP(target, config.timeout, config.dnsServers)
}

// QueryIPAll 查询目标域名的全部 A 记录，返回所有解析到的 IPv4 地址
// 参数:
//   - target: 要查询的域名
//   - opts: 可选配置，例如 dns.timeout、dns.dnsServers
//
// 返回值:
//   - 解析到的所有 IP 地址字符串切片；解析失败时返回空切片
//
// Example:
// ```
// // 真实 DNS 查询，结果依赖网络与解析服务，此处仅作示意
// ips = dns.QueryIPAll("www.example.com", dns.timeout(5))
// for ip in ips { println(ip) }
// ```
func _dnsQueryIPAll(target string, opts ..._dnsConfigOpt) []string {
	var config = &_dnsConfig{
		timeout: 5 * time.Second,
	}

	for _, o := range opts {
		o(config)
	}

	return dnsutil.QueryIPAll(target, config.timeout, config.dnsServers)
}

// QueryNS 查询目标域名的 NS（权威域名服务器）记录
// 参数:
//   - target: 要查询的域名
//   - opts: 可选配置，例如 dns.timeout、dns.dnsServers
//
// 返回值:
//   - 解析到的 NS 记录字符串切片；解析失败时返回空切片
//
// Example:
// ```
// // 真实 DNS 查询，结果依赖网络与解析服务，此处仅作示意
// records = dns.QueryNS("example.com", dns.timeout(5))
// for r in records { println(r) }
// ```
func _dnsQueryNS(target string, opts ..._dnsConfigOpt) []string {
	var config = &_dnsConfig{
		timeout: 5 * time.Second,
	}

	for _, o := range opts {
		o(config)
	}

	return dnsutil.QueryNS(target, config.timeout, config.dnsServers)
}

// QueryTXT 查询目标域名的 TXT 记录
// 参数:
//   - target: 要查询的域名
//   - opts: 可选配置，例如 dns.timeout、dns.dnsServers
//
// 返回值:
//   - 解析到的 TXT 记录字符串切片；解析失败时返回空切片
//
// Example:
// ```
// // 真实 DNS 查询，结果依赖网络与解析服务，此处仅作示意
// records = dns.QueryTXT("example.com", dns.timeout(5))
// for r in records { println(r) }
// ```
func _dnsQueryTxt(target string, opts ..._dnsConfigOpt) []string {

	var config = &_dnsConfig{
		timeout: 5 * time.Second,
	}

	for _, o := range opts {
		o(config)
	}
	return dnsutil.QueryTxt(target, config.timeout, config.dnsServers)
}

// QueryAxfr 对目标域名发起 AXFR（DNS 区域传送）查询，常用于检测域传送配置错误
// 参数:
//   - target: 要查询的域名
//   - opts: 可选配置，例如 dns.timeout、dns.dnsServers
//
// 返回值:
//   - 区域传送返回的记录字符串切片；失败或被拒绝时返回空切片
//
// Example:
// ```
// // 真实 DNS 区域传送查询，结果依赖目标配置，此处仅作示意
// records = dns.QueryAxfr("zonetransfer.me", dns.timeout(5))
// for r in records { println(r) }
// ```
func _dnsQueryAxfr(target string, opts ..._dnsConfigOpt) []string {

	var config = &_dnsConfig{
		timeout: 5 * time.Second,
	}

	for _, o := range opts {
		o(config)
	}
	return dnsutil.QueryAXFR(target, config.timeout, config.dnsServers)
}

var DnsExports = map[string]interface{}{
	"QueryIP":    _dnsQueryIP,
	"QueryIPAll": _dnsQueryIPAll,
	"QueryNS":    _dnsQueryNS,
	"QueryTXT":   _dnsQueryTxt,
	"QuertAxfr":  _dnsQueryAxfr,
	"QueryAxfr":  _dnsQueryAxfr,

	"timeout":    _dnsConfigOpt_WithTimeout,
	"dnsServers": _dnsConfigOpt_WithDNSServers,
}
