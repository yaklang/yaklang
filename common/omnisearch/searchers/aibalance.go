package searchers

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// AiBalanceSearchConfig contains the configuration for the AiBalance web search relay client
type AiBalanceSearchConfig struct {
	// BaseURL is the AiBalance server URL (default: http://127.0.0.1:80)
	BaseURL string
	// APIKey is the Bearer token for AiBalance authentication
	APIKey string
	// BackendSearcherType specifies which backend searcher to use via AiBalance ("brave", "tavily" or "chatglm")
	BackendSearcherType string
	// Proxy is an optional proxy for connecting to AiBalance
	Proxy string
	// Timeout is the request timeout in seconds
	Timeout float64
}

// NewDefaultAiBalanceConfig returns a default AiBalance configuration
func NewDefaultAiBalanceConfig() *AiBalanceSearchConfig {
	return &AiBalanceSearchConfig{
		BaseURL:             "https://aibalance.yaklang.com",
		BackendSearcherType: "", // empty means "auto" - let the server choose based on available keys
		Timeout:             30,
	}
}

// AiBalanceSearchClient implements web search via an AiBalance relay server
type AiBalanceSearchClient struct {
	Config *AiBalanceSearchConfig
}

// NewAiBalanceSearchClient creates a new AiBalance search client with the given config
func NewAiBalanceSearchClient(config *AiBalanceSearchConfig) *AiBalanceSearchClient {
	return &AiBalanceSearchClient{Config: config}
}

// NewDefaultAiBalanceSearchClient creates a new AiBalance search client with default configuration
func NewDefaultAiBalanceSearchClient() *AiBalanceSearchClient {
	return NewAiBalanceSearchClient(NewDefaultAiBalanceConfig())
}

// AiBalanceSearchRequest represents the JSON request body for /v1/web-search
type AiBalanceSearchRequest struct {
	Query        string `json:"query"`
	SearcherType string `json:"searcher_type"`
	MaxResults   int    `json:"max_results"`
	Page         int    `json:"page"`
	PageSize     int    `json:"page_size"`
}

// AiBalanceSearchResponse represents the JSON response body from /v1/web-search
type AiBalanceSearchResponse struct {
	Results      []*ostype.OmniSearchResult `json:"results"`
	Total        int                        `json:"total"`
	SearcherType string                     `json:"searcher_type"`
}

// AiBalanceErrorResponse represents an error response from the AiBalance server
type AiBalanceErrorResponse struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
	} `json:"error"`
}

// Search performs a search through the AiBalance web search relay
func (c *AiBalanceSearchClient) Search(query string, page, pageSize int) (*AiBalanceSearchResponse, error) {
	if c.Config.APIKey == "" {
		return nil, fmt.Errorf("aibalance api key (bearer token) is required")
	}

	if query == "" {
		return nil, fmt.Errorf("query is required")
	}

	// Build request body
	reqBody := &AiBalanceSearchRequest{
		Query:        query,
		SearcherType: c.Config.BackendSearcherType,
		MaxResults:   pageSize,
		Page:         page,
		PageSize:     pageSize,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	// Build the full URL for the web search endpoint
	// Strip trailing /v1/web-search from BaseURL to avoid double path
	// (user may pass "https://host/v1/web-search" as baseurl)
	baseURL := strings.TrimSuffix(c.Config.BaseURL, "/v1/web-search")
	baseURL = strings.TrimSuffix(baseURL, "/")
	searchURL := baseURL + "/v1/web-search"

	// Prepare HTTP request options
	opts := []lowhttp.LowhttpOpt{}
	if c.Config.Timeout > 0 {
		opts = append(opts, lowhttp.WithTimeoutFloat(c.Config.Timeout))
	}
	if c.Config.Proxy != "" {
		opts = append(opts, lowhttp.WithProxy(c.Config.Proxy))
	}

	// Send POST request
	raw, err := Request("POST", searchURL, map[string]string{
		"Content-Type":  "application/json",
		"Authorization": "Bearer " + c.Config.APIKey,
		"User-Agent":    "Yaklang-AiBalance-OmniSearch/1.0",
	}, nil, bodyBytes, opts...)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to aibalance: %v", err)
	}

	// Get response body and status code
	body := lowhttp.GetHTTPPacketBody(raw)
	statusCode := lowhttp.GetStatusCodeFromResponse(raw)

	if statusCode != 200 {
		// Try to parse error response
		var errResp AiBalanceErrorResponse
		if json.Unmarshal(body, &errResp) == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("aibalance web search failed (status %d): %s [%s]",
				statusCode, errResp.Error.Message, errResp.Error.Type)
		}
		return nil, fmt.Errorf("aibalance web search failed with status code %d: %s",
			statusCode, string(body))
	}

	// Parse the success response
	var resp AiBalanceSearchResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse aibalance search response: %v", err)
	}

	return &resp, nil
}
