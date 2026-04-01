package searchers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// Qwen Web Search via DashScope native generation API.
// Uses /api/v1/services/aigc/text-generation/generation and reads
// official search_info.search_results instead of parsing model text.

type QwenSearchConfig struct {
	APIKey                string
	BaseURL               string
	Model                 string
	Timeout               float64
	Proxy                 string
	SearchStrategy        string // turbo (default), max, agent, agent_max
	ForcedSearch          bool
	MaxTokens             int
	EnableCitation        bool
	CitationFormat        string
	EnableSearchExtension bool
	Freshness             int
	AssignedSiteList      []string
	PromptIntervene       string
	PrependSearchResult   bool
}

type QwenSearchClient struct {
	Config *QwenSearchConfig
}

func NewQwenSearchClient(config *QwenSearchConfig) *QwenSearchClient {
	return &QwenSearchClient{Config: config}
}

func NewDefaultQwenConfig() *QwenSearchConfig {
	return &QwenSearchConfig{
		BaseURL:               "https://dashscope.aliyuncs.com/api/v1/services/aigc/text-generation/generation",
		Model:                 "qwen-plus",
		Timeout:               60,
		SearchStrategy:        "max",
		ForcedSearch:          true,
		MaxTokens:             2048,
		EnableCitation:        true,
		CitationFormat:        "[ref_<number>]",
		EnableSearchExtension: true,
	}
}

type qwenMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type qwenIntentionOptions struct {
	PromptIntervene string `json:"prompt_intervene,omitempty"`
}

type qwenSearchOptions struct {
	EnableSource          bool                  `json:"enable_source,omitempty"`
	EnableCitation        bool                  `json:"enable_citation,omitempty"`
	CitationFormat        string                `json:"citation_format,omitempty"`
	ForcedSearch          bool                  `json:"forced_search,omitempty"`
	SearchStrategy        string                `json:"search_strategy,omitempty"`
	EnableSearchExtension bool                  `json:"enable_search_extension,omitempty"`
	Freshness             int                   `json:"freshness,omitempty"`
	AssignedSiteList      []string              `json:"assigned_site_list,omitempty"`
	IntentionOptions      *qwenIntentionOptions `json:"intention_options,omitempty"`
	PrependSearchResult   bool                  `json:"prepend_search_result,omitempty"`
}

type qwenInput struct {
	Messages []qwenMessage `json:"messages"`
}

type qwenParameters struct {
	EnableSearch   bool               `json:"enable_search"`
	SearchOptions  *qwenSearchOptions `json:"search_options,omitempty"`
	MaxTokens      int                `json:"max_tokens,omitempty"`
	EnableThinking *bool              `json:"enable_thinking,omitempty"`
	ResultFormat   string             `json:"result_format,omitempty"`
}

type qwenRequest struct {
	Model      string          `json:"model"`
	Input      *qwenInput      `json:"input"`
	Parameters *qwenParameters `json:"parameters,omitempty"`
}

type qwenResponseMessage struct {
	Content string `json:"content"`
	Role    string `json:"role"`
}

type qwenChoice struct {
	Message      qwenResponseMessage `json:"message"`
	FinishReason string              `json:"finish_reason"`
	Index        int                 `json:"index"`
}

type qwenSearchResult struct {
	Index    int    `json:"index"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	SiteName string `json:"site_name"`
	Icon     string `json:"icon"`
	Content  string `json:"content,omitempty"`
}

type qwenSearchInfo struct {
	ExtraToolInfo []map[string]any   `json:"extra_tool_info,omitempty"`
	SearchResults []qwenSearchResult `json:"search_results"`
}

type qwenOutput struct {
	Choices    []qwenChoice    `json:"choices"`
	SearchInfo *qwenSearchInfo `json:"search_info,omitempty"`
}

type qwenResponse struct {
	Output    *qwenOutput `json:"output,omitempty"`
	RequestID string      `json:"request_id,omitempty"`
	Code      string      `json:"code,omitempty"`
	Message   string      `json:"message,omitempty"`
}

type QwenSearchResponse struct {
	Summary       string
	ExtraToolInfo []map[string]any
	SearchResults []qwenSearchResult
}

func (c *QwenSearchClient) Search(query string) (*QwenSearchResponse, error) {
	if c.Config.APIKey == "" {
		return nil, errors.New("qwen/dashscope api key is required")
	}
	if query == "" {
		return nil, errors.New("search query is required")
	}

	disableThinking := false
	searchOptions := &qwenSearchOptions{
		EnableSource:          true,
		EnableCitation:        c.Config.EnableCitation,
		CitationFormat:        c.Config.CitationFormat,
		ForcedSearch:          c.Config.ForcedSearch,
		SearchStrategy:        c.Config.SearchStrategy,
		EnableSearchExtension: c.Config.EnableSearchExtension,
		Freshness:             c.Config.Freshness,
		AssignedSiteList:      c.Config.AssignedSiteList,
		PrependSearchResult:   c.Config.PrependSearchResult,
	}
	if c.Config.PromptIntervene != "" {
		searchOptions.IntentionOptions = &qwenIntentionOptions{PromptIntervene: c.Config.PromptIntervene}
	}

	reqBody := &qwenRequest{
		Model: c.Config.Model,
		Input: &qwenInput{
			Messages: []qwenMessage{{Role: "user", Content: query}},
		},
		Parameters: &qwenParameters{
			EnableSearch:   true,
			SearchOptions:  searchOptions,
			MaxTokens:      c.Config.MaxTokens,
			EnableThinking: &disableThinking,
			ResultFormat:   "message",
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

	var resp qwenResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse dashscope response: %v", err)
	}
	if resp.Code != "" {
		return nil, fmt.Errorf("dashscope api error %s: %s", resp.Code, resp.Message)
	}
	if resp.Output == nil {
		return nil, errors.New("dashscope response missing output")
	}

	result := &QwenSearchResponse{}

	if len(resp.Output.Choices) > 0 {
		result.Summary = strings.TrimSpace(resp.Output.Choices[0].Message.Content)
	}

	if resp.Output.SearchInfo != nil {
		result.ExtraToolInfo = resp.Output.SearchInfo.ExtraToolInfo
		result.SearchResults = resp.Output.SearchInfo.SearchResults
	}

	return result, nil
}
