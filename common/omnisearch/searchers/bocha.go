package searchers

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// Bocha AI Web Search API: https://api.bochaai.com/v1/web-search
// POST /v1/web-search
// Authorization: Bearer <token>

// BochaFreshness enumerates the time range filter options
type BochaFreshness string

const (
	BochaFreshnessOneDay   BochaFreshness = "OneDay"
	BochaFreshnessOneWeek  BochaFreshness = "OneWeek"
	BochaFreshnessOneMonth BochaFreshness = "OneMonth"
	BochaFreshnessOneYear  BochaFreshness = "OneYear"
	BochaFreshnessNoLimit  BochaFreshness = "noLimit"
)

// BochaSearchRequest represents the request body for Bocha Web Search API
type BochaSearchRequest struct {
	Query     string `json:"query"`               // Required: search keywords
	Freshness string `json:"freshness,omitempty"`  // Optional: OneDay, OneWeek, OneMonth, OneYear, noLimit (default: noLimit)
	Summary   bool   `json:"summary,omitempty"`    // Optional: whether to include summary (default: false)
	Count     int    `json:"count,omitempty"`      // Optional: number of results, 1-50 (default: 10)
}

// BochaSearchResponse represents the response from Bocha Web Search API
type BochaSearchResponse struct {
	Code  int         `json:"code"`
	LogID string      `json:"log_id"`
	Msg   *string     `json:"msg"`
	Data  *BochaData  `json:"data"`
}

// BochaData represents the data field in the response
type BochaData struct {
	Type         string            `json:"_type"`
	QueryContext *BochaQueryContext `json:"queryContext"`
	WebPages     *BochaWebPages    `json:"webPages"`
	Images       *BochaImages      `json:"images"`
}

// BochaQueryContext represents the query context
type BochaQueryContext struct {
	OriginalQuery string `json:"originalQuery"`
}

// BochaWebPages represents the web pages result
type BochaWebPages struct {
	TotalEstimatedMatches int              `json:"totalEstimatedMatches"`
	Value                 []BochaWebResult `json:"value"`
}

// BochaWebResult represents a single web search result
type BochaWebResult struct {
	Name             string `json:"name"`
	URL              string `json:"url"`
	DisplayURL       string `json:"displayUrl"`
	Snippet          string `json:"snippet"`
	Summary          string `json:"summary,omitempty"`
	SiteName         string `json:"siteName"`
	SiteIcon         string `json:"siteIcon,omitempty"`
	DateLastCrawled  string `json:"dateLastCrawled"`
	Language         string `json:"language"`
	IsFamilyFriendly bool   `json:"isFamilyFriendly"`
	IsNavigational   bool   `json:"isNavigational"`
}

// BochaImages represents the images result
type BochaImages struct {
	Value []BochaImageResult `json:"value"`
}

// BochaImageResult represents a single image result
type BochaImageResult struct {
	Name               string         `json:"name"`
	WebSearchURL       string         `json:"webSearchUrl"`
	ThumbnailURL       string         `json:"thumbnailUrl"`
	DatePublished      string         `json:"datePublished"`
	ContentURL         string         `json:"contentUrl"`
	HostPageURL        string         `json:"hostPageUrl"`
	ContentSize        string         `json:"contentSize"`
	EncodingFormat     string         `json:"encodingFormat"`
	HostPageDisplayURL string         `json:"hostPageDisplayUrl"`
	Width              int            `json:"width"`
	Height             int            `json:"height"`
	Thumbnail          *BochaThumbnail `json:"thumbnail,omitempty"`
}

// BochaThumbnail represents a thumbnail
type BochaThumbnail struct {
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	ContentURL string `json:"contentUrl"`
}

// BochaSearchConfig holds configuration for the Bocha web search client
type BochaSearchConfig struct {
	APIKey    string
	BaseURL   string
	Timeout   float64
	MaxResults int
	Proxy     string
	Freshness string // OneDay, OneWeek, OneMonth, OneYear, noLimit
	Summary   bool   // whether to include summary
}

// BochaSearchClient is the client for Bocha Web Search API
type BochaSearchClient struct {
	Config *BochaSearchConfig
}

// NewBochaSearchClient creates a new Bocha search client with the given config
func NewBochaSearchClient(config *BochaSearchConfig) *BochaSearchClient {
	return &BochaSearchClient{
		Config: config,
	}
}

// NewDefaultBochaSearchClient creates a client with default configuration
func NewDefaultBochaSearchClient() *BochaSearchClient {
	return NewBochaSearchClient(NewDefaultBochaConfig())
}

// NewDefaultBochaConfig returns a default configuration for Bocha web search
func NewDefaultBochaConfig() *BochaSearchConfig {
	return &BochaSearchConfig{
		APIKey:     "",
		BaseURL:    "https://api.bochaai.com/v1/web-search",
		Timeout:    15,
		MaxResults: 10,
		Freshness:  string(BochaFreshnessNoLimit),
		Summary:    true,
	}
}

// Search performs a search query using Bocha Web Search API
func (c *BochaSearchClient) Search(query string) (*BochaSearchResponse, error) {
	return c.SearchWithCustomParams(query, nil)
}

// SearchWithCustomParams performs a search with custom request parameters
func (c *BochaSearchClient) SearchWithCustomParams(query string, customParams *BochaSearchRequest) (*BochaSearchResponse, error) {
	if c.Config.APIKey == "" {
		return nil, errors.New("bocha web search api key is required")
	}

	if query == "" {
		return nil, errors.New("search query is required")
	}

	// Build request body
	reqBody := &BochaSearchRequest{
		Query:     query,
		Freshness: c.Config.Freshness,
		Summary:   c.Config.Summary,
		Count:     c.Config.MaxResults,
	}

	// Apply custom params overrides
	if customParams != nil {
		if customParams.Freshness != "" {
			reqBody.Freshness = customParams.Freshness
		}
		if customParams.Count > 0 {
			reqBody.Count = customParams.Count
		}
		reqBody.Summary = customParams.Summary
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
		"User-Agent":    "Yaklang-BochaSearch/1.0",
	}, nil, bodyBytes, opts...)
	if err != nil {
		return nil, fmt.Errorf("request failed: %v", err)
	}

	// Get response body
	body := lowhttp.GetHTTPPacketBody(raw)

	// Check response status code
	statusCode := lowhttp.GetStatusCodeFromResponse(raw)
	if statusCode != 200 {
		return nil, fmt.Errorf("bocha search request returned status code %d: %s", statusCode, string(body))
	}

	// Parse the response body
	var result BochaSearchResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("failed to parse bocha search results: %v", err)
	}

	// Check for error in response (code != 200 inside response body)
	if result.Code != 0 && result.Code != 200 {
		msg := "unknown error"
		if result.Msg != nil {
			msg = *result.Msg
		}
		return nil, fmt.Errorf("bocha api error (code: %d): %s", result.Code, msg)
	}

	return &result, nil
}

// FormatBochaResults formats Bocha search results into a human-readable string
func FormatBochaResults(response *BochaSearchResponse) string {
	if response == nil || response.Data == nil {
		return "No search results available"
	}

	var sb strings.Builder

	if response.Data.WebPages != nil && len(response.Data.WebPages.Value) > 0 {
		sb.WriteString(fmt.Sprintf("Found %d results (estimated %d total):\n\n",
			len(response.Data.WebPages.Value), response.Data.WebPages.TotalEstimatedMatches))

		for i, result := range response.Data.WebPages.Value {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, result.Name))
			sb.WriteString(fmt.Sprintf("   URL: %s\n", result.URL))
			if result.SiteName != "" {
				sb.WriteString(fmt.Sprintf("   Source: %s\n", result.SiteName))
			}
			sb.WriteString(fmt.Sprintf("   %s\n", result.Snippet))
			if result.Summary != "" {
				sb.WriteString(fmt.Sprintf("   Summary: %s\n", result.Summary))
			}
			if result.DateLastCrawled != "" {
				sb.WriteString(fmt.Sprintf("   Date: %s\n", result.DateLastCrawled))
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("No search results found.\n")
	}

	return sb.String()
}

// SearchFormatted performs a search and returns formatted results
func (c *BochaSearchClient) SearchFormatted(query string) (string, error) {
	response, err := c.Search(query)
	if err != nil {
		return "", err
	}
	return FormatBochaResults(response), nil
}
