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

var Exports = map[string]interface{}{
	"Search": Search,
	"type": func(name string) ostype.SearchOption {
		return ostype.WithSearchType(ostype.SearcherType(name))
	},
	"proxy":    ostype.WithProxy,
	"baseurl":  ostype.WithBaseURL,
	"timeout":  ostype.WithTimeout,
	"pagesize": ostype.WithPageSize,
	"page":     ostype.WithPage,
	"customSearcher": func(name string, handle CustomSearcherHandle) ostype.SearchOption {
		return ostype.WithExtra("customSearcher", NewCustomSearcher(name, handle))
	},
	"apikey": func(keys ...string) ostype.SearchOption {
		return ostype.WithExtra("apikeys", keys)
	},
	"backendType": func(backendType string) ostype.SearchOption {
		return ostype.WithExtra("backend_searcher_type", backendType)
	},
}
