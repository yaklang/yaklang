package fp

import (
	"context"
	"fmt"
	"io/ioutil"
	"time"
	"github.com/yaklang/yaklang/common/filter"
	"github.com/yaklang/yaklang/common/fp/webfingerprint"
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
	// ForceEnableWebFingerprint 表示强制检测 Web 指纹
	ForceEnableWebFingerprint bool

	// OnlyEnableWebFingerprint 表示值进行 Web 指纹识别
	//    这个选项为 True 的时候，行为将会覆盖 ForceEnableWebFingerprint
	OnlyEnableWebFingerprint bool

	// 禁用专门的 Web 指纹扫描
	DisableWebFingerprint bool

	// 这个选项标志着，如果 Web 指纹检测中途已经检测出了某些指纹，也应该继续检测其他指纹
	WebFingerprintUseAllRules bool

	// 爬虫发现的最大 URL 数量，默认是 5 个
	CrawlerMaxUrlCount int

	// 使用指定的 WebRule 来测试 Web 指纹，默认为使用默认指纹
	WebFingerprintRules []*webfingerprint.WebRule

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
	ExcludePortsFilter *filter.StringFilter
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
		if c.ExcludePortsFilter.Exist(fmt.Sprint(port)) {
			return true
		}
	}
	return false
}

func WithCache(b bool) ConfigOption {
	return func(config *Config) {
		config.EnableCache = b
	}
}

func WithDatabaseCache(b bool) ConfigOption {
	return func(config *Config) {
		config.EnableDatabaseCache = b
	}
}

func WithProxy(proxies ...string) ConfigOption {
	return func(config *Config) {
		config.Proxies = utils.StringArrayFilterEmpty(proxies)
	}
}

func WithPoolSize(size int) ConfigOption {
	return func(config *Config) {
		config.PoolSize = size
		if config.PoolSize <= 0 {
			config.PoolSize = 50
		}
	}
}

func WithExcludeHosts(hosts string) ConfigOption {
	return func(config *Config) {
		config.ExcludeHostsFilter = hostsparser.NewHostsParser(context.Background(), hosts)
	}
}

func WithExcludePorts(ports string) ConfigOption {
	return func(config *Config) {
		config.ExcludePortsFilter = filter.NewFilter()
		for _, port := range utils.ParseStringToPorts(ports) {
			config.ExcludePortsFilter.Insert(fmt.Sprint(port))
		}
	}
}

func (f *Config) GenerateWebFingerprintConfigOptions() []webfingerprint.ConfigOption {
	return []webfingerprint.ConfigOption{
		webfingerprint.WithActiveMode(f.ActiveMode),
		webfingerprint.WithForceAllRuleMatching(f.WebFingerprintUseAllRules),
		webfingerprint.WithProbeTimeout(f.ProbeTimeout),
		webfingerprint.WithWebFingerprintRules(f.WebFingerprintRules),
		webfingerprint.WithWebProxy(f.Proxies...),
	}
}

func NewConfig(options ...ConfigOption) *Config {
	config := &Config{}
	config.init()

	for _, p := range options {
		p(config)
	}
	if len(config.Proxies) <= 0 && utils.GetProxyFromEnv() != "" {
		WithProxy(utils.GetProxyFromEnv())(config)
	}

	config.lazyInit()
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
	c.ForceEnableWebFingerprint = false
	c.WebFingerprintUseAllRules = true
	c.CrawlerMaxUrlCount = 5
	c.PoolSize = 20

	c.FingerprintDataSize = 20480
}

func (c *Config) lazyInit() {
	if len(c.WebFingerprintRules) <= 0 {
		c.WebFingerprintRules, _ = GetDefaultWebFingerprintRules()
	}

	if len(c.FingerprintRules) <= 0 {
		c.FingerprintRules, _ = GetDefaultNmapServiceProbeRules()
	}
}

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

func WithProbeTimeoutHumanRead(f float64) ConfigOption {
	return func(config *Config) {
		config.ProbeTimeout = utils.FloatSecondDuration(f)
		if config.ProbeTimeout <= 0 {
			config.ProbeTimeout = 10 * time.Second
		}
	}
}

func WithForceEnableWebFingerprint(b bool) ConfigOption {
	return func(config *Config) {
		config.ForceEnableWebFingerprint = b
	}
}

func WithOnlyEnableWebFingerprint(b bool) ConfigOption {
	return func(config *Config) {
		config.OnlyEnableWebFingerprint = b
		if b {
			config.ForceEnableWebFingerprint = true
		}
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

func WithWebFingerprintRule(i interface{}) ConfigOption {
	var rules []*webfingerprint.WebRule
	switch ret := i.(type) {
	case []byte:
		rules, _ = webfingerprint.ParseWebFingerprintRules(ret)
	case string:
		e := utils.GetFirstExistedPath(ret)
		if e != "" {
			raw, _ := ioutil.ReadFile(e)
			rules, _ = webfingerprint.ParseWebFingerprintRules(raw)
		} else {
			rules, _ = webfingerprint.ParseWebFingerprintRules([]byte(ret))
		}
	case []*webfingerprint.WebRule:
		rules = ret
	}

	return func(config *Config) {
		if rules == nil {
			return
		}

		config.WebFingerprintRules = rules
	}
}

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
