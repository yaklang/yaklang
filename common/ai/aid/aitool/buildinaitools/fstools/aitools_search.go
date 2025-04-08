package fstools

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/omnisearch"
	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/utils"
)

func CreateAiToolsSearchTools(tools []*aitool.Tool, query string) ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()
	err := factory.RegisterTool(
		"tools_search",
		aitool.WithDescription("Search for tools."),
		aitool.WithStringParam("query",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("search query"),
		),
		aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			query := params.GetString("query")

			searchClient := NewAiToolsSearchClient(tools)
			// 准备搜索选项
			searchOptions := []ostype.SearchOption{
				ostype.WithSearchType(ostype.SearcherType("aitools")),
				ostype.WithPageSize(100),
				ostype.WithPage(1),
			}

			// 创建搜索客户端并执行搜索
			client := omnisearch.NewOmniSearchClient(omnisearch.WithExtSearcher(searchClient))
			results, err := client.Search(query, searchOptions...)
			if err != nil {
				return nil, utils.Errorf("search failed: %v", err)
			}
			toolNames := []string{}
			for _, result := range results.Results {
				toolNames = append(toolNames, result.Title)
			}
			// 格式化结果为JSON
			var buf bytes.Buffer
			resultMap := map[string]interface{}{
				"total":   len(toolNames),
				"results": toolNames,
			}

			data, err := json.MarshalIndent(resultMap, "", "  ")
			if err != nil {
				return nil, utils.Errorf("serialize result failed: %v", err)
			}
			buf.Write(data)

			return buf.String(), nil
		}),
	)

	if err != nil {
		log.Errorf("register omni_search tool failed: %v", err)
		return nil, err
	}

	// 添加一个辅助工具，列出支持的搜索引擎类型
	err = factory.RegisterTool(
		"list_search_engines",
		aitool.WithDescription("List supported search engines"),
		aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			engines := map[string]string{
				string(ostype.SearcherTypeBrave):  "Brave search engine",
				string(ostype.SearcherTypeTavily): "Tavily search engine",
			}

			data, err := json.MarshalIndent(engines, "", "  ")
			if err != nil {
				return nil, utils.Errorf("serialize engine list failed: %v", err)
			}

			return string(data), nil
		}),
	)

	if err != nil {
		log.Errorf("register list_search_engines tool failed: %v", err)
		return nil, err
	}

	return factory.Tools(), nil
}

type AiToolsSearchClient struct {
	tools []*aitool.Tool
}

func NewAiToolsSearchClient(tools []*aitool.Tool) *AiToolsSearchClient {
	return &AiToolsSearchClient{
		tools: tools,
	}
}

var _ ostype.SearchClient = &AiToolsSearchClient{}

func (c *AiToolsSearchClient) Search(query string, config *ostype.SearchConfig) (*ostype.OmniSearchResultList, error) {
	results := &ostype.OmniSearchResultList{}
	for _, tool := range c.tools {
		searchFields := []string{
			tool.Name,
			tool.Description,
		}
		searchOk := false
		for _, field := range searchFields {
			if strings.Contains(field, query) {
				results.Results = append(results.Results, &ostype.OmniSearchResult{
					Title: tool.Name,
				})
				searchOk = true
				break
			}
		}
		if searchOk {
			continue
		}
	}
	return results, nil
}

func (c *AiToolsSearchClient) GetType() ostype.SearcherType {
	return ostype.SearcherType("aitools")
}
