package searchers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

const tavilyRequestTemplate = `POST /%s HTTP/1.1
Host: api.tavily.com
Content-Type: application/json
Authorization: Bearer %s
Accept: application/json
Accept-Encoding: gzip
User-Agent: Yaklang-TavilySearch/1.0
Content-Length: %d

%s`

// SearchDepth defines the depth of search
type SearchDepth string

const (
	// Basic search depth
	BasicSearch SearchDepth = "basic"
	// Advanced search depth
	AdvancedSearch SearchDepth = "advanced"
)

// Topic defines the topic to search
type Topic string

const (
	// General topic
	GeneralTopic Topic = "general"
	// News topic
	NewsTopic Topic = "news"
	// Finance topic
	FinanceTopic Topic = "finance"
)

// TimeRange defines the time range for search
type TimeRange string

const (
	// Day time range
	DayRange TimeRange = "day"
	// Week time range
	WeekRange TimeRange = "week"
	// Month time range
	MonthRange TimeRange = "month"
	// Year time range
	YearRange TimeRange = "year"
)

// TavilySearchParams represents the query parameters for Tavily Search API
type TavilySearchParams struct {
	Query             string      `json:"query"`
	SearchDepth       SearchDepth `json:"search_depth"`
	Topic             Topic       `json:"topic"`
	TimeRange         TimeRange   `json:"time_range,omitempty"`
	Days              int         `json:"days"`
	MaxResults        int         `json:"max_results"`
	IncludeDomains    []string    `json:"include_domains,omitempty"`
	ExcludeDomains    []string    `json:"exclude_domains,omitempty"`
	IncludeAnswer     interface{} `json:"include_answer"` // can be bool or string
	IncludeRawContent bool        `json:"include_raw_content"`
	IncludeImages     bool        `json:"include_images"`
}

// TavilySearchResult represents a single search result
type TavilySearchResult struct {
	URL           string  `json:"url"`
	Content       string  `json:"content"`
	Title         string  `json:"title,omitempty"`
	Score         float64 `json:"score"`
	PublishedDate string  `json:"published_date,omitempty"`
	Author        string  `json:"author,omitempty"`
	ImageURL      string  `json:"image_url,omitempty"`
	ImageAlt      string  `json:"image_alt,omitempty"`
	RawContent    string  `json:"raw_content,omitempty"`
}

// TavilySearchResponse represents the response from the Tavily Search API
type TavilySearchResponse struct {
	Results       []TavilySearchResult `json:"results"`
	Answer        string               `json:"answer,omitempty"`
	Query         string               `json:"query"`
	SearchID      string               `json:"search_id"`
	SearchDepth   string               `json:"search_depth"`
	MaxResults    int                  `json:"max_results"`
	Topic         string               `json:"topic"`
	CreatedAt     string               `json:"created_at"`
	FailedResults []string             `json:"failed_results,omitempty"`
}

// TavilyExtractParams represents the parameters for the extract API
type TavilyExtractParams struct {
	URLs          []string    `json:"urls"`
	IncludeImages bool        `json:"include_images"`
	ExtractDepth  SearchDepth `json:"extract_depth"`
}

// TavilyExtractResponse represents the response from the extract API
type TavilyExtractResponse struct {
	Results       []TavilySearchResult `json:"results"`
	FailedResults []string             `json:"failed_results,omitempty"`
}

// TavilySearchConfig holds configuration for the Tavily search client
type TavilySearchConfig struct {
	APIKey          string
	BaseURL         string
	Timeout         float64
	MaxResults      int
	Proxy           string
	DefaultTopic    Topic
	DefaultDays     int
	DefaultDepth    SearchDepth
	CompanyInfoTags []Topic
}

// TavilySearchClient is the client for Tavily Search API
type TavilySearchClient struct {
	Config *TavilySearchConfig
}

// NewTavilySearchClient creates a new Tavily search client with the specified configuration
func NewTavilySearchClient(config *TavilySearchConfig) *TavilySearchClient {
	return &TavilySearchClient{
		Config: config,
	}
}

// NewDefaultTavilySearchClient creates a new Tavily search client with default configuration
func NewDefaultTavilySearchClient() *TavilySearchClient {
	return NewTavilySearchClient(NewDefaultTavilyConfig())
}

// DefaultTavilyConfig returns a default configuration for Tavily search
func NewDefaultTavilyConfig() *TavilySearchConfig {
	apiKey := os.Getenv("TAVILY_API_KEY")
	return &TavilySearchConfig{
		APIKey:          apiKey, // Get from environment variable or set explicitly
		BaseURL:         "https://api.tavily.com",
		Timeout:         60,
		MaxResults:      5,
		DefaultTopic:    GeneralTopic,
		DefaultDays:     7,
		DefaultDepth:    BasicSearch,
		CompanyInfoTags: []Topic{NewsTopic, GeneralTopic, FinanceTopic},
	}
}

// TavilySearch performs a search query using Tavily Search API with default client
func TavilySearch(query string) ([]TavilySearchResult, error) {
	client := NewDefaultTavilySearchClient()
	response, err := client.Search(query)
	if err != nil {
		return nil, err
	}
	return response.Results, nil
}

// Search performs a search with the client's configuration and optional custom params
func (t *TavilySearchClient) Search(query string) (*TavilySearchResponse, error) {
	return t.SearchWithCustomParams(query, nil)
}

// Search performs a search with the client's configuration and optional custom params
func (t *TavilySearchClient) SearchWithCustomParams(query string, customParams *TavilySearchParams) (*TavilySearchResponse, error) {
	if t.Config.APIKey == "" {
		return nil, errors.New("tavily search api key is required")
	}

	// Create default params
	params := TavilySearchParams{
		Query:       query,
		SearchDepth: t.Config.DefaultDepth,
		Topic:       t.Config.DefaultTopic,
		Days:        t.Config.DefaultDays,
		MaxResults:  t.Config.MaxResults,

		IncludeAnswer:     "basic",
		IncludeRawContent: false,
		IncludeImages:     false,
	}

	// Apply custom params if provided
	if customParams != nil {
		if customParams.SearchDepth != "" {
			params.SearchDepth = customParams.SearchDepth
		}
		if customParams.Topic != "" {
			params.Topic = customParams.Topic
		}
		if customParams.TimeRange != "" {
			params.TimeRange = customParams.TimeRange
		}
		if customParams.Days > 0 {
			params.Days = customParams.Days
		}
		if customParams.MaxResults > 0 {
			params.MaxResults = customParams.MaxResults
		}
		if len(customParams.IncludeDomains) > 0 {
			params.IncludeDomains = customParams.IncludeDomains
		}
		if len(customParams.ExcludeDomains) > 0 {
			params.ExcludeDomains = customParams.ExcludeDomains
		}
		if customParams.IncludeAnswer != nil {
			params.IncludeAnswer = customParams.IncludeAnswer
		}
		if customParams.IncludeRawContent != false {
			params.IncludeRawContent = customParams.IncludeRawContent
		}
		if customParams.IncludeImages != false {
			params.IncludeImages = customParams.IncludeImages
		}
	}

	// Parse the base URL
	parsedURL, err := url.Parse(t.Config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %v", err)
	}

	host := parsedURL.Host

	// Determine if HTTPS should be used
	isHttps := strings.HasPrefix(t.Config.BaseURL, "https://")

	// Marshal request body to JSON
	requestBody, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Create request packet
	requestPacket := []byte(fmt.Sprintf(tavilyRequestTemplate,
		"search",
		t.Config.APIKey,
		len(requestBody),
		string(requestBody)))

	// Prepare HTTP request options
	opts := []lowhttp.LowhttpOpt{
		lowhttp.WithHttps(isHttps),
		lowhttp.WithHost(host),
		lowhttp.WithTimeoutFloat(t.Config.Timeout),
		lowhttp.WithPacketBytes(requestPacket),
	}

	// Add proxy if specified
	if t.Config.Proxy != "" {
		opts = append(opts, lowhttp.WithProxy(t.Config.Proxy))
	}

	// Execute the HTTP request
	resp, err := lowhttp.HTTP(opts...)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %v", err)
	}

	// Check response status code
	statusCode := resp.GetStatusCode()
	if statusCode != 200 {
		// Handle specific error cases
		switch statusCode {
		case 400:
			return nil, fmt.Errorf("bad request: %s", string(resp.GetBody()))
		case 401:
			return nil, fmt.Errorf("invalid API key: %s", string(resp.GetBody()))
		case 403, 432, 433:
			return nil, fmt.Errorf("forbidden: %s", string(resp.GetBody()))
		case 429:
			return nil, fmt.Errorf("usage limit exceeded: %s", string(resp.GetBody()))
		default:
			return nil, fmt.Errorf("search request returned status code %d: %s", statusCode, string(resp.GetBody()))
		}
	}

	// Parse the response body
	var result TavilySearchResponse
	if err := json.Unmarshal(resp.GetBody(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %v", err)
	}

	return &result, nil
}

// Extract extracts content from the provided URLs
func (t *TavilySearchClient) Extract(urls []string, includeImages bool, extractDepth SearchDepth) (*TavilyExtractResponse, error) {
	if t.Config.APIKey == "" {
		return nil, errors.New("tavily search api key is required")
	}

	// Create params
	params := TavilyExtractParams{
		URLs:          urls,
		IncludeImages: includeImages,
		ExtractDepth:  extractDepth,
	}

	// Parse the base URL
	parsedURL, err := url.Parse(t.Config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %v", err)
	}

	host := parsedURL.Host

	// Determine if HTTPS should be used
	isHttps := strings.HasPrefix(t.Config.BaseURL, "https://")

	// Marshal request body to JSON
	requestBody, err := json.Marshal(params)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %v", err)
	}

	// Create request packet
	requestPacket := []byte(fmt.Sprintf(tavilyRequestTemplate,
		"extract",
		t.Config.APIKey,
		len(requestBody),
		string(requestBody)))

	// Prepare HTTP request options
	opts := []lowhttp.LowhttpOpt{
		lowhttp.WithHttps(isHttps),
		lowhttp.WithHost(host),
		lowhttp.WithTimeoutFloat(t.Config.Timeout),
		lowhttp.WithPacketBytes(requestPacket),
	}

	// Add proxy if specified
	if t.Config.Proxy != "" {
		opts = append(opts, lowhttp.WithProxy(t.Config.Proxy))
	}

	// Execute the HTTP request
	resp, err := lowhttp.HTTP(opts...)
	if err != nil {
		return nil, fmt.Errorf("extract request failed: %v", err)
	}

	// Check response status code
	statusCode := resp.GetStatusCode()
	if statusCode != 200 {
		// Handle specific error cases
		switch statusCode {
		case 400:
			return nil, fmt.Errorf("bad request: %s", string(resp.GetBody()))
		case 401:
			return nil, fmt.Errorf("invalid API key: %s", string(resp.GetBody()))
		case 403, 432, 433:
			return nil, fmt.Errorf("forbidden: %s", string(resp.GetBody()))
		case 429:
			return nil, fmt.Errorf("usage limit exceeded: %s", string(resp.GetBody()))
		default:
			return nil, fmt.Errorf("extract request returned status code %d: %s", statusCode, string(resp.GetBody()))
		}
	}

	// Parse the response body
	var result TavilyExtractResponse
	if err := json.Unmarshal(resp.GetBody(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse extract results: %v", err)
	}

	return &result, nil
}

// QnASearch performs a question and answer search
func (t *TavilySearchClient) QnASearch(query string, customParams *TavilySearchParams) (string, error) {
	params := &TavilySearchParams{
		SearchDepth:       AdvancedSearch,
		Topic:             t.Config.DefaultTopic,
		Days:              t.Config.DefaultDays,
		MaxResults:        t.Config.MaxResults,
		IncludeAnswer:     true,
		IncludeRawContent: false,
		IncludeImages:     false,
	}

	// Apply custom params if provided
	if customParams != nil {
		if customParams.Topic != "" {
			params.Topic = customParams.Topic
		}
		if customParams.Days > 0 {
			params.Days = customParams.Days
		}
		if customParams.MaxResults > 0 {
			params.MaxResults = customParams.MaxResults
		}
		if len(customParams.IncludeDomains) > 0 {
			params.IncludeDomains = customParams.IncludeDomains
		}
		if len(customParams.ExcludeDomains) > 0 {
			params.ExcludeDomains = customParams.ExcludeDomains
		}
	}

	// Perform the search
	response, err := t.SearchWithCustomParams(query, params)
	if err != nil {
		return "", err
	}

	return response.Answer, nil
}

// GetCompanyInfo searches for company information across multiple topics
func (t *TavilySearchClient) GetCompanyInfo(query string, searchDepth SearchDepth, maxResults int) ([]TavilySearchResult, error) {
	if searchDepth == "" {
		searchDepth = AdvancedSearch
	}

	if maxResults <= 0 {
		maxResults = t.Config.MaxResults
	}

	allResults := []TavilySearchResult{}

	// Perform searches for each topic
	for _, topic := range t.Config.CompanyInfoTags {
		params := &TavilySearchParams{
			SearchDepth:   searchDepth,
			Topic:         topic,
			MaxResults:    maxResults,
			IncludeAnswer: false,
		}

		response, err := t.SearchWithCustomParams(query, params)
		if err != nil {
			// Continue with other topics if one fails
			continue
		}

		allResults = append(allResults, response.Results...)
	}

	// Sort results by score (descending)
	for i := 0; i < len(allResults); i++ {
		for j := i + 1; j < len(allResults); j++ {
			if allResults[i].Score < allResults[j].Score {
				allResults[i], allResults[j] = allResults[j], allResults[i]
			}
		}
	}

	// Limit to maxResults
	if len(allResults) > maxResults {
		allResults = allResults[:maxResults]
	}

	return allResults, nil
}

// FormatResults formats the search results into a human-readable string
func FormatTavilyResults(response *TavilySearchResponse) string {
	if response == nil {
		return "No search results available"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for: %s\n\n", response.Query))

	// Check if we have an answer
	if response.Answer != "" {
		sb.WriteString("Answer: " + response.Answer + "\n\n")
	}

	// Add search results
	if len(response.Results) > 0 {
		sb.WriteString(fmt.Sprintf("Found %d results:\n\n", len(response.Results)))

		for i, result := range response.Results {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, result.Title))
			sb.WriteString(fmt.Sprintf("   URL: %s\n", result.URL))
			sb.WriteString(fmt.Sprintf("   Score: %.2f\n", result.Score))
			if result.PublishedDate != "" {
				sb.WriteString(fmt.Sprintf("   Published: %s\n", result.PublishedDate))
			}
			if result.Author != "" {
				sb.WriteString(fmt.Sprintf("   Author: %s\n", result.Author))
			}
			sb.WriteString(fmt.Sprintf("   %s\n", result.Content))
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("No results found.\n")
	}

	if len(response.FailedResults) > 0 {
		sb.WriteString("\nFailed URLs:\n")
		for _, url := range response.FailedResults {
			sb.WriteString(fmt.Sprintf("- %s\n", url))
		}
	}

	return sb.String()
}

// SearchFormatted performs a search and returns formatted results
func (t *TavilySearchClient) SearchFormatted(query string) (string, error) {
	response, err := t.Search(query)
	if err != nil {
		return "", err
	}

	return FormatTavilyResults(response), nil
}
