package omnisearch

import (
	"fmt"
	"sort"
	"sync"

	"github.com/yaklang/yaklang/common/omnisearch/searchers"

	"github.com/yaklang/yaklang/common/omnisearch/ostype"
)

var (
	searcherList = map[ostype.SearcherType]ostype.SearchClient{} // 存储各种类型的搜索引擎实例
)

func RegisterSearchers(searcher ...ostype.SearchClient) {
	for _, s := range searcher {
		searcherList[s.GetType()] = s
	}
}

// OmniSearchClient 全能搜索客户端，支持多种搜索引擎和API key自动轮换
type OmniSearchClient struct {
	searcherList map[ostype.SearcherType]ostype.SearchClient // 存储各种类型的搜索引擎实例
	config       *OmniSearchConfig                           // 客户端配置
	mu           sync.RWMutex                                // 互斥锁，保护共享资源访问
}

// NewOmniSearchClient 创建一个新的全能搜索客户端
func NewOmniSearchClient(opts ...OmniSearchConfigOption) *OmniSearchClient {
	return NewOmniSearchClientWithConfig(NewOmniSearchConfig(opts...))
}

// NewOmniSearchClientWithConfig 使用已有配置创建全能搜索客户端
func NewOmniSearchClientWithConfig(config *OmniSearchConfig) *OmniSearchClient {
	osearch := &OmniSearchClient{
		searcherList: map[ostype.SearcherType]ostype.SearchClient{},
		config:       config,
		mu:           sync.RWMutex{},
	}
	osearch.InitDefaultSearchers()
	return osearch
}

// InitDefaultSearchers 初始化默认的搜索引擎
func (s *OmniSearchClient) InitDefaultSearchers() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.searcherList[ostype.SearcherTypeBrave] = searchers.NewOmniBraveSearchClient()
	s.searcherList[ostype.SearcherTypeTavily] = searchers.NewOmniTavilySearchClient()
	s.searcherList[ostype.SearcherTypeAiBalance] = searchers.NewOmniAiBalanceSearchClient()
	s.searcherList[ostype.SearcherTypeChatGLM] = searchers.NewOmniChatGLMSearchClient()
	s.searcherList[ostype.SearcherTypeBocha] = searchers.NewOmniBochaSearchClient()
	s.searcherList[ostype.SearcherTypeUnifuncs] = searchers.NewOmniUnifuncsSearchClient()
	for _, searcher := range searcherList {
		s.searcherList[searcher.GetType()] = searcher
	}
}

// Search 执行搜索操作，支持API key自动轮换与负载均衡
// 参数:
//   - query: 搜索查询字符串
//   - options: 搜索选项，如搜索引擎类型、API key等
//
// 返回:
//   - 搜索结果
//   - 错误信息
func (s *OmniSearchClient) Search(query string, options ...ostype.SearchOption) (*ostype.OmniSearchResultList, error) {
	newOptions := []ostype.SearchOption{}
	currentConfig := ostype.NewSearchConfig(options...)
	searcherType := currentConfig.SearcherType

	// 对共享资源的访问使用互斥锁保护
	s.mu.Lock()

	// 查找适合的API key
	keyList := []*SearchKeyInfo{}
	if s.config.searchKeys != nil {
		for _, key := range s.config.searchKeys {
			if key.Type == searcherType {
				keyList = append(keyList, key)
			}
		}
	}
	// 按照hit count排序，选择使用次数最少的key
	sort.Slice(keyList, func(i, j int) bool {
		return keyList[i].HitCount < keyList[j].HitCount
	})

	// 使用hit count最小的API key
	if len(keyList) > 0 {
		newOptions = append(newOptions, ostype.WithApiKey(keyList[0].ApiKey))
		keyList[0].HitCount++ // 增加该key的使用计数
	}

	// 获取对应的搜索引擎实例
	var searcher ostype.SearchClient
	var ok bool

	// 优先使用扩展搜索引擎
	searcher, ok = s.config.extSearcherMap[searcherType]
	if !ok {
		// 使用默认搜索引擎
		searcher, ok = s.searcherList[searcherType]
	}

	// 释放锁，因为后续的API调用不需要访问共享资源
	s.mu.Unlock()

	// 添加其他搜索选项
	newOptions = append(newOptions, options...)

	// 检查是否找到合适的搜索引擎
	if !ok {
		return nil, fmt.Errorf("未找到搜索引擎")
	}

	config := ostype.NewSearchConfig(newOptions...)
	// 执行搜索并返回结果
	res, err := searcher.Search(query, config)
	if err != nil {
		return nil, err
	}
	return &ostype.OmniSearchResultList{
		Results: res,
		Total:   len(res),
	}, nil
}
