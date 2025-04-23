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

const searchMethod = "aikeyword"

type AiToolInfo struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Reason      string `json:"reason"`
}

func CreateAiToolsSearchTools(toolsGetter func() []*aitool.Tool) ([]*aitool.Tool, error) {
	factory := aitool.NewFactory()
	err := factory.RegisterTool(
		"tools_search",
		aitool.WithDescription("Search tool that can search the names of all currently supported tools."),
		aitool.WithStringParam("query",
			aitool.WithParam_Required(true),
			aitool.WithParam_Description("The name of the tool to query, can describe tool requirements using natural language."),
		),
		aitool.WithCtxCallback(func(ctx *aitool.ToolInvokeCtx, params aitool.InvokeParams, stdout io.Writer, stderr io.Writer) (any, error) {
			query := params.GetString("query")
			clientName := "buildin-aitools"
			// 准备搜索选项
			searchOptions := []ostype.SearchOption{
				ostype.WithSearchType(ostype.SearcherType("aitools")),
				ostype.WithPageSize(100),
				ostype.WithPage(1),
				ostype.WithSearchType(ostype.SearcherType(clientName)),
			}

			// 创建搜索客户端并执行搜索
			client := omnisearch.NewOmniSearchClient(omnisearch.WithExtSearcher(NewAiToolsSearchClient(toolsGetter, &AiToolsSearchClientConfig{
				SearchType:   searchMethod,
				ClientName:   clientName,
				ChatToAiFunc: ctx.ChatToAiFunc,
			})))
			results, err := client.Search(query, searchOptions...)
			if err != nil {
				return nil, utils.Errorf("search failed: %v", err)
			}
			aitoolList := []AiToolInfo{}
			for _, result := range results.Results {
				toolInfo, ok := result.Data.(*AiToolInfo)
				if !ok {
					continue
				}
				aitoolList = append(aitoolList, *toolInfo)
			}
			data, err := json.MarshalIndent(aitoolList, "", "  ")
			if err != nil {
				return nil, utils.Errorf("serialize result failed: %v", err)
			}

			return string(data), nil
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
	ClientName string
	SearchType string // "ai" or "keyword"

	ChatToAiFunc aitool.ChatToAiFuncType
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

func (c *AiToolsSearchClient) SearchByAIKeyWords(query string, config *ostype.SearchConfig) ([]*ostype.OmniSearchResult, error) {
	return nil, nil
}
func (c *AiToolsSearchClient) SearchByAI(query string, config *ostype.SearchConfig) ([]*ostype.OmniSearchResult, error) {
	if c.cfg.ChatToAiFunc == nil {
		return nil, utils.Errorf("ai callback is not set")
	}
	tools := c.toolsGetter()
	toolDescList := []string{}
	toolMap := map[string]*aitool.Tool{}
	for _, tool := range tools {
		toolDescList = append(toolDescList, fmt.Sprintf("%s: %s", tool.Name, tool.Description))
		toolMap[tool.Name] = tool
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

	stream, err := c.cfg.ChatToAiFunc(buf.String())
	if err != nil {
		return nil, err
	}
	rspBytes, err := io.ReadAll(stream)
	if err != nil {
		return nil, err
	}
	rsp := string(rspBytes)
	var callResults []*AiToolsSearchResult
	for _, item := range jsonextractor.ExtractObjectIndexes(rsp) {
		start, end := item[0], item[1]
		toolJSON := rsp[start:end]
		res := []*AiToolsSearchResult{}
		err = json.Unmarshal([]byte(toolJSON), &res)
		if err != nil {
			continue
		}
		callResults = append(callResults, res...)
	}
	if len(callResults) == 0 {
		return nil, utils.Errorf("no tool found")
	}
	results := []*ostype.OmniSearchResult{}
	for _, res := range callResults {
		tool, ok := toolMap[res.Tool]
		if !ok {
			continue
		}
		results = append(results, &ostype.OmniSearchResult{
			Data: &AiToolInfo{
				Name:        tool.Name,
				Description: tool.Description,
				Reason:      res.Reason,
			},
		})
	}
	return results, nil
}

func (c *AiToolsSearchClient) Search(query string, config *ostype.SearchConfig) ([]*ostype.OmniSearchResult, error) {
	if c.cfg.SearchType == "keyword" {
		return c.SearchByKeyword(query, config)
	} else if c.cfg.SearchType == "ai" {
		return c.SearchByAI(query, config)
	} else if c.cfg.SearchType == "aikeyword" {
		return c.SearchByAIKeyWords(query, config)
	}
	return nil, utils.Errorf("invalid search type: %s", c.cfg.SearchType)
}

func (c *AiToolsSearchClient) GetType() ostype.SearcherType {
	return ostype.SearcherType(c.cfg.ClientName)
}
