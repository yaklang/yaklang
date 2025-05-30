package searchtools

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/template"

	"github.com/yaklang/yaklang/common/log"

	_ "embed"

	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/jsonextractor"
	"github.com/yaklang/yaklang/common/utils"
)

type ToolSearchRequest struct {
	Query string
	Tools []*aitool.Tool
}
type AiToolSearcher func(req *ToolSearchRequest) ([]*aitool.Tool, error)

//go:embed aitool-search.txt
var __prompt_SearchByAIPrompt string

//go:embed aitool-keyword-search.txt
var __prompt_KeywordSearch string

//go:embed aitool-keyword-summary.txt
var __prompt_KeywordSummary string

type AiToolsSearchResult struct {
	Tool   string `json:"tool"`
	Reason string `json:"reason"`
}

type KeywordSearchResult struct {
	Tool            string   `json:"tool"`
	MatchedKeywords []string `json:"matched_keywords"`
}

func NewKeyWordSearcher(chatToAiFunc func(string) (io.Reader, error)) AiToolSearcher {
	return func(req *ToolSearchRequest) ([]*aitool.Tool, error) {
		log.Infof("start to search with query: %v", req.Query)
		query := req.Query
		if chatToAiFunc == nil {
			return nil, utils.Errorf("ai callback is not set")
		}

		tools := req.Tools

		type ToolWithKeywords struct {
			Name     string `json:"Name"`
			Keywords string `json:"Keywords"`
		}

		toolsLists := []ToolWithKeywords{}
		toolMap := map[string]*aitool.Tool{}

		for _, tool := range tools {
			if len(tool.Keywords) == 0 {
				continue
			}
			toolsLists = append(toolsLists, ToolWithKeywords{
				Name:     tool.Name,
				Keywords: strings.Join(tool.Keywords, ", "),
			})
			toolMap[tool.Name] = tool
		}

		prompt, err := template.New("search_by_keyword").Parse(__prompt_KeywordSearch)
		if err != nil {
			return nil, utils.Errorf("parse prompt failed: %v", err)
		}

		var buf bytes.Buffer
		err = prompt.Execute(&buf, map[string]interface{}{
			"UserRequirement": query,
			"ToolsLists":      toolsLists,
		})
		if err != nil {
			return nil, utils.Errorf("execute prompt failed: %v", err)
		}

		stream, err := chatToAiFunc(buf.String())
		if err != nil {
			return nil, err
		}

		rspBytes, err := io.ReadAll(stream)
		if err != nil {
			return nil, err
		}

		rsp := string(rspBytes)

		var callResults []*KeywordSearchResult
		err = jsonextractor.ExtractStructuredJSON(rsp, jsonextractor.WithObjectCallback(func(data map[string]any) {
			toolname, ok := data["tool"]
			if !ok {
				return
			}
			reason, ok := data["reason"]
			if !ok {
				return
			}
			_ = reason
			callResults = append(callResults, &KeywordSearchResult{
				Tool: fmt.Sprint(toolname),
			})
		}))
		if err != nil {
			return nil, utils.Errorf("extract result failed: %v", err)
		}
		//for _, item := range  {
		//	start, end := item[0], item[1]
		//	resultJSON := rsp[start:end]
		//	res := KeywordSearchResult{}
		//	err = json.Unmarshal([]byte(resultJSON), &res)
		//	if err != nil {
		//		continue
		//	}
		//	callResults = append(callResults, &res)
		//}

		if len(callResults) == 0 {
			return nil, utils.Errorf("no tool found")
		}

		results := []*aitool.Tool{}
		for _, res := range callResults {
			tool, ok := toolMap[res.Tool]
			if !ok {
				continue
			}

			results = append(results, tool)
		}
		return results, nil
	}
}

func NewDescSearch(toolsGetter func() []*aitool.Tool, chatToAiFunc func(string) (io.Reader, error)) AiToolSearcher {
	return func(req *ToolSearchRequest) ([]*aitool.Tool, error) {
		query := req.Query
		if chatToAiFunc == nil {
			return nil, utils.Errorf("ai callback is not set")
		}

		tools := toolsGetter()
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

		stream, err := chatToAiFunc(buf.String())
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
		results := []*aitool.Tool{}
		for _, res := range callResults {
			tool, ok := toolMap[res.Tool]
			if !ok {
				continue
			}
			results = append(results, tool)
		}
		return results, nil
	}
}

func ToolKeywordSummary(query string, tools []*aitool.Tool, limit int, aiCallback func(string) (io.Reader, error)) ([]string, error) {
	if aiCallback == nil {
		return nil, utils.Errorf("ai callback is not set")
	}

	type ToolWithKeywords struct {
		Name     string `json:"Name"`
		Keywords string `json:"Keywords"`
	}

	toolsLists := []ToolWithKeywords{}

	for _, tool := range tools {
		if len(tool.Keywords) == 0 {
			continue
		}
		toolsLists = append(toolsLists, ToolWithKeywords{
			Name:     tool.Name,
			Keywords: strings.Join(tool.Keywords, ", "),
		})
	}

	if len(toolsLists) == 0 {
		return nil, utils.Errorf("no tools with keywords found")
	}

	prompt, err := template.New("keyword_summary").Parse(__prompt_KeywordSummary)
	if err != nil {
		return nil, utils.Errorf("parse prompt failed: %v", err)
	}

	var buf bytes.Buffer
	err = prompt.Execute(&buf, map[string]interface{}{
		"Query":      query,
		"Limit":      limit,
		"ToolsLists": toolsLists,
	})
	if err != nil {
		return nil, utils.Errorf("execute prompt failed: %v", err)
	}

	stream, err := aiCallback(buf.String())
	if err != nil {
		return nil, err
	}

	rspBytes, err := io.ReadAll(stream)
	if err != nil {
		return nil, err
	}

	rsp := string(rspBytes)

	var summary []string
	for _, item := range jsonextractor.ExtractObjectIndexes(rsp) {
		start, end := item[0], item[1]
		resultJSON := rsp[start:end]

		// Parse as an object with a "result" field containing the array
		var response struct {
			Result []string `json:"result"`
		}

		err = json.Unmarshal([]byte(resultJSON), &response)
		if err == nil && len(response.Result) > 0 {
			summary = response.Result
			break
		}
	}

	if len(summary) == 0 {
		return nil, utils.Errorf("failed to extract summary keywords")
	}

	// Ensure we don't exceed the limit
	if len(summary) > limit {
		summary = summary[:limit]
	}

	return summary, nil
}
