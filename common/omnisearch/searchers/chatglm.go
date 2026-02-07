package searchers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// ChatGLM Web Search API: https://docs.bigmodel.cn/api-reference/%E5%B7%A5%E5%85%B7-api/%E7%BD%91%E7%BB%9C%E6%90%9C%E7%B4%A2
// POST /api/paas/v4/web_search
// Authorization: Bearer <token>

// ChatGLMSearchEngine enumerates the available search engines
type ChatGLMSearchEngine string

const (
	ChatGLMSearchStd      ChatGLMSearchEngine = "search_std"
	ChatGLMSearchPro      ChatGLMSearchEngine = "search_pro"
	ChatGLMSearchProSogou ChatGLMSearchEngine = "search_pro_sogou"
	ChatGLMSearchProQuark ChatGLMSearchEngine = "search_pro_quark"
)

// ChatGLMRecencyFilter enumerates the recency filter options
type ChatGLMRecencyFilter string

const (
	ChatGLMRecencyOneDay   ChatGLMRecencyFilter = "oneDay"
	ChatGLMRecencyOneWeek  ChatGLMRecencyFilter = "oneWeek"
	ChatGLMRecencyOneMonth ChatGLMRecencyFilter = "oneMonth"
	ChatGLMRecencyOneYear  ChatGLMRecencyFilter = "oneYear"
	ChatGLMRecencyNoLimit  ChatGLMRecencyFilter = "noLimit"
)

// ChatGLMContentSize enumerates the content size options
type ChatGLMContentSize string

const (
	ChatGLMContentMedium ChatGLMContentSize = "medium"
	ChatGLMContentHigh   ChatGLMContentSize = "high"
)

// ChatGLMSearchRequest represents the request body for ChatGLM Web Search API
type ChatGLMSearchRequest struct {
	SearchQuery        string `json:"search_query"`                   // Required: search content, max 70 chars
	SearchEngine       string `json:"search_engine"`                  // Required: search_std, search_pro, search_pro_sogou, search_pro_quark
	SearchIntent       bool   `json:"search_intent"`                  // Required: whether to perform intent recognition
	Count              int    `json:"count,omitempty"`                // Optional: 1-50, default 10
	SearchDomainFilter string `json:"search_domain_filter,omitempty"` // Optional: whitelist domain
	SearchRecencyFilter string `json:"search_recency_filter,omitempty"` // Optional: oneDay, oneWeek, oneMonth, oneYear, noLimit
	ContentSize        string `json:"content_size,omitempty"`         // Optional: medium, high
	RequestID          string `json:"request_id,omitempty"`           // Optional: unique request ID
	UserID             string `json:"user_id,omitempty"`              // Optional: end user ID (6-128 chars)
}

// ChatGLMSearchResponse represents the response from ChatGLM Web Search API
type ChatGLMSearchResponse struct {
	ID           string                   `json:"id"`
	Created      int64                    `json:"created"`
	RequestID    string                   `json:"request_id"`
	SearchIntent []ChatGLMSearchIntent    `json:"search_intent"`
	SearchResult []ChatGLMSearchResult    `json:"search_result"`
	Error        *ChatGLMSearchError      `json:"error,omitempty"`
}

// ChatGLMSearchIntent represents a search intent result
type ChatGLMSearchIntent struct {
	Query    string `json:"query"`
	Intent   string `json:"intent"`   // SEARCH_ALL, SEARCH_NONE, SEARCH_ALWAYS
	Keywords string `json:"keywords"`
}

// ChatGLMSearchResult represents a single search result
type ChatGLMSearchResult struct {
	Title       string `json:"title"`
	Content     string `json:"content"`
	Link        string `json:"link"`
	Media       string `json:"media"`       // website name
	Icon        string `json:"icon"`        // website icon URL
	Refer       string `json:"refer"`       // reference index
	PublishDate string `json:"publish_date"`
}

// ChatGLMSearchError represents an API error response
type ChatGLMSearchError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// ChatGLMSearchConfig holds configuration for the ChatGLM web search client
type ChatGLMSearchConfig struct {
	APIKey              string
	BaseURL             string
	Timeout             float64
	MaxResults          int
	Proxy               string
	SearchEngine        string // search_std, search_pro, search_pro_sogou, search_pro_quark
	SearchIntent        bool
	SearchRecencyFilter string // oneDay, oneWeek, oneMonth, oneYear, noLimit
	ContentSize         string // medium, high
	SearchDomainFilter  string // whitelist domain
}

// ChatGLMSearchClient is the client for ChatGLM Web Search API
type ChatGLMSearchClient struct {
	Config *ChatGLMSearchConfig
}

// NewChatGLMSearchClient creates a new ChatGLM search client with the given config
func NewChatGLMSearchClient(config *ChatGLMSearchConfig) *ChatGLMSearchClient {
	return &ChatGLMSearchClient{
		Config: config,
	}
}

// NewDefaultChatGLMSearchClient creates a client with default configuration
func NewDefaultChatGLMSearchClient() *ChatGLMSearchClient {
	return NewChatGLMSearchClient(NewDefaultChatGLMConfig())
}

// NewDefaultChatGLMConfig returns a default configuration for ChatGLM web search
func NewDefaultChatGLMConfig() *ChatGLMSearchConfig {
	return &ChatGLMSearchConfig{
		APIKey:              "",
		BaseURL:             "https://open.bigmodel.cn/api/paas/v4/web_search",
		Timeout:             15,
		MaxResults:          10,
		SearchEngine:        string(ChatGLMSearchStd),
		SearchIntent:        false,
		SearchRecencyFilter: string(ChatGLMRecencyNoLimit),
		ContentSize:         string(ChatGLMContentMedium),
	}
}

// Search performs a search query using ChatGLM Web Search API
func (c *ChatGLMSearchClient) Search(query string) (*ChatGLMSearchResponse, error) {
	return c.SearchWithCustomParams(query, nil)
}

// SearchWithCustomParams performs a search with custom request parameters
func (c *ChatGLMSearchClient) SearchWithCustomParams(query string, customParams *ChatGLMSearchRequest) (*ChatGLMSearchResponse, error) {
	if c.Config.APIKey == "" {
		return nil, errors.New("chatglm web search api key is required")
	}

	if query == "" {
		return nil, errors.New("search query is required")
	}

	// Build request body
	reqBody := &ChatGLMSearchRequest{
		SearchQuery:        query,
		SearchEngine:       c.Config.SearchEngine,
		SearchIntent:       c.Config.SearchIntent,
		Count:              c.Config.MaxResults,
		SearchRecencyFilter: c.Config.SearchRecencyFilter,
		ContentSize:        c.Config.ContentSize,
		SearchDomainFilter: c.Config.SearchDomainFilter,
	}

	// Apply custom params overrides
	if customParams != nil {
		if customParams.SearchEngine != "" {
			reqBody.SearchEngine = customParams.SearchEngine
		}
		if customParams.Count > 0 {
			reqBody.Count = customParams.Count
		}
		if customParams.SearchRecencyFilter != "" {
			reqBody.SearchRecencyFilter = customParams.SearchRecencyFilter
		}
		if customParams.ContentSize != "" {
			reqBody.ContentSize = customParams.ContentSize
		}
		if customParams.SearchDomainFilter != "" {
			reqBody.SearchDomainFilter = customParams.SearchDomainFilter
		}
		if customParams.RequestID != "" {
			reqBody.RequestID = customParams.RequestID
		}
		if customParams.UserID != "" {
			reqBody.UserID = customParams.UserID
		}
		reqBody.SearchIntent = customParams.SearchIntent
	}

	// Validate count range
	if reqBody.Count < 1 {
		reqBody.Count = 10
	}
	if reqBody.Count > 50 {
		reqBody.Count = 50
	}

	// Marshal request body
	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request body: %v", err)
	}

	// Determine HTTPS
	isHttps := strings.HasPrefix(c.Config.BaseURL, "https://")

	// Prepare HTTP request options
	opts := []lowhttp.LowhttpOpt{
		lowhttp.WithHttps(isHttps),
		lowhttp.WithTimeoutFloat(c.Config.Timeout),
	}

	// Add proxy if specified
	if c.Config.Proxy != "" {
		opts = append(opts, lowhttp.WithProxy(c.Config.Proxy))
	}

	// Send POST request with JSON body
	raw, err := Request("POST", c.Config.BaseURL, map[string]string{
		"Content-Type":  "application/json",
		"Accept":        "application/json",
		"Authorization": "Bearer " + c.Config.APIKey,
		"User-Agent":    "Yaklang-ChatGLMSearch/1.0",
	}, nil, bodyBytes, opts...)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}

	// Get response body
	body := lowhttp.GetHTTPPacketBody(raw)

	// Check response status code
	statusCode := lowhttp.GetStatusCodeFromResponse(raw)
	if statusCode != 200 {
		// Try to parse error response
		var errResp ChatGLMSearchResponse
		if jsonErr := json.Unmarshal(body, &errResp); jsonErr == nil && errResp.Error != nil {
			return nil, fmt.Errorf("chatglm api error (code: %s): %s", errResp.Error.Code, errResp.Error.Message)
		}
		return nil, fmt.Errorf("chatglm search request returned status code %d: %s", statusCode, string(body))
	}

	// Parse the response body
	var result ChatGLMSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse chatglm search results: %v", err)
	}

	// Check for error in response body (some APIs return 200 with error)
	if result.Error != nil {
		return nil, fmt.Errorf("chatglm api error (code: %s): %s", result.Error.Code, result.Error.Message)
	}

	return &result, nil
}

// FormatChatGLMResults formats ChatGLM search results into a human-readable string
func FormatChatGLMResults(response *ChatGLMSearchResponse) string {
	if response == nil {
		return "No search results available"
	}

	var sb strings.Builder

	// Show search intent if available
	if len(response.SearchIntent) > 0 {
		for _, intent := range response.SearchIntent {
			sb.WriteString(fmt.Sprintf("Query: %s (Intent: %s)\n", intent.Query, intent.Intent))
			if intent.Keywords != "" {
				sb.WriteString(fmt.Sprintf("Keywords: %s\n", intent.Keywords))
			}
		}
		sb.WriteString("\n")
	}

	// Show search results
	if len(response.SearchResult) > 0 {
		sb.WriteString(fmt.Sprintf("Found %d results:\n\n", len(response.SearchResult)))

		for i, result := range response.SearchResult {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, result.Title))
			sb.WriteString(fmt.Sprintf("   URL: %s\n", result.Link))
			if result.Media != "" {
				sb.WriteString(fmt.Sprintf("   Source: %s\n", result.Media))
			}
			sb.WriteString(fmt.Sprintf("   %s\n", result.Content))
			if result.PublishDate != "" {
				sb.WriteString(fmt.Sprintf("   Published: %s\n", result.PublishDate))
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("No search results found.\n")
	}

	return sb.String()
}

// SearchFormatted performs a search and returns formatted results
func (c *ChatGLMSearchClient) SearchFormatted(query string) (string, error) {
	response, err := c.Search(query)
	if err != nil {
		return "", err
	}
	return FormatChatGLMResults(response), nil
}
