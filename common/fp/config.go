package fp

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"io/ioutil"
	"sync"
	"time"

	"github.com/yaklang/yaklang/common/fp/fingerprint/parsers"
	"github.com/yaklang/yaklang/common/fp/fingerprint/rule"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/hostsparser"
)

type ConfigOption func(config *Config)

type Config struct {
	// 针对那种传输层协议进行指纹识别？
	TransportProtos []TransportProto

	// 在主动模式发包的基础上进行探测控制
	// 稀有度越大，表示这个服务在现实存在的可能性越小
	// 取值范围为1-9
	// 默认值为 5
	RarityMax int

	/*
		Probe 为主动发送一些数据包来检测指纹信息的机制，以下选项可以控制 Probe 的使用
	*/
	// 主动模式，这个模式下，会主动发包进行探测指纹，（启用 Probe）
	// 默认值为 false
	ActiveMode bool

	// 默认每一个 Probe 的超时时间
	ProbeTimeout time.Duration

	// 发送 Probe 的数量限制，默认值为 5
	ProbesMax int

	// 发送 Probe 的并发量，默认值为 5
	ProbesConcurrentMax int

	// 指定规则
	FingerprintRules map[*NmapProbe][]*NmapMatch

	// 指纹检测时候取的数据大小，意味着多大的数据会参与到指纹识别中
	// 2048 为默认值
	// 主机指纹识别的时间与这个值成正比
	FingerprintDataSize int

	/* Web Fingerprint */
	// Active Mode 这里的 ActiveMode 应该和当前配置保持一致，所以暂时不需要设置

	//
	// ForceEnableAllFingerprint 表示强制检测 Web 指纹
	ForceEnableAllFingerprint bool

	// OnlyEnableWebFingerprint 表示值进行 Web 指纹识别
	//    这个选项为 True 的时候，行为将会覆盖 ForceEnableAllFingerprint
	OnlyEnableWebFingerprint bool

	// 禁用专门的 Web 指纹扫描
	DisableWebFingerprint bool

	// 这个选项标志着，如果 Web 指纹检测中途已经检测出了某些指纹，也应该继续检测其他指纹
	WebFingerprintUseAllRules bool

	// 爬虫发现的最大 URL 数量，默认是 5 个
	CrawlerMaxUrlCount int

	// 使用指定的 WebRule 来测试 Web 指纹，默认为使用默认指纹
	WebFingerprintRules []*rule.FingerPrintRule

	// 并发池的大小配置（单体不生效）
	PoolSize int

	// 为端口扫描设置代理
	Proxies []string

	// 在同一个引擎进程内，可以缓存
	EnableCache bool

	// 设置数据库缓存，可以跨进程
	EnableDatabaseCache bool

	// Exclude
	ExcludeHostsFilter *hostsparser.HostsParser
	ExcludePortsFilter *utils.PortsFilter

	// Runtime id
	RuntimeId string

	DebugLog bool

	OpenPortSyncMap    *sync.Map
	OnPortOpenCallback func(*MatchResult)
	OnFinishedCallback func(*MatchResult)

	// ctx
	Ctx context.Context

	// Disable default fingerprint
	DisableDefaultFingerprint    bool
	DisableDefaultIotFingerprint bool

	// once
	fingerprintRulesOnce    sync.Once
	webFingerprintRulesOnce sync.Once

	WebScanDisableConnPool bool // 是否禁用 Web 扫描的连接池，默认值为 false
}

func (c *Config) IsFiltered(host string, port int) bool {
	if c == nil {
		return false
	}
	if c.ExcludeHostsFilter != nil {
		if c.ExcludeHostsFilter.Contains(host) {
			return true
		}
	}

	if c.ExcludePortsFilter != nil {
		if c.ExcludePortsFilter.Contains(port) {
			return true
		}
	}
	return false
}

// debugLog 的配置选项，设置本次扫描是否使用 debugLog
// @param {bool} b 是否使用 debugLog
// @return {ConfigOption} 返回配置项
// Example:
// ```
// result, err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.debugLog(true))
// die(err)
//
//	for v := range result {
//		println(v.String())
//	}
//
// ```
func WithDebugLog(b ...bool) ConfigOption {
	return func(config *Config) {
		if len(b) > 0 {
			config.DebugLog = b[0]
		} else {
			config.DebugLog = true
		}
	}
}

// cache servicescan 的配置选项，设置本次扫描是否使用缓存
// @param {bool} b 是否使用缓存
// @return {ConfigOption} 返回配置项
// Example:
// ```
// result, err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.cache(true))
// die(err)
//
//	for v := range result {
//		println(v.String())
//	}
//
// ```
func WithCache(b bool) ConfigOption {
	return func(config *Config) {
		config.EnableCache = b
	}
}

// onOpen servicescan 的配置选项，设置本次扫描端口开放时的回调函数
// @param {func(*MatchResult)} cb 回调函数
// @return {ConfigOption} 返回配置项
// Example:
// ```
// result, err := servicescan.Scan("127.0.0.1", "22,80,443", servicescan.onOpen(result => dump(result.String())))
// die(err)
//
//	for i in result {
//			println(i.String())
//		}
//
// ```
func WithOnPortOpenCallback(cb func(*MatchResult)) ConfigOption {
	return func(config *Config) {
		config.OnPortOpenCallback = cb
	}
}

// onFinish servicescan 的配置选项，设置本次扫描端口开放时的回调函数
// @param {func(*MatchResult)} cb 回调函数
// @return {ConfigOption} 返回配置项
// Example:
// ```
//
//	result, err := servicescan.Scan("127.0.0.1", "22,80,443", servicescan.onFinish(result => dump(result.String())))
//	die(err)
//	for i in result {
//		println(i.String())
//	}
//
// ```
func WithOnFinished(cb func(*MatchResult)) ConfigOption {
	return func(config *Config) {
		config.OnFinishedCallback = cb
	}
}

// databaseCache servicescan 的配置选项，设置本次扫描是否使用数据库缓存
// @param {bool} b 是否使用数据库缓存
// @return {ConfigOption} 返回配置项
// Example:
// ```
// result, err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.databaseCache(true))
// die(err)
//
//	for v := range result {
//		println(v.String())
//	}
//
// ```
func WithDatabaseCache(b bool) ConfigOption {
	return func(config *Config) {
		config.EnableDatabaseCache = b
	}
}

// proxy servicescan 的配置选项，设置本次扫描使用的代理
// @param {string} proxies 代理地址，支持 http 和 socks5
// @return {ConfigOption} 返回配置项
// Example:
// ```
// result, err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.proxy("http://127.0.0.1:1080"))
// die(err)
//
//	for v := range result {
//		println(v.String())
//	}
//
// ```
func WithProxy(proxies ...string) ConfigOption {
	return func(config *Config) {
		config.Proxies = utils.StringArrayFilterEmpty(proxies)
	}
}

// concurrent servicescan 的配置选项，用于设置整体扫描并发
// @param {int} size 并发数量
// @return {ConfigOption} 返回配置项
// Example:
// ```
// result, err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.concurrent(100))
// die(err)
//
//	for v := range result {
//		println(v.String())
//	}
//
// ```
func WithPoolSize(size int) ConfigOption {
	return func(config *Config) {
		config.PoolSize = size
		if config.PoolSize <= 0 {
			config.PoolSize = 50
		}
	}
}

// excludeHosts servicescan 的配置选项，设置本次扫描排除的主机
// @param {string} hosts 主机，支持逗号分割、CIDR、-的格式
// @return {ConfigOption} 返回配置项
// Example:
// ```
// result, err = servicescan.Scan("192.168.1.1/24", "22-80,443,3389", servicescan.excludeHosts("192.168.1.1"))
// die(err)
//
//	for v := range result {
//		println(v.String())
//	}
//
// ```
func WithExcludeHosts(hosts string) ConfigOption {
	return func(config *Config) {
		config.ExcludeHostsFilter = hostsparser.NewHostsParser(context.Background(), hosts)
	}
}

// excludePorts servicescan 的配置选项，设置本次扫描排除的端口
// @param {string} ports 端口，支持逗号分割、-的格式
// @return {ConfigOption} 返回配置项
// Example:
// ```
// result, err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.excludePorts("22,80"))
// die(err)
//
//	for v := range result {
//		println(v.String())
//	}
//
// ```
func WithExcludePorts(ports string) ConfigOption {
	return func(config *Config) {
		config.ExcludePortsFilter = utils.NewPortsFilter(ports)
	}
}

func WithRuntimeId(id string) ConfigOption {
	return func(config *Config) {
		config.RuntimeId = id
	}
}

func WithCtx(ctx context.Context) ConfigOption {
	return func(config *Config) {
		config.Ctx = ctx
	}
}

//func (f *Config) GenerateWebFingerprintConfigOptions() []webfingerprint.ConfigOption {
//	return []webfingerprint.ConfigOption{
//		webfingerprint.WithActiveMode(f.ActiveMode),
//		webfingerprint.WithForceAllRuleMatching(f.WebFingerprintUseAllRules),
//		webfingerprint.WithProbeTimeout(f.ProbeTimeout),
//		//webfingerprint.WithWebFingerprintRules(f.WebFingerprintRules),
//		webfingerprint.WithWebFingerprintDataSize(f.FingerprintDataSize),
//		webfingerprint.WithWebProxy(f.Proxies...),
//		webfingerprint.WithRuntimeId(f.RuntimeId),
//	}
//}

func NewConfig(options ...ConfigOption) *Config {
	config := &Config{}
	config.init()

	for _, p := range options {
		p(config)
	}

	return config
}

func (c *Config) Configure(ops ...ConfigOption) {
	for _, op := range ops {
		op(c)
	}
}

func (c *Config) init() {
	c.TransportProtos = []TransportProto{TCP}
	c.ActiveMode = true
	c.RarityMax = 5
	c.ProbesMax = 5
	c.ProbeTimeout = 5 * time.Second
	c.ProbesConcurrentMax = 5
	c.OnlyEnableWebFingerprint = false
	c.DisableWebFingerprint = false
	c.ForceEnableAllFingerprint = false
	c.WebFingerprintUseAllRules = true
	c.CrawlerMaxUrlCount = 5
	c.PoolSize = 20

	c.FingerprintDataSize = 20480
	c.Ctx = context.Background()
}

func (c *Config) GetWebFingerprintRules() []*rule.FingerPrintRule {
	c.webFingerprintRulesOnce.Do(func() {
		if !c.DisableDefaultFingerprint {
			webFingerprintRules, _ := GetDefaultWebFingerprintRules()
			c.WebFingerprintRules = append(webFingerprintRules, c.WebFingerprintRules...)
		}
	})

	return c.WebFingerprintRules
}

func (c *Config) GetFingerprintRules() map[*NmapProbe][]*NmapMatch {
	c.fingerprintRulesOnce.Do(func() {
		if len(c.FingerprintRules) <= 0 {
			c.FingerprintRules, _ = GetDefaultNmapServiceProbeRules()
		}
	})

	return c.FingerprintRules
}

// maxProbes servicescan 的配置选项，在主动模式发包的基础上设置本次扫描使用的最大探测包数量，默认值为 5
// @param {int} m 最大探测包数量
// @return {ConfigOption} 返回配置项
// Example:
// ```
// result, err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161",
// servicescan.active(true), // 需要在主动发包的基础上
// servicescan.maxProbes(10)
// )
// die(err)
//
//	for v := range result {
//		println(v.String())
//	}
//
// ```
func WithProbesMax(m int) ConfigOption {
	if m <= 0 {
		m = 5
	}

	return func(config *Config) {
		config.ProbesMax = m
	}
}

func ParseStringToProto(protos ...interface{}) []TransportProto {
	var ret []TransportProto

	var raw []string
	for _, proto := range protos {
		raw = append(raw, utils.ToLowerAndStrip(fmt.Sprint(proto)))
	}

	if utils.StringSliceContain(raw, "tcp") {
		ret = append(ret, TCP)
	}

	if utils.StringSliceContain(raw, "udp") {
		ret = append(ret, UDP)
	}

	return ret
}

// maxProbesConcurrent servicescan 的配置选项，设置本次扫描发送 Probe 的并发量，默认值为 5
// @param {int} m 并发量
// @return {ConfigOption} 返回配置项
// Example:
// ```
// result, err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161",
// servicescan.active(true), // 需要在主动发包的基础上
// servicescan.maxProbes(50), // 设置本次扫描使用的最大探测包数量
// servicescan.maxProbesConcurrent(10) // 设置本次扫描发送 Probe 的并发量
// )
// die(err)
//
//	for v := range result {
//		println(v.String())
//	}
//
// ```
func WithProbesConcurrentMax(m int) ConfigOption {
	if m <= 0 {
		m = 5
	}

	return func(config *Config) {
		config.ProbesConcurrentMax = m
	}
}

func WithTransportProtos(protos ...TransportProto) ConfigOption {
	r := map[TransportProto]int{}
	for _, p := range protos {
		r[p] = 1
	}

	var results []TransportProto
	for key := range r {
		results = append(results, key)
	}

	if results == nil {
		results = []TransportProto{TCP}
	}

	return func(config *Config) {
		config.TransportProtos = results
	}
}

// nmapRarityMax servicescan 的配置选项，设置本次扫描使用的 Nmap 指纹稀有度，在主动模式发包的基础上进行探测控制
// 稀有度越大，表示这个服务在现实存在的可能性越小，取值范围为 1-9，默认值为 5
// @param {int} rarity 稀有度，取值范围为 1-9
// @return {ConfigOption} 返回配置项
// Example:
// ```
// result, err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161",
// servicescan.active(true), // 需要在主动发包的基础上通过稀有度进行筛选
// servicescan.nmapRarityMax(9),
// )
// die(err)
//
//	for v := range result {
//		println(v.String())
//	}
//
// ```
func WithRarityMax(rarity int) ConfigOption {
	return func(config *Config) {
		config.RarityMax = rarity
	}
}

func WithActiveMode(raw bool) ConfigOption {
	return func(config *Config) {
		config.ActiveMode = raw
	}
}

func WithProbeTimeout(timeout time.Duration) ConfigOption {
	return func(config *Config) {
		config.ProbeTimeout = timeout
	}
}

// probeTimeout servicescan 的配置选项，设置每一个探测包的超时时间
// @param {float64} f 超时时间，单位为秒
// @return {ConfigOption} 返回配置项
// Example:
// ```
// result, err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.probeTimeout(5))
// die(err)
//
//	for v := range result {
//		println(v.String())
//	}
//
// ```
func WithProbeTimeoutHumanRead(f float64) ConfigOption {
	return func(config *Config) {
		config.ProbeTimeout = utils.FloatSecondDuration(f)
		if config.ProbeTimeout <= 0 {
			config.ProbeTimeout = 10 * time.Second
		}
	}
}

func WithForceEnableAllFingerprint(b bool) ConfigOption {
	return func(config *Config) {
		config.ForceEnableAllFingerprint = b
	}
}

func WithOnlyEnableWebFingerprint(b bool) ConfigOption {
	return func(config *Config) {
		config.OnlyEnableWebFingerprint = b
	}
}

func WithFingerprintRule(rules map[*NmapProbe][]*NmapMatch) ConfigOption {
	return func(config *Config) {
		config.FingerprintRules = rules
	}
}

func WithDisableWebFingerprint(t bool) ConfigOption {
	return func(config *Config) {
		config.DisableWebFingerprint = t
	}
}

func WithDisableIotWebFingerprint(t bool) ConfigOption {
	return func(config *Config) {
		config.DisableDefaultIotFingerprint = t
	}
}

func WithFingerprintDataSize(size int) ConfigOption {
	return func(config *Config) {
		config.FingerprintDataSize = size
	}
}

func WithWebFingerprintUseAllRules(b bool) ConfigOption {
	return func(config *Config) {
		config.WebFingerprintUseAllRules = b
	}
}

// webRule servicescan 的配置选项，设置本次扫描使用的 Web 指纹规则
// @param {interface{}} i Web 指纹规则
func WithWebFingerprintRule(rs ...any) ConfigOption {
	var allRules []*rule.FingerPrintRule
	for _, i := range rs {
		var rules []*rule.FingerPrintRule
		switch ret := i.(type) {
		case []byte:
			rules, _ = parsers.ParseYamlRule(string(ret))
		case string:
			e := utils.GetFirstExistedPath(ret)
			if e != "" {
				raw, _ := ioutil.ReadFile(e)
				rules, _ = parsers.ParseYamlRule(string(raw))
			} else {
				rules, _ = parsers.ParseYamlRule(ret)
			}
		case []*rule.FingerPrintRule:
			rules = ret
		}
		allRules = append(allRules, rules...)
	}

	return func(config *Config) {
		if allRules == nil {
			return
		}
		config.WebFingerprintRules = append(config.WebFingerprintRules, allRules...)
	}
}

// service servicescan 的配置选项，用于指定指纹库中的指纹组。
// @return {ConfigOption} 返回配置选项
// Example:
// ```
// result,err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.withRuleGroup("group1","group2")) // 使用"group1"和"group2"指纹组的指纹进行扫描
// die(err) // 如果错误非空则报错
// for res := range result { // 通过遍历管道的形式获取管道中的结果，一旦有结果返回就会执行循环体的代码
//
//	   println(res.String()) // 输出结果，调用String方法获取可读字符串
//	}
//
// ```
func WithFingerprintRuleGroup(groups ...string) ConfigOption {
	rules, err := yakit.QueryGeneralRuleByGroup(consts.GetGormProfileDatabase(), groups...)
	if err != nil {
		log.Errorf("query fingerprint rule by group %v failed: %s", groups, err)
	}
	allRules, err := parsers.ParseExpRule(rules...)
	if err != nil {
		log.Errorf("parse fingerprint rule by group %v failed: %s", groups, err)
	}
	return func(config *Config) {
		if allRules == nil {
			return
		}
		config.WebFingerprintRules = append(config.WebFingerprintRules, allRules...)
	}
}

// service servicescan 的配置选项，用于指定使用指纹组的全部指纹。
// @return {ConfigOption} 返回配置选项
// Example:
// ```
// result,err = servicescan.Scan("127.0.0.1", "22-80,443,3389,161", servicescan.withRuleGroupAll()) // 使用全部指纹组的指纹进行扫描
// die(err) // 如果错误非空则报错
// for res := range result { // 通过遍历管道的形式获取管道中的结果，一旦有结果返回就会执行循环体的代码
//
//	   println(res.String()) // 输出结果，调用String方法获取可读字符串
//	}
//
// ```
func WithFingerprintRuleGroupAll() ConfigOption {
	rules, err := yakit.QueryGeneralRuleFast(consts.GetGormProfileDatabase(), &ypb.FingerprintFilter{})
	if err != nil {
		log.Errorf("query fingerprint rule fast failed: %s", err)
	}
	allRules, err := parsers.ParseExpRule(rules...)
	if err != nil {
		log.Errorf("parse fingerprint rule fast failed: %s", err)
	}
	return func(config *Config) {
		if allRules == nil {
			return
		}
		config.WebFingerprintRules = append(config.WebFingerprintRules, allRules...)
	}
}

// nmapRule servicescan 的配置选项，设置本次扫描使用的 Nmap 指纹规则
// @param {interface{}} i Nmap 指纹规则
func WithNmapRule(i interface{}) ConfigOption {
	var nmapRules map[*NmapProbe][]*NmapMatch
	switch ret := i.(type) {
	case []byte:
		nmapRules, _ = ParseNmapServiceProbeToRuleMap(ret)
	case string:
		e := utils.GetFirstExistedPath(ret)
		if e != "" {
			raw, _ := ioutil.ReadFile(e)
			nmapRules, _ = ParseNmapServiceProbeToRuleMap(raw)
		} else {
			nmapRules, _ = ParseNmapServiceProbeToRuleMap([]byte(ret))
		}
	case map[*NmapProbe][]*NmapMatch:
		nmapRules = ret
	}
	return func(config *Config) {
		if nmapRules == nil {
			return
		}

		if len(nmapRules) <= 0 {
			return
		}
		config.FingerprintRules = nmapRules
	}
}
