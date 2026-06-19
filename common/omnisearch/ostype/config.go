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

// WithPage 设置搜索结果页码（导出名为 omnisearch.page）
// 参数:
//   - page: 页码，从 1 开始
//
// 返回值:
//   - 搜索可选项
//
// Example:
// ```
// // 示意性示例，需要有效的 apikey 与网络
// results = omnisearch.Search("yaklang", omnisearch.page(2))~
// ```
func WithPage(page int) SearchOption {
	return func(o *SearchConfig) {
		o.Page = page
	}
}

// WithBaseURL 设置搜索服务的基础 URL（导出名为 omnisearch.baseurl）
// 参数:
//   - baseURL: 搜索后端服务地址
//
// 返回值:
//   - 搜索可选项
//
// Example:
// ```
// // 示意性示例，需要有效的搜索后端
// results = omnisearch.Search("yaklang", omnisearch.baseurl("https://api.example.com"))~
// ```
func WithBaseURL(baseURL string) SearchOption {
	return func(o *SearchConfig) {
		o.BaseURL = baseURL
	}
}

// WithTimeout 设置搜索超时时间（导出名为 omnisearch.timeout）
// 参数:
//   - timeout: 超时时间
//
// 返回值:
//   - 搜索可选项
//
// Example:
// ```
// // 示意性示例，需要有效的 apikey 与网络
// results = omnisearch.Search("yaklang", omnisearch.timeout(10))~
// ```
func WithTimeout(timeout time.Duration) SearchOption {
	return func(o *SearchConfig) {
		o.Timeout = timeout
	}
}

// WithPageSize 设置每页结果数量（导出名为 omnisearch.pagesize）
// 参数:
//   - pageSize: 每页结果数量
//
// 返回值:
//   - 搜索可选项
//
// Example:
// ```
// // 示意性示例，需要有效的 apikey 与网络
// results = omnisearch.Search("yaklang", omnisearch.pagesize(20))~
// ```
func WithPageSize(pageSize int) SearchOption {
	return func(o *SearchConfig) {
		o.PageSize = pageSize
	}
}

// WithProxy 设置搜索请求代理（导出名为 omnisearch.proxy）
// 参数:
//   - proxy: 代理地址，如 http://127.0.0.1:7890
//
// 返回值:
//   - 搜索可选项
//
// Example:
// ```
// // 示意性示例，需要有效的 apikey 与网络
// results = omnisearch.Search("yaklang", omnisearch.proxy("http://127.0.0.1:7890"))~
// ```
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
		Page:     1,
		PageSize: 10,
		// Default SearcherType to "aibalance" to avoid empty searcher lookup failure
		// in OmniSearchClient.Search which does not implement auto-selection
		SearcherType: "aibalance",
		Extra:        map[string]interface{}{},
	}
	for _, option := range options {
		option(config)
	}
	return config
}
