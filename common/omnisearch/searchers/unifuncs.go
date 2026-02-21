package searchers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// UniFuncs Web Search API: https://api.unifuncs.com/api/web-search/search
// POST /api/web-search/search
// Authorization: Bearer <token>

// UnifuncsFreshness enumerates the time range filter options
type UnifuncsFreshness string

const (
	UnifuncsFreshnessDay   UnifuncsFreshness = "Day"
	UnifuncsFreshnessWeek  UnifuncsFreshness = "Week"
	UnifuncsFreshnessMonth UnifuncsFreshness = "Month"
	UnifuncsFreshnessYear  UnifuncsFreshness = "Year"
)

// UnifuncsSearchRequest represents the request body for UniFuncs Web Search API
type UnifuncsSearchRequest struct {
	Query     string `json:"query"`               // Required: search keywords
	Freshness string `json:"freshness,omitempty"`  // Optional: Day, Week, Month, Year
	Count     int    `json:"count,omitempty"`      // Optional: number of results per page (1-50), default 10
	Page      int    `json:"page,omitempty"`       // Optional: page number, default 1
	Format    string `json:"format,omitempty"`     // Optional: json, markdown, md, text, txt (default: json)
}

// UnifuncsSearchResponse represents the response from UniFuncs Web Search API
type UnifuncsSearchResponse struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Data      *UnifuncsData   `json:"data"`
	RequestID string          `json:"requestId"`
}

// UnifuncsData represents the data field in the response
type UnifuncsData struct {
	WebPages []UnifuncsWebResult   `json:"webPages"`
	Images   []UnifuncsImageResult `json:"images"`
}

// UnifuncsWebResult represents a single web search result
type UnifuncsWebResult struct {
	Name          string  `json:"name"`
	URL           string  `json:"url"`
	DisplayURL    string  `json:"displayUrl"`
	Snippet       string  `json:"snippet"`
	Summary       string  `json:"summary,omitempty"`
	SiteName      string  `json:"siteName"`
	SiteIcon      string  `json:"siteIcon,omitempty"`
	DatePublished *string `json:"datePublished"`
}

// UnifuncsImageResult represents a single image result
type UnifuncsImageResult struct {
	ThumbnailURL       string `json:"thumbnailUrl"`
	ContentURL         string `json:"contentUrl"`
	Width              int    `json:"width"`
	Height             int    `json:"height"`
	HostPageURL        string `json:"hostPageUrl"`
	HostPageDisplayURL string `json:"hostPageDisplayUrl"`
}

// UnifuncsSearchConfig holds configuration for the UniFuncs web search client
type UnifuncsSearchConfig struct {
	APIKey     string
	BaseURL    string
	Timeout    float64
	MaxResults int
	Proxy      string
	Freshness  string // Day, Week, Month, Year
}

// UnifuncsSearchClient is the client for UniFuncs Web Search API
type UnifuncsSearchClient struct {
	Config *UnifuncsSearchConfig
}

// NewUnifuncsSearchClient creates a new UniFuncs search client with the given config
func NewUnifuncsSearchClient(config *UnifuncsSearchConfig) *UnifuncsSearchClient {
	return &UnifuncsSearchClient{
		Config: config,
	}
}

// NewDefaultUnifuncsSearchClient creates a client with default configuration
func NewDefaultUnifuncsSearchClient() *UnifuncsSearchClient {
	return NewUnifuncsSearchClient(NewDefaultUnifuncsConfig())
}

// NewDefaultUnifuncsConfig returns a default configuration for UniFuncs web search
func NewDefaultUnifuncsConfig() *UnifuncsSearchConfig {
	return &UnifuncsSearchConfig{
		APIKey:     "",
		BaseURL:    "https://api.unifuncs.com/api/web-search/search",
		Timeout:    15,
		MaxResults: 10,
	}
}

// Search performs a search query using UniFuncs Web Search API
func (c *UnifuncsSearchClient) Search(query string) (*UnifuncsSearchResponse, error) {
	return c.SearchWithCustomParams(query, nil)
}

// SearchWithCustomParams performs a search with custom request parameters
func (c *UnifuncsSearchClient) SearchWithCustomParams(query string, customParams *UnifuncsSearchRequest) (*UnifuncsSearchResponse, error) {
	if c.Config.APIKey == "" {
		return nil, errors.New("unifuncs web search api key is required")
	}

	if query == "" {
		return nil, errors.New("search query is required")
	}

	reqBody := &UnifuncsSearchRequest{
		Query:     query,
		Freshness: c.Config.Freshness,
		Count:     c.Config.MaxResults,
		Format:    "json",
	}

	if customParams != nil {
		if customParams.Freshness != "" {
			reqBody.Freshness = customParams.Freshness
		}
		if customParams.Count > 0 {
			reqBody.Count = customParams.Count
		}
		if customParams.Page > 0 {
			reqBody.Page = customParams.Page
		}
	}

	if reqBody.Count < 1 {
		reqBody.Count = 10
	}
	if reqBody.Count > 50 {
		reqBody.Count = 50
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
		"User-Agent":    "Yaklang-UnifuncsSearch/1.0",
	}, nil, bodyBytes, opts...)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}

	body := lowhttp.GetHTTPPacketBody(raw)

	statusCode := lowhttp.GetStatusCodeFromResponse(raw)
	if statusCode != 200 {
		return nil, fmt.Errorf("unifuncs search request returned status code %d: %s", statusCode, string(body))
	}

	var result UnifuncsSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse unifuncs search results: %v", err)
	}

	if result.Code != 0 {
		return nil, fmt.Errorf("unifuncs api error (code: %d): %s", result.Code, result.Message)
	}

	return &result, nil
}

// FormatUnifuncsResults formats UniFuncs search results into a human-readable string
func FormatUnifuncsResults(response *UnifuncsSearchResponse) string {
	if response == nil || response.Data == nil {
		return "No search results available"
	}

	var sb strings.Builder

	if len(response.Data.WebPages) > 0 {
		sb.WriteString(fmt.Sprintf("Found %d results:\n\n", len(response.Data.WebPages)))

		for i, result := range response.Data.WebPages {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, result.Name))
			sb.WriteString(fmt.Sprintf("   URL: %s\n", result.URL))
			if result.SiteName != "" {
				sb.WriteString(fmt.Sprintf("   Source: %s\n", result.SiteName))
			}
			sb.WriteString(fmt.Sprintf("   %s\n", result.Snippet))
			if result.Summary != "" && result.Summary != result.Snippet {
				sb.WriteString(fmt.Sprintf("   Summary: %s\n", result.Summary))
			}
			if result.DatePublished != nil && *result.DatePublished != "" {
				sb.WriteString(fmt.Sprintf("   Date: %s\n", *result.DatePublished))
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("No search results found.\n")
	}

	return sb.String()
}

// SearchFormatted performs a search and returns formatted results
func (c *UnifuncsSearchClient) SearchFormatted(query string) (string, error) {
	response, err := c.Search(query)
	if err != nil {
		return "", err
	}
	return FormatUnifuncsResults(response), nil
}
