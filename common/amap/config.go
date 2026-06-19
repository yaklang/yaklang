package amap

import (
	"strings"
	"time"

	"github.com/yaklang/yaklang/common/aibalanceclient"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// deriveServerBase extracts the aibalance server base URL from an amap BaseURL.
// e.g. "http://127.0.0.1:8223/amap" -> "http://127.0.0.1:8223"
// e.g. "https://aibalance.yaklang.com/amap" -> "https://aibalance.yaklang.com"
func deriveServerBase(amapBaseURL string) string {
	s := strings.TrimSuffix(amapBaseURL, "/")
	s = strings.TrimSuffix(s, "/amap")
	s = strings.TrimSuffix(s, "/")
	return s
}

type AmapConfigOption func(*Config)

// Config holds the Amap API client configuration.
type Config struct {
	ApiKey         string
	Timeout        time.Duration
	BaseURL        string
	City           string
	Extensions     string
	Page           int
	PageSize       int
	Type           string
	Radius         int
	SortRule       string
	GeocodeFilter  func(geocodes []*GeocodeResult) *GeocodeResult
	lowhttpOptions []poc.PocConfigOption

	EnableWeatherForecast bool

	// aibalance proxy fields (internal)
	isAIBalanceProxy bool   // true when using aibalance proxy, enables TOTP retry
	proxyServerBase  string // aibalance server base URL without /amap, for TOTP refresh
}

// NewConfig returns a default config for the Amap API client.
func NewConfig(opts ...AmapConfigOption) *Config {
	cfg := &Config{
		BaseURL:    "https://restapi.amap.com",
		Timeout:    time.Second * 5,
		Extensions: "base",
		Page:       1,
		PageSize:   20,
		Radius:     3000,
		Type:       "driving",
		SortRule:   "distance",
		GeocodeFilter: func(geocodes []*GeocodeResult) *GeocodeResult {
			if len(geocodes) == 0 {
				return nil
			}
			return geocodes[0]
		},
	}
	for _, o := range opts {
		o(cfg)
	}
	if cfg.ApiKey == "" {
		key, err := LoadAmapKeywordFromYakit()
		if err != nil {
			log.Warnf("load amap apikey from yakit failed: %v", err)
		} else {
			cfg.ApiKey = key
		}
	}

	// Fallback: if no API key is configured, use the aibalance proxy
	if cfg.ApiKey == "" {
		log.Infof("no amap api key configured, falling back to aibalance proxy")
		// Only set fallback BaseURL if user hasn't explicitly set a custom one
		if cfg.BaseURL == "https://restapi.amap.com" {
			cfg.BaseURL = "https://aibalance.yaklang.com/amap"
		}
		cfg.ApiKey = "aibalance-proxy" // placeholder key, server will inject real key
		cfg.isAIBalanceProxy = true
		cfg.proxyServerBase = deriveServerBase(cfg.BaseURL)

		// Load TOTP header for authentication from the actual target server
		totpHeader, err := LoadAmapTOTPHeader(cfg.BaseURL)
		if err != nil {
			log.Warnf("failed to load TOTP header for amap proxy: %v", err)
		} else if totpHeader != "" {
			cfg.lowhttpOptions = append(cfg.lowhttpOptions, poc.WithReplaceHttpPacketHeader("X-Memfit-OTP-Auth", totpHeader))
		}

		// Add Trace-ID header for rate limiting identification
		traceID := aibalanceclient.GetTraceID()
		if traceID != "" {
			cfg.lowhttpOptions = append(cfg.lowhttpOptions, poc.WithReplaceHttpPacketHeader("Trace-ID", traceID))
		}
	}

	return cfg
}

// WithLowhttpOptions 设置高德 API 客户端底层 HTTP 请求选项（导出名为 amap.pocOpts）
// 参数:
//   - opts: 零个到多个 poc 请求选项函数
//
// 返回值:
//   - 高德 API 配置可选项
//
// Example:
// ```
// opt = amap.pocOpts(poc.timeout(10))
// println(opt)
// ```
func WithLowhttpOptions(opts ...poc.PocConfigOption) AmapConfigOption {
	return func(c *Config) {
		c.lowhttpOptions = append(c.lowhttpOptions, opts...)
	}
}

// WithGeocodeFilter 设置地理编码结果过滤器，用于从多个候选中选出一个（导出名为 amap.geocodeFilter）
// 参数:
//   - filter: 过滤回调，输入候选地理编码结果，返回选中的结果
//
// 返回值:
//   - 高德 API 配置可选项
//
// Example:
// ```
// opt = amap.geocodeFilter(func(geocodes) { return geocodes[0] })
// println(opt)
// ```
func WithGeocodeFilter(filter func(geocodes []*GeocodeResult) *GeocodeResult) AmapConfigOption {
	return func(c *Config) {
		c.GeocodeFilter = filter
	}
}

// WithEnableWeatherForecast 设置天气查询是否返回预报信息（导出名为 amap.enableWeatherForecast）
// 参数:
//   - enable: 是否启用天气预报
//
// 返回值:
//   - 高德 API 配置可选项
//
// Example:
// ```
// opt = amap.enableWeatherForecast(true)
// println(opt)
// ```
func WithEnableWeatherForecast(enable bool) AmapConfigOption {
	return func(c *Config) {
		c.EnableWeatherForecast = enable
	}
}

// WithApiKey 设置高德开放平台 API Key（导出名为 amap.apiKey）
// 参数:
//   - apiKey: 高德开放平台申请的 API Key
//
// 返回值:
//   - 高德 API 配置可选项
//
// Example:
// ```
// opt = amap.apiKey("your-amap-api-key")
// println(opt)
// ```
func WithApiKey(apiKey string) AmapConfigOption {
	return func(c *Config) {
		c.ApiKey = apiKey
	}
}

// WithTimeout 设置 HTTP 请求超时时间（导出名为 amap.timeout）
// 参数:
//   - timeout: 超时时间（time.Duration）
//
// 返回值:
//   - 高德 API 配置可选项
//
// Example:
// ```
// opt = amap.timeout(10 * time.Second)
// println(opt)
// ```
func WithTimeout(timeout time.Duration) AmapConfigOption {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithBaseURL 设置高德 API 的基础 URL（导出名为 amap.baseURL）
// 参数:
//   - baseURL: API 基础地址
//
// 返回值:
//   - 高德 API 配置可选项
//
// Example:
// ```
// opt = amap.baseURL("https://restapi.amap.com")
// println(opt)
// ```
func WithBaseURL(baseURL string) AmapConfigOption {
	return func(c *Config) {
		c.BaseURL = baseURL
	}
}

// WithCity 设置请求关联的城市（导出名为 amap.city）
// 参数:
//   - city: 城市名或 citycode
//
// 返回值:
//   - 高德 API 配置可选项
//
// Example:
// ```
// opt = amap.city("北京")
// println(opt)
// ```
func WithCity(city string) AmapConfigOption {
	return func(c *Config) {
		c.City = city
	}
}

// WithExtensions 设置返回结果的详细程度（base 或 all，导出名为 amap.extensions）
// 参数:
//   - extensions: 取值 base 或 all
//
// 返回值:
//   - 高德 API 配置可选项
//
// Example:
// ```
// opt = amap.extensions("all")
// println(opt)
// ```
func WithExtensions(extensions string) AmapConfigOption {
	return func(c *Config) {
		c.Extensions = extensions
	}
}

// WithPage 设置分页结果的页码（导出名为 amap.page）
// 参数:
//   - page: 页码（从 1 开始）
//
// 返回值:
//   - 高德 API 配置可选项
//
// Example:
// ```
// opt = amap.page(1)
// println(opt)
// ```
func WithPage(page int) AmapConfigOption {
	return func(c *Config) {
		c.Page = page
	}
}

// WithPageSize 设置分页结果的每页数量（导出名为 amap.pageSize）
// 参数:
//   - pageSize: 每页数量
//
// 返回值:
//   - 高德 API 配置可选项
//
// Example:
// ```
// opt = amap.pageSize(20)
// println(opt)
// ```
func WithPageSize(pageSize int) AmapConfigOption {
	return func(c *Config) {
		c.PageSize = pageSize
	}
}

// WithType 设置类型参数（如距离计算的类型，导出名为 amap.type）
// 参数:
//   - typ: 类型参数
//
// 返回值:
//   - 高德 API 配置可选项
//
// Example:
// ```
// opt = amap.type("1")
// println(opt)
// ```
func WithType(typ string) AmapConfigOption {
	return func(c *Config) {
		c.Type = typ
	}
}

// WithRadius 设置周边搜索的半径（导出名为 amap.radius）
// 参数:
//   - radius: 搜索半径（米）
//
// 返回值:
//   - 高德 API 配置可选项
//
// Example:
// ```
// opt = amap.radius(1000)
// println(opt)
// ```
func WithRadius(radius int) AmapConfigOption {
	return func(c *Config) {
		c.Radius = radius
	}
}

// WithSortRule 设置搜索结果的排序规则（导出名为 amap.sortRule）
// 参数:
//   - sortRule: 排序规则，如 distance/weight
//
// 返回值:
//   - 高德 API 配置可选项
//
// Example:
// ```
// opt = amap.sortRule("distance")
// println(opt)
// ```
func WithSortRule(sortRule string) AmapConfigOption {
	return func(c *Config) {
		c.SortRule = sortRule
	}
}
