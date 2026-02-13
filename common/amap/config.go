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

// WithLowhttpOptions sets the lowhttp options for the Amap API client.
func WithLowhttpOptions(opts ...poc.PocConfigOption) AmapConfigOption {
	return func(c *Config) {
		c.lowhttpOptions = append(c.lowhttpOptions, opts...)
	}
}

// WithGeocodeFilter sets the geocode filter for the Amap API client.
func WithGeocodeFilter(filter func(geocodes []*GeocodeResult) *GeocodeResult) AmapConfigOption {
	return func(c *Config) {
		c.GeocodeFilter = filter
	}
}

// WithEnableWeatherForecast sets the enable weather forecast in the config.
func WithEnableWeatherForecast(enable bool) AmapConfigOption {
	return func(c *Config) {
		c.EnableWeatherForecast = enable
	}
}

// WithApiKey sets the API key in the config.
func WithApiKey(apiKey string) AmapConfigOption {
	return func(c *Config) {
		c.ApiKey = apiKey
	}
}

// WithTimeout sets the HTTP client timeout in the config.
func WithTimeout(timeout time.Duration) AmapConfigOption {
	return func(c *Config) {
		c.Timeout = timeout
	}
}

// WithBaseURL sets the base URL in the config.
func WithBaseURL(baseURL string) AmapConfigOption {
	return func(c *Config) {
		c.BaseURL = baseURL
	}
}

// WithCity sets the city for API requests
func WithCity(city string) AmapConfigOption {
	return func(c *Config) {
		c.City = city
	}
}

// WithExtensions sets the extensions parameter (base or all)
func WithExtensions(extensions string) AmapConfigOption {
	return func(c *Config) {
		c.Extensions = extensions
	}
}

// WithPage sets the page number for paginated results
func WithPage(page int) AmapConfigOption {
	return func(c *Config) {
		c.Page = page
	}
}

// WithPageSize sets the page size for paginated results
func WithPageSize(pageSize int) AmapConfigOption {
	return func(c *Config) {
		c.PageSize = pageSize
	}
}

// WithType sets the type parameter for distance calculations
func WithType(typ string) AmapConfigOption {
	return func(c *Config) {
		c.Type = typ
	}
}

// WithRadius sets the radius for nearby searches
func WithRadius(radius int) AmapConfigOption {
	return func(c *Config) {
		c.Radius = radius
	}
}

// WithSortRule sets the sort rule for search results
func WithSortRule(sortRule string) AmapConfigOption {
	return func(c *Config) {
		c.SortRule = sortRule
	}
}
