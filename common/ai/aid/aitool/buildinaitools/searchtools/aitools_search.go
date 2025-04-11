package searchtools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"strings"

	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/omnisearch"
	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/utils"
)

func CreateAiToolsSearchTools(toolsGetter func() []*aitool.Tool) ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()
	err := factory.RegisterTool(
		"tools_search",
		aitool.WithDescription("Search tool that can search the names of all currently supported tools."),
		aitool.WithStringParam("query",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("The name of the tool to query, can describe tool requirements using natural language."),
		),
		aitool.WithCallback(func(params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			query := params.GetString("query")

			// 准备搜索选项
			searchOptions := []ostype.SearchOption{
				ostype.WithSearchType(ostype.SearcherType("aitools")),
				ostype.WithPageSize(100),
				ostype.WithPage(1),
				ostype.WithSearchType("aitools"),
			}

			// 创建搜索客户端并执行搜索
			client := omnisearch.NewOmniSearchClient()
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
	return factory.Tools(), nil
}

type CallAiFunc func(msg string) (string, error)
type AiToolsSearchClientConfig struct {
	SearchType string // "ai" or "keyword"
	Model      string
	CallAiFunc CallAiFunc
}

type AiToolsSearchClient struct {
	toolsGetter func() []*aitool.Tool
	cfg         *AiToolsSearchClientConfig
}

func NewAiToolsSearchClient(toolsGetter func() []*aitool.Tool, cfg *AiToolsSearchClientConfig) *AiToolsSearchClient {
	return &AiToolsSearchClient{
		toolsGetter: toolsGetter,
		cfg:         cfg,
	}
}

var _ ostype.SearchClient = &AiToolsSearchClient{}

func (c *AiToolsSearchClient) SearchByKeyword(query string, config *ostype.SearchConfig) ([]*ostype.OmniSearchResult, error) {
	results := []*ostype.OmniSearchResult{}
	for _, tool := range c.toolsGetter() {
		searchFields := []string{
			tool.Name,
			tool.Description,
		}
		searchOk := false
		for _, field := range searchFields {
			if strings.Contains(field, query) {
				results = append(results, &ostype.OmniSearchResult{
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

//go:embed aitool-search.txt
var __prompt_SearchByAIPrompt string

type AiToolsSearchResult struct {
	Tool   string `json:"tool"`
	Reason string `json:"reason"`
}

func (c *AiToolsSearchClient) SearchByAI(query string, config *ostype.SearchConfig) ([]*ostype.OmniSearchResult, error) {
	tools := c.toolsGetter()
	toolDescList := []string{}
	for _, tool := range tools {
		toolDescList = append(toolDescList, fmt.Sprintf("%s: %s", tool.Name, tool.Description))
	}
	prompt, err := template.New("search_by_ai").Parse(__prompt_SearchByAIPrompt)
	if err != nil {
		return nil, utils.Errorf("parse prompt failed: %v", err)
	}
	var buf bytes.Buffer
	err = prompt.Execute(&buf, map[string]interface{}{
		"Query":        query,
		"ToolDescList": strings.Join(toolDescList, "\n"),
	})
	if err != nil {
		return nil, utils.Errorf("execute prompt failed: %v", err)
	}

	rsp, err := c.cfg.CallAiFunc(buf.String())
	if err != nil {
		log.Errorf("chat error: %v", err)
	}
	var callResults *AiToolsSearchResult
	for _, item := range jsonextractor.ExtractObjectIndexes(rsp) {
		start, end := item[0], item[1]
		toolJSON := rsp[start:end]
		res := AiToolsSearchResult{}
		err = json.Unmarshal([]byte(toolJSON), &res)
		if err != nil {
			continue
		}
		callResults = &res
	}
	if callResults == nil {
		return nil, utils.Errorf("no tool found")
	}
	results := []*ostype.OmniSearchResult{}
	results = append(results, &ostype.OmniSearchResult{
		Title: callResults.Tool,
	})
	return results, nil
}

func (c *AiToolsSearchClient) Search(query string, config *ostype.SearchConfig) ([]*ostype.OmniSearchResult, error) {
	if c.cfg.SearchType == "keyword" {
		return c.SearchByKeyword(query, config)
	} else if c.cfg.SearchType == "ai" {
		return c.SearchByAI(query, config)
	}
	return nil, utils.Errorf("invalid search type: %s", c.cfg.SearchType)
}

func (c *AiToolsSearchClient) GetType() ostype.SearcherType {
	return ostype.SearcherType("aitools")
}
