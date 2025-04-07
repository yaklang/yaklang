package ostype

import (
	"time"
)

type SearchConfig struct {
	SearcherType SearcherType
	Page         int
	ApiKey       string
	PageSize     int
	Proxy        string
	BaseURL      string
	Timeout      time.Duration
	Extra        map[string]interface{}
}

type SearchOption func(*SearchConfig)

func WithApiKey(apiKey string) SearchOption {
	return func(o *SearchConfig) {
		o.ApiKey = apiKey
	}
}

func WithSearchType(searchType SearcherType) SearchOption {
	return func(o *SearchConfig) {
		o.SearcherType = searchType
	}
}

func WithPage(page int) SearchOption {
	return func(o *SearchConfig) {
		o.Page = page
	}
}

func WithBaseURL(baseURL string) SearchOption {
	return func(o *SearchConfig) {
		o.BaseURL = baseURL
	}
}

func WithTimeout(timeout time.Duration) SearchOption {
	return func(o *SearchConfig) {
		o.Timeout = timeout
	}
}

func WithPageSize(pageSize int) SearchOption {
	return func(o *SearchConfig) {
		o.PageSize = pageSize
	}
}

func WithProxy(proxy string) SearchOption {
	return func(o *SearchConfig) {
		o.Proxy = proxy
	}
}

func WithExtra(name string, val any) SearchOption {
	return func(o *SearchConfig) {
		o.Extra[name] = val
	}
}

func NewSearchConfig(options ...SearchOption) *SearchConfig {
	config := &SearchConfig{
		Page:         1,
		PageSize:     10,
		SearcherType: SearcherTypeBrave,
		Extra:        map[string]interface{}{},
	}
	for _, option := range options {
		option(config)
	}
	return config
}
