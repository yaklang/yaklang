package omnisearch

import (
	"fmt"

	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/utils"
)

type CustomSearcherHandle func(query string, req *ostype.SearchConfig) (any, any)

type CustomSearcher struct {
	Name   string
	Handle CustomSearcherHandle
}

func NewCustomSearcher(name string, handle CustomSearcherHandle) *CustomSearcher {
	return &CustomSearcher{
		Name:   name,
		Handle: handle,
	}
}

func (c *CustomSearcher) GetType() ostype.SearcherType {
	return ostype.SearcherType(c.Name)
}

func (c *CustomSearcher) Search(query string, config *ostype.SearchConfig) ([]*ostype.OmniSearchResult, error) {
	iresults, err := c.Handle(query, config)
	if err != nil {
		return nil, fmt.Errorf("call custom searcher failed: %v", err)
	}
	results := utils.InterfaceToStringSlice(iresults)

	resultList := []*ostype.OmniSearchResult{}
	for _, result := range results {
		resultList = append(resultList, &ostype.OmniSearchResult{
			Content: utils.InterfaceToString(result),
			Source:  c.Name,
		})
	}
	return resultList, nil
}

var _ ostype.SearchClient = &CustomSearcher{}

// Search 使用聚合搜索引擎执行一次综合搜索（默认走 aibalance 聚合层）
// 依赖外部搜索服务与 API Key，需要网络环境
// 参数:
//   - query: 搜索关键词
//   - options: 搜索可选项，如 omnisearch.apikey / omnisearch.type / omnisearch.page 等
//
// 返回值:
//   - 搜索结果列表，每个结果包含内容与来源
//   - 错误信息
//
// Example:
// ```
// // 示意性示例，需要有效的 apikey 与网络
// results = omnisearch.Search("yaklang", omnisearch.apikey("your-api-key"))~
//
//	for r in results {
//	    println(r.Content)
//	}
//
// ```
func Search(query string, options ...ostype.SearchOption) ([]*ostype.OmniSearchResult, error) {
	config := ostype.NewSearchConfig(options...)
	extra := config.Extra
	iapikeys := extra["apikeys"]
	var apikeys []string
	if iapikeys == nil {
		apikeys = []string{}
	} else {
		apikeys = utils.InterfaceToStringSlice(iapikeys)
	}

	// Auto-detect searcher type if not explicitly set:
	// - If user provides apikeys → default to aibalance (the aggregation layer)
	// - If no apikeys and no type → default to aibalance as well
	searcherType := config.SearcherType
	if searcherType == "" {
		if len(apikeys) > 0 {
			searcherType = ostype.SearcherTypeAiBalance
		} else {
			searcherType = ostype.SearcherTypeAiBalance
		}
	}

	searchKeys := []*SearchKeyInfo{}
	for _, key := range apikeys {
		searchKeys = append(searchKeys, &SearchKeyInfo{ApiKey: key, Type: searcherType})
	}

	// omnisearch 配置
	opts := []OmniSearchConfigOption{}
	opts = append(opts, WithSearchKeys(searchKeys...))

	// 自定义搜索
	customSearcher := extra["customSearcher"]
	if customSearcher != nil {
		customSearcher, _ := customSearcher.(*CustomSearcher)
		if customSearcher != nil {
			opts = append(opts, WithExtSearcher(customSearcher))
		}
	}

	// Override searcher type in options to use the resolved type
	finalOptions := append([]ostype.SearchOption{ostype.WithSearchType(searcherType)}, options...)

	res, err := NewOmniSearchClient(opts...).Search(query, finalOptions...)
	if err != nil {
		return nil, err
	}
	return res.Results, nil
}

// omnisearchType 指定本次搜索使用的搜索器类型（导出名为 omnisearch.type）
// 作为 omnisearch.Search 的可选项使用；配合 omnisearch.customSearcher 可将搜索路由到自定义搜索器
//
// 参数:
//   - name: 搜索器类型名称
//
// 返回值:
//   - 可传入 omnisearch.Search 的搜索选项
//
// Example:
// ```
// results = omnisearch.Search(
//
//	"yaklang",
//	omnisearch.type("mylocal"),
//	omnisearch.customSearcher("mylocal", (query, cfg) => { return [f"hit-for-${query}"], nil }),
//
// )~
// println(len(results))   // OUT: 1
// assert results[0].Content == "hit-for-yaklang", "type option should route to the local searcher"
// ```
func omnisearchType(name string) ostype.SearchOption {
	return ostype.WithSearchType(ostype.SearcherType(name))
}

// omnisearchCustomSearcher 注册一个自定义搜索器，使 omnisearch.Search 调用本地处理函数（导出名为 omnisearch.customSearcher）
// 配合 omnisearch.type(同名) 使用，可在不依赖外部搜索服务的情况下完成搜索
//
// 参数:
//   - name: 自定义搜索器名称，需与 omnisearch.type 指定的类型名一致
//   - handle: 处理函数，接收查询串与搜索配置，返回结果与错误
//
// 返回值:
//   - 可传入 omnisearch.Search 的搜索选项
//
// Example:
// ```
// results = omnisearch.Search(
//
//	"yaklang",
//	omnisearch.type("mylocal"),
//	omnisearch.customSearcher("mylocal", (query, cfg) => { return [f"hit-for-${query}"], nil }),
//
// )~
// println(results[0].Source)   // OUT: mylocal
// assert results[0].Source == "mylocal", "customSearcher should produce results tagged with its name"
// ```
func omnisearchCustomSearcher(name string, handle CustomSearcherHandle) ostype.SearchOption {
	return ostype.WithExtra("customSearcher", NewCustomSearcher(name, handle))
}

// omnisearchAPIKey 为本次搜索设置一个或多个 API Key（导出名为 omnisearch.apikey）
// 作为 omnisearch.Search 的可选项；真实联网搜索时用于鉴权，可一次传入多个 key 做轮询
//
// 参数:
//   - keys: 一个或多个 API Key
//
// 返回值:
//   - 可传入 omnisearch.Search 的搜索选项
//
// Example:
// ```
// // 此处用自定义搜索器离线演示 apikey 选项被正确接收并传入 Search
// results = omnisearch.Search(
//
//	"yaklang",
//	omnisearch.type("mylocal"),
//	omnisearch.apikey("demo-key-1", "demo-key-2"),
//	omnisearch.customSearcher("mylocal", (query, cfg) => { return ["ok"], nil }),
//
// )~
// println(len(results))   // OUT: 1
// assert len(results) == 1, "apikey option should be accepted by Search"
// ```
func omnisearchAPIKey(keys ...string) ostype.SearchOption {
	return ostype.WithExtra("apikeys", keys)
}

// omnisearchBackendType 指定聚合层后端使用的实际搜索器类型（导出名为 omnisearch.backendType）
// 作为 omnisearch.Search 的可选项，用于让 aibalance 等聚合后端选择具体的下游搜索引擎
//
// 参数:
//   - backendType: 后端搜索器类型名称
//
// 返回值:
//   - 可传入 omnisearch.Search 的搜索选项
//
// Example:
// ```
// // 此处用自定义搜索器离线演示 backendType 选项被正确接收并传入 Search
// results = omnisearch.Search(
//
//	"yaklang",
//	omnisearch.type("mylocal"),
//	omnisearch.backendType("custom"),
//	omnisearch.customSearcher("mylocal", (query, cfg) => { return ["ok"], nil }),
//
// )~
// println(len(results))   // OUT: 1
// assert len(results) == 1, "backendType option should be accepted by Search"
// ```
func omnisearchBackendType(backendType string) ostype.SearchOption {
	return ostype.WithExtra("backend_searcher_type", backendType)
}

var Exports = map[string]interface{}{
	"Search":         Search,
	"type":           omnisearchType,
	"proxy":          ostype.WithProxy,
	"baseurl":        ostype.WithBaseURL,
	"timeout":        ostype.WithTimeout,
	"pagesize":       ostype.WithPageSize,
	"page":           ostype.WithPage,
	"customSearcher": omnisearchCustomSearcher,
	"apikey":         omnisearchAPIKey,
	"backendType":    omnisearchBackendType,
}
