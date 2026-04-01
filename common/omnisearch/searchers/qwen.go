package searchers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// Qwen Web Search via DashScope API
// https://help.aliyun.com/zh/model-studio/web-search
// Uses the DashScope protocol (not OpenAI compatible) to get both
// AI summary and search source results via search_info.

type QwenSearchConfig struct {
	APIKey         string
	BaseURL        string
	Model          string
	Timeout        float64
	Proxy          string
	SearchStrategy string // turbo (default), max, agent, agent_max
	ForcedSearch   bool
}

type QwenSearchClient struct {
	Config *QwenSearchConfig
}

func NewQwenSearchClient(config *QwenSearchConfig) *QwenSearchClient {
	return &QwenSearchClient{Config: config}
}

func NewDefaultQwenConfig() *QwenSearchConfig {
	return &QwenSearchConfig{
		BaseURL:        "https://dashscope.aliyuncs.com/api/v1/services/aigc/text-generation/generation",
		Model:          "qwen-plus",
		Timeout:        60,
		SearchStrategy: "turbo",
		ForcedSearch:   true,
	}
}

// DashScope request/response structs

type dashscopeMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type dashscopeInput struct {
	Messages []dashscopeMessage `json:"messages"`
}

type dashscopeSearchOptions struct {
	EnableSource   bool   `json:"enable_source"`
	ForcedSearch   bool   `json:"forced_search,omitempty"`
	SearchStrategy string `json:"search_strategy,omitempty"`
}

type dashscopeParameters struct {
	EnableSearch  bool                   `json:"enable_search"`
	SearchOptions dashscopeSearchOptions `json:"search_options"`
	ResultFormat  string                 `json:"result_format"`
}

type dashscopeRequest struct {
	Model      string              `json:"model"`
	Input      dashscopeInput      `json:"input"`
	Parameters dashscopeParameters `json:"parameters"`
}

type dashscopeResponseMessage struct {
	Content string `json:"content"`
	Role    string `json:"role"`
}

type dashscopeChoice struct {
	Message      dashscopeResponseMessage `json:"message"`
	FinishReason string                   `json:"finish_reason"`
}

type dashscopeSearchResult struct {
	Index    int    `json:"index"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	SiteName string `json:"site_name"`
	Icon     string `json:"icon"`
}

type dashscopeSearchInfo struct {
	SearchResults []dashscopeSearchResult `json:"search_results"`
}

type dashscopeOutput struct {
	Choices    []dashscopeChoice    `json:"choices"`
	SearchInfo *dashscopeSearchInfo `json:"search_info,omitempty"`
}

type dashscopeResponse struct {
	Output    dashscopeOutput `json:"output"`
	RequestID string          `json:"request_id"`
}

type QwenSearchResponse struct {
	Summary       string
	SearchResults []dashscopeSearchResult
}

func (c *QwenSearchClient) Search(query string) (*QwenSearchResponse, error) {
	if c.Config.APIKey == "" {
		return nil, errors.New("qwen/dashscope api key is required")
	}
	if query == "" {
		return nil, errors.New("search query is required")
	}

	reqBody := &dashscopeRequest{
		Model: c.Config.Model,
		Input: dashscopeInput{
			Messages: []dashscopeMessage{
				{Role: "user", Content: query},
			},
		},
		Parameters: dashscopeParameters{
			EnableSearch: true,
			SearchOptions: dashscopeSearchOptions{
				EnableSource:   true,
				ForcedSearch:   c.Config.ForcedSearch,
				SearchStrategy: c.Config.SearchStrategy,
			},
			ResultFormat: "message",
		},
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	isHttps := strings.HasPrefix(c.Config.BaseURL, "https://")

	opts := []lowhttp.LowhttpOpt{
		lowhttp.WithHttps(isHttps),
		lowhttp.WithTimeoutFloat(c.Config.Timeout),
	}
	if c.Config.Proxy != "" {
		opts = append(opts, lowhttp.WithProxy(c.Config.Proxy))
	}

	raw, err := Request("POST", c.Config.BaseURL, map[string]string{
		"Content-Type":  "application/json",
		"Accept":        "application/json",
		"Authorization": "Bearer " + c.Config.APIKey,
		"User-Agent":    "Yaklang-QwenSearch/1.0",
	}, nil, bodyBytes, opts...)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}

	body := lowhttp.GetHTTPPacketBody(raw)
	statusCode := lowhttp.GetStatusCodeFromResponse(raw)
	if statusCode != 200 {
		return nil, fmt.Errorf("dashscope api returned status %d: %s", statusCode, string(body))
	}

	var resp dashscopeResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse dashscope response: %v", err)
	}

	result := &QwenSearchResponse{}

	if len(resp.Output.Choices) > 0 {
		result.Summary = resp.Output.Choices[0].Message.Content
	}

	if resp.Output.SearchInfo != nil {
		result.SearchResults = resp.Output.SearchInfo.SearchResults
	}

	return result, nil
}
