package omnisearch

import "github.com/yaklang/yaklang/common/omnisearch/ostype"

// SearchKeyInfo API密钥信息，包含API key、类型和使用计数
type SearchKeyInfo struct {
	ApiKey   string              // API密钥
	Type     ostype.SearcherType // 搜索引擎类型
	HitCount int                 // 使用次数计数
}

// OmniSearchConfig 全能搜索客户端配置
type OmniSearchConfig struct {
	searchKeys     []*SearchKeyInfo                            // API密钥列表
	extSearcherMap map[ostype.SearcherType]ostype.SearchClient // 扩展搜索引擎映射
}

// NewOmniSearchConfig 创建新的配置实例
func NewOmniSearchConfig(opts ...OmniSearchConfigOption) *OmniSearchConfig {
	config := &OmniSearchConfig{
		extSearcherMap: map[ostype.SearcherType]ostype.SearchClient{},
	}
	for _, opt := range opts {
		opt(config)
	}
	return config
}

// OmniSearchConfigOption 配置选项函数类型
type OmniSearchConfigOption func(*OmniSearchConfig)

// WithSearchKeys 添加API密钥配置选项
func WithSearchKeys(searchKey ...*SearchKeyInfo) OmniSearchConfigOption {
	return func(config *OmniSearchConfig) {
		config.searchKeys = append(config.searchKeys, searchKey...)
	}
}

// WithExtSearcher 添加扩展搜索引擎配置选项
func WithExtSearcher(searcher ostype.SearchClient) OmniSearchConfigOption {
	return func(config *OmniSearchConfig) {
		config.extSearcherMap[searcher.GetType()] = searcher
	}
}
