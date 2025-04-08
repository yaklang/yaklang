package fstools

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/omnisearch"
	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/utils"
)

// CreateOmniSearchTools 创建全能搜索工具
func CreateOmniSearchTools() ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()

	err := factory.RegisterTool(
		"omni_search",
		aitool.WithDescription("This is an aggregated search tool that can search for various types of information."),
		aitool.WithStringParam("search_engine",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("Search engine name, options include: 'brave', 'tavily'."),
		),
		aitool.WithStringParam("query",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("search query"),
		),
		aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			searchType := params.GetString("search_engine")
			query := params.GetString("query")

			// 准备搜索选项
			searchOptions := []ostype.SearchOption{
				ostype.WithSearchType(ostype.SearcherType(searchType)),
				ostype.WithPageSize(100),
				ostype.WithPage(1),
			}

			// 创建搜索客户端并执行搜索
			client := omnisearch.NewOmniSearchClient()
			results, err := client.Search(query, searchOptions...)
			if err != nil {
				return nil, utils.Errorf("search failed: %v", err)
			}

			// 格式化结果为JSON
			var buf bytes.Buffer
			resultMap := map[string]interface{}{
				"total":   len(results.Results),
				"results": results.Results,
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
