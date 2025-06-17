package searchtools

import (
	"bytes"
	"encoding/json"
	"io"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/omnisearch"
	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/utils"
)

// CreateOmniSearchTools 创建全能搜索工具
func CreateOmniSearchTools() ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()

	err := factory.RegisterTool(
		"web_search",
		aitool.WithDescription("This is a web search tool."),
		aitool.WithStringParam("query",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("search query"),
		),
		aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			searchType := "brave"
			query := params.GetString("query")

			// 准备搜索选项
			searchOptions := []ostype.SearchOption{
				ostype.WithSearchType(ostype.SearcherType(searchType)),
				ostype.WithPageSize(100),
				ostype.WithPage(1),
			}

			cfg := &ostype.YakitOmniSearchKeyConfig{}
			err := consts.GetThirdPartyApplicationConfig("brave", cfg)
			if err != nil {
				log.Errorf("get brave api key config failed: %v", err)
			} else {
				searchOptions = append(searchOptions, ostype.WithApiKey(cfg.APIKey))
				searchOptions = append(searchOptions, ostype.WithProxy(cfg.Proxy))
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
	return factory.Tools(), nil
}
