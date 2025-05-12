package amap

import (
	"time"

	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

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
