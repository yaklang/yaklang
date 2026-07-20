package subdomain

import "time"

const (
	_ = iota
	BRUTE
	SEARCH
	ZONE_TRANSFER
)

type SubdomainScannerConfig struct {
	// 设置
	Modes []int

	// 允许递归扫描
	AllowToRecursive bool

	// DNS Worker Count
	// 同时允许多少个 DNS 查询 Goroutine
	WorkerCount int

	// DNS Servers 预设 DNS 服务器
	DNSServers []string

	// 子域名遍历的最大深度 // 默认为 5
	MaxDepth int

	// 平行执行的任务数量 Default: 10
	ParallelismTasksCount int

	// 每个目标的超时时间
	TimeoutForEachTarget time.Duration

	// 子域名爆破主字典
	MainDictionary []byte

	// 子域名爆破次级字典
	SubDictionary []byte

	// 是否开启自动 Web 发现？
	//	allow_auto_web_discover: bool = True
	// 自动发现 Web 不应该交由本模块实现
	// 应该在外部将子域名扫描结果交给爬虫模块去处理

	// 每一次查询的超时时间，默认值为 3s
	TimeoutForEachQuery time.Duration

	// 遇到泛解析的情况，马上停止解析
	//   这里有两种情况，第一种是自己设的泛解析，第二种是运营商设置的泛解析
	//   如果遇到泛解析不马上停止的话，子域名爆破会自动将泛解析到的 IP 地址添加进黑名单
	//   默认值为 false
	WildCardToStop bool

	// 泛解析探测次数
	//   检测泛解析时生成的随机不存在的子域名数量，默认为 10
	//   这些探测结果会用来判断是“经典泛解析”（全部命中且 IP 相同），
	//   还是“DNS 被劫持/接管”（全部命中但 IP 各不相同）。
	//   探测数量越大判断越稳，但耗时越长。
	WildCardProbeCount int

	// 泛解析 sinkhole 验证
	//   当泛解析探测全部命中且返回同一个 IP（经典泛解析特征）时，
	//   额外抽样几个真实子域名前缀（如 www/mail/ftp）验证：
	//     - 若抽样词也都解析到同一个 wildcard IP，说明 DNS 被劫持到
	//       单一 sinkhole IP（常见于本地 TUN 模式劫持 DNS），爆破无意义，
	//       会被判定为 WildcardHijacked 并中止。
	//     - 若抽样词解析到不同 IP 或解析失败（NXDOMAIN），说明是真实泛解析，
	//       保持 WildcardSingleIP 语义并按黑名单过滤。
	//   默认值为 true（默认生效）。
	WildCardSinkholeVerify bool

	// 进行各种数据源搜索的时候，需要设置的 HTTP 超时时间
	// 默认 10s
	TimeoutForEachHTTPSearch time.Duration
}

func (s *SubdomainScannerConfig) init() {
	s.Modes = []int{BRUTE, SEARCH, ZONE_TRANSFER}
	s.AllowToRecursive = true
	s.WorkerCount = 50
	s.DNSServers = []string{"114.114.114.114", "8.8.8.8"}
	s.MaxDepth = 5
	s.ParallelismTasksCount = 10
	s.TimeoutForEachTarget = 300 * time.Second
	s.MainDictionary = DefaultMainDictionary
	s.SubDictionary = DefaultSubDictionary
	s.TimeoutForEachQuery = 3 * time.Second
	s.WildCardToStop = false
	s.WildCardProbeCount = 10
	s.WildCardSinkholeVerify = true
	s.TimeoutForEachHTTPSearch = 10 * time.Second
}

type ConfigOption func(s *SubdomainScannerConfig)

// 配置子域名发现模式
func WithModes(modes ...int) ConfigOption {
	return func(s *SubdomainScannerConfig) {
		s.Modes = modes
	}
}

// recursive 是一个选项参数，设置是否递归扫描子域名，如果不递归扫描，那么只会扫描一层子域名，默认为false
// 参数:
//   - b: 是否递归扫描子域名
//
// 返回值:
//   - 一个 subdomain.Scan 可接收的配置选项
//
// Example:
// ```
// subdomain.Scan("example.com", subdomain.recursive(true))
// ```
func WithAllowToRecursive(b bool) ConfigOption {
	return func(s *SubdomainScannerConfig) {
		s.AllowToRecursive = b
	}
}

// workerConcurrent 是一个选项参数，设置总的工作线程数量，默认为 50
// 参数:
//   - c: 总的工作线程数量
//
// 返回值:
//   - 一个 subdomain.Scan 可接收的配置选项
//
// Example:
// ```
// subdomain.Scan("example.com", subdomain.workerConcurrent(10))
// ```
func WithWorkerCount(c int) ConfigOption {
	return func(s *SubdomainScannerConfig) {
		s.WorkerCount = c
	}
}

// dnsServer 是一个选项参数，设置用于解析域名的 DNS 服务器，默认为 114.114.114.114 和 8.8.8.8
// 参数:
//   - servers: 用于解析域名的 DNS 服务器列表
//
// 返回值:
//   - 一个 subdomain.Scan 可接收的配置选项
//
// Example:
// ```
// subdomain.Scan("example.com", subdomain.dnsServer("1.1.1.1"))
// ```
func WithDNSServers(servers []string) ConfigOption {
	return func(s *SubdomainScannerConfig) {
		s.DNSServers = servers
	}
}

// maxDepth 是一个选项参数，设置子域名遍历的最大深度，默认为 5，通常与 recursive 一起使用
// 参数:
//   - d: 子域名遍历的最大深度
//
// 返回值:
//   - 一个 subdomain.Scan 可接收的配置选项
//
// Example:
// ```
// subdomain.Scan("example.com", subdomain.maxDepth(10), subdomain.recursive(true))
// ```
func WithMaxDepth(d int) ConfigOption {
	return func(s *SubdomainScannerConfig) {
		s.MaxDepth = d
	}
}

// targetConcurrent 是一个选项参数，设置每个目标的最大线程数量，默认为 10
// 参数:
//   - c: 每个目标的最大线程数量
//
// 返回值:
//   - 一个 subdomain.Scan 可接收的配置选项
//
// Example:
// ```
// subdomain.Scan("example.com", subdomain.targetConcurrent(5))
// ```
func WithParallelismTasksCount(c int) ConfigOption {
	return func(s *SubdomainScannerConfig) {
		s.ParallelismTasksCount = c
	}
}

func WithTimeoutForEachTarget(t time.Duration) ConfigOption {
	return func(s *SubdomainScannerConfig) {
		s.TimeoutForEachTarget = t
	}
}

func WithMainDictionary(raw []byte) ConfigOption {
	return func(s *SubdomainScannerConfig) {
		s.MainDictionary = raw
	}
}

// recursiveDict
func WithSubDictionary(raw []byte) ConfigOption {
	return func(s *SubdomainScannerConfig) {
		s.SubDictionary = raw
	}
}

func WithTimeoutForEachQuery(timeout time.Duration) ConfigOption {
	return func(s *SubdomainScannerConfig) {
		s.TimeoutForEachQuery = timeout
	}
}

// wildcardToStop 是一个选项参数，遇到泛解析的情况，是否马上停止解析，默认为 false
// 参数:
//   - t: 遇到泛解析时是否马上停止解析
//
// 返回值:
//   - 一个 subdomain.Scan 可接收的配置选项
//
// Example:
// ```
// subdomain.Scan("example.com", subdomain.wildcardToStop(true))
// ```
func WithWildCardToStop(t bool) ConfigOption {
	return func(s *SubdomainScannerConfig) {
		s.WildCardToStop = t
	}
}

// wildcardProbeCount 是一个选项参数，设置泛解析探测时生成的随机不存在的子域名数量，默认为 10
// 参数:
//   - c: 泛解析探测次数，必须大于 0
//
// 返回值:
//   - 一个 subdomain.Scan 可接收的配置选项
//
// Example:
// ```
// subdomain.Scan("example.com", subdomain.wildcardProbeCount(20))
// ```
func WithWildCardProbeCount(c int) ConfigOption {
	return func(s *SubdomainScannerConfig) {
		if c <= 0 {
			c = 10
		}
		s.WildCardProbeCount = c
	}
}

// wildcardSinkholeVerify 是一个选项参数，是否在判定为经典泛解析（单 IP）
// 后再抽样几个真实子域名前缀验证，若全部解析到同一个 wildcard IP 则判定为
// DNS 被劫持到单一 sinkhole（如本地 TUN 模式劫持 DNS）并中止，默认为 true
// 参数:
//   - b: 是否开启 sinkhole 验证
//
// 返回值:
//   - 一个 subdomain.Scan 可接收的配置选项
//
// Example:
// ```
// subdomain.Scan("example.com", subdomain.wildcardSinkholeVerify(false))
// ```
func WithWildCardSinkholeVerify(b bool) ConfigOption {
	return func(s *SubdomainScannerConfig) {
		s.WildCardSinkholeVerify = b
	}
}

func WithTimeoutForEachHTTPSearch(timeout time.Duration) ConfigOption {
	return func(s *SubdomainScannerConfig) {
		s.TimeoutForEachHTTPSearch = timeout
	}
}

func NewSubdomainScannerConfig(options ...ConfigOption) *SubdomainScannerConfig {
	config := &SubdomainScannerConfig{}
	config.init()

	for _, option := range options {
		option(config)
	}
	return config
}
