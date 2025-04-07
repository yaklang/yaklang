package searchers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils/lowhttp"
)

// Brave Search API request template with fixed formatting
const braveRequestTemplate = `GET /res/v1/web/search?%s HTTP/1.1
Host: api.search.brave.com
Accept: application/json
Accept-Encoding: gzip
User-Agent: Yaklang-BraveSearch/1.0
X-Subscription-Token: %s
`

// BraveSearchParams represents the query parameters for Brave Search API
type BraveSearchParams struct {
	Query           string   `json:"q"`                 // Required: Search query
	Country         string   `json:"country,omitempty"` // Optional: 2-letter country code
	SearchLang      string   `json:"search_lang,omitempty"`
	UILang          string   `json:"ui_lang,omitempty"`
	Count           int      `json:"count,omitempty"`            // Number of results (max 20)
	Offset          int      `json:"offset,omitempty"`           // Pagination offset (max 9)
	SafeSearch      string   `json:"safesearch,omitempty"`       // off, moderate, strict
	Freshness       string   `json:"freshness,omitempty"`        // pd, pw, pm, py or date range
	TextDecorations bool     `json:"text_decorations,omitempty"` // Whether to include highlighting
	Spellcheck      bool     `json:"spellcheck,omitempty"`       // Whether to enable spellcheck
	ResultFilter    string   `json:"result_filter,omitempty"`    // Comma-delimited list of result types
	Goggles         []string `json:"goggles,omitempty"`          // Custom re-ranking
	Units           string   `json:"units,omitempty"`            // metric or imperial
	ExtraSnippets   bool     `json:"extra_snippets,omitempty"`   // Enable additional snippets
	Summary         bool     `json:"summary,omitempty"`          // Enable summary generation
}

// BraveSearchResponse represents the response from the Brave Search API
type BraveSearchResponse struct {
	Type             string              `json:"type"`
	Query            QueryInfo           `json:"query"`
	Mixed            interface{}         `json:"mixed,omitempty"`
	Web              *WebResults         `json:"web,omitempty"`
	News             interface{}         `json:"news,omitempty"`
	Videos           interface{}         `json:"videos,omitempty"`
	Discussions      interface{}         `json:"discussions,omitempty"`
	Infobox          interface{}         `json:"infobox,omitempty"`
	Locations        interface{}         `json:"locations,omitempty"`
	GogglesAvailable []interface{}       `json:"goggles_available,omitempty"`
	SpellCheck       interface{}         `json:"spellcheck,omitempty"`
	FAQ              interface{}         `json:"faq,omitempty"`
	Summarizer       *SummarizerResponse `json:"summarizer,omitempty"`
	HasMore          bool                `json:"has_more"`
}

// SummarizerResponse represents the summarizer section of the response
type SummarizerResponse struct {
	SearchQuery     string        `json:"search_query"`
	Status          string        `json:"status"`
	Synopsis        string        `json:"synopsis"`
	GenDate         string        `json:"gen_date"`
	ResultType      string        `json:"result_type"`
	Results         []interface{} `json:"results"`
	Attribution     interface{}   `json:"attribution"`
	IsControlled    bool          `json:"is_controlled"`
	IsSummarizable  bool          `json:"is_summarizable"`
	GenerationTime  float64       `json:"generation_time"`
	FeedbackEnabled bool          `json:"feedback_enabled"`
}

// QueryInfo contains information about the search query
type QueryInfo struct {
	Original   string   `json:"original"`
	Altered    string   `json:"altered,omitempty"`
	QueryTime  float64  `json:"query_time"`
	Locale     string   `json:"locale"`
	Safesearch string   `json:"safesearch"`
	Goggles    []string `json:"goggles,omitempty"`
}

// WebResults contains the web search results
type WebResults struct {
	Results    []WebResult `json:"results"`
	TotalCount int         `json:"total_count"`
}

// WebResult represents a single search result
type WebResult struct {
	Title         string      `json:"title"`
	URL           string      `json:"url"`
	Description   string      `json:"description"`
	FaviconURL    string      `json:"favicon_url,omitempty"`
	Age           string      `json:"age,omitempty"`
	IsFamily      bool        `json:"is_family_friendly,omitempty"`
	MetricsURL    string      `json:"metrics_url,omitempty"`
	Language      string      `json:"language,omitempty"`
	ExtraSnippets []string    `json:"extra_snippets,omitempty"`
	DeepResults   interface{} `json:"deep_results,omitempty"`
	Tags          []string    `json:"tags,omitempty"`
	MetaURL       interface{} `json:"meta_url,omitempty"`
	Rank          int         `json:"rank,omitempty"`
	Source        string      `json:"source,omitempty"`
	Summary       string      `json:"summary,omitempty"`
}

// BraveSearchConfig holds configuration for the Brave search client
type BraveSearchConfig struct {
	APIKey        string
	BaseURL       string
	Timeout       float64
	MaxResults    int
	Proxy         string
	Country       string
	Language      string
	UILanguage    string
	SafeSearch    string
	Units         string
	EnableSummary bool
	ExtraSnippets bool
}

type BraveSearchClient struct {
	Config *BraveSearchConfig
}

func NewBraveSearchClient(config *BraveSearchConfig) *BraveSearchClient {
	return &BraveSearchClient{
		Config: config,
	}
}

func NewDefaultBraveSearchClient() *BraveSearchClient {
	return NewBraveSearchClient(NewDefaultBraveConfig())
}

// NewDefaultBraveConfig returns a default configuration for Brave search
func NewDefaultBraveConfig() *BraveSearchConfig {
	return &BraveSearchConfig{
		APIKey:        "", // API key should be set by the user
		BaseURL:       "https://api.search.brave.com/res/v1/web/search",
		Timeout:       10,
		MaxResults:    10,
		Country:       "US",       // Default country
		Language:      "en",       // Default search language
		UILanguage:    "en-US",    // Default UI language
		SafeSearch:    "moderate", // Default safesearch setting
		Units:         "metric",   // Default units
		EnableSummary: true,       // Enable summarization by default
		ExtraSnippets: false,      // Extra snippets disabled by default
	}
}

// Search performs a search query using Brave Search API
func BraveSearch(query string) ([]WebResult, error) {
	b := NewDefaultBraveSearchClient()
	results, err := b.Search(query)
	if err != nil {
		return nil, err
	}
	if results.Web != nil {
		return results.Web.Results, nil
	}
	return []WebResult{}, nil
}
func (b *BraveSearchClient) Search(query string) (*BraveSearchResponse, error) {
	return b.SearchWithCustomParams(query, nil)
}

// Search performs a search with the client's configuration
func (b *BraveSearchClient) SearchWithCustomParams(query string, customParams *BraveSearchParams) (*BraveSearchResponse, error) {
	if b.Config.APIKey == "" {
		return nil, errors.New("brave search api key is required")
	}

	// Create query parameters
	params := BraveSearchParams{
		Query:           query,
		Country:         b.Config.Country,
		SearchLang:      b.Config.Language,
		UILang:          b.Config.UILanguage,
		Count:           b.Config.MaxResults,
		Offset:          0,
		SafeSearch:      b.Config.SafeSearch,
		Units:           b.Config.Units,
		TextDecorations: true,
		Spellcheck:      true,
	}

	if b.Config.EnableSummary {
		params.Summary = true
	}

	if b.Config.ExtraSnippets {
		params.ExtraSnippets = true
	}

	if customParams != nil {
		if customParams.Country != "" {
			params.Country = customParams.Country
		}
		if customParams.SearchLang != "" {
			params.SearchLang = customParams.SearchLang
		}
		if customParams.UILang != "" {
			params.UILang = customParams.UILang
		}
		if customParams.Count > 0 {
			params.Count = customParams.Count
		}
		if customParams.Offset > 0 {
			params.Offset = customParams.Offset
		}
		if customParams.SafeSearch != "" {
			params.SafeSearch = customParams.SafeSearch
		}
		if customParams.Freshness != "" {
			params.Freshness = customParams.Freshness
		}
		if customParams.TextDecorations != false {
			params.TextDecorations = customParams.TextDecorations
		}
		if customParams.Spellcheck != false {
			params.Spellcheck = customParams.Spellcheck
		}
		if customParams.ResultFilter != "" {
			params.ResultFilter = customParams.ResultFilter
		}
		if len(customParams.Goggles) > 0 {
			params.Goggles = customParams.Goggles
		}
		if customParams.Units != "" {
			params.Units = customParams.Units
		}
		if customParams.ExtraSnippets != false {
			params.ExtraSnippets = customParams.ExtraSnippets
		}
		if customParams.Summary != false {
			params.Summary = customParams.Summary
		}
	}

	// Parse the base URL to extract host and scheme
	parsedURL, err := url.Parse(b.Config.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid base URL: %v", err)
	}

	host := parsedURL.Host

	// Determine if HTTPS should be used
	isHttps := strings.HasPrefix(b.Config.BaseURL, "https://")

	// Build query string
	values := url.Values{}
	values.Add("q", params.Query)

	if params.Country != "" {
		values.Add("country", params.Country)
	}

	if params.SearchLang != "" {
		values.Add("search_lang", params.SearchLang)
	}

	if params.UILang != "" {
		values.Add("ui_lang", params.UILang)
	}

	if params.Count > 0 {
		values.Add("count", strconv.Itoa(params.Count))
	}

	if params.Offset > 0 {
		values.Add("offset", strconv.Itoa(params.Offset))
	}

	if params.SafeSearch != "" {
		values.Add("safesearch", params.SafeSearch)
	}

	if params.Freshness != "" {
		values.Add("freshness", params.Freshness)
	}

	values.Add("text_decorations", strconv.FormatBool(params.TextDecorations))
	values.Add("spellcheck", strconv.FormatBool(params.Spellcheck))

	if params.ResultFilter != "" {
		values.Add("result_filter", params.ResultFilter)
	}

	for _, goggle := range params.Goggles {
		values.Add("goggles", goggle)
	}

	if params.Units != "" {
		values.Add("units", params.Units)
	}

	if params.ExtraSnippets {
		values.Add("extra_snippets", "true")
	}

	if params.Summary {
		values.Add("summary", "true")
	}

	// Prepare HTTP request options
	opts := []lowhttp.LowhttpOpt{
		lowhttp.WithHttps(isHttps),
		lowhttp.WithHost(host),
		lowhttp.WithTimeoutFloat(b.Config.Timeout),
	}

	// Create and add the request
	queryString := values.Encode()
	requestPacket := []byte(fmt.Sprintf(braveRequestTemplate, queryString, b.Config.APIKey))
	opts = append(opts, lowhttp.WithPacketBytes(requestPacket))

	// Add proxy if specified
	if b.Config.Proxy != "" {
		opts = append(opts, lowhttp.WithProxy(b.Config.Proxy))
	}

	// Execute the HTTP request
	resp, err := lowhttp.HTTP(opts...)
	if err != nil {
		return nil, fmt.Errorf("search request failed: %v", err)
	}

	// Check response status code
	statusCode := resp.GetStatusCode()
	if statusCode != 200 {
		return nil, fmt.Errorf("search request returned status code %d: %s", statusCode, string(resp.GetBody()))
	}

	// Parse the response body
	var result BraveSearchResponse
	if err := json.Unmarshal(resp.GetBody(), &result); err != nil {
		return nil, fmt.Errorf("failed to parse search results: %v", err)
	}

	return &result, nil
}

// SearchWithDetails performs a search and returns the full response
func (b *BraveSearchClient) SearchWithDetails(query string) (*BraveSearchResponse, error) {
	return b.Search(query)
}

// FormatResults formats the search results into a human-readable string
func FormatResults(response *BraveSearchResponse) string {
	if response == nil {
		return "No search results available"
	}

	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Search results for: %s\n\n", response.Query.Original))

	// Check if we have a summary
	if response.Summarizer != nil && response.Summarizer.Synopsis != "" {
		sb.WriteString("Summary: " + response.Summarizer.Synopsis + "\n\n")
	}

	// Add web results
	if response.Web != nil && len(response.Web.Results) > 0 {
		sb.WriteString(fmt.Sprintf("Found %d results:\n\n", response.Web.TotalCount))

		for i, result := range response.Web.Results {
			sb.WriteString(fmt.Sprintf("%d. %s\n", i+1, result.Title))
			sb.WriteString(fmt.Sprintf("   URL: %s\n", result.URL))
			sb.WriteString(fmt.Sprintf("   %s\n", result.Description))
			if result.Age != "" {
				sb.WriteString(fmt.Sprintf("   Age: %s\n", result.Age))
			}
			if len(result.Tags) > 0 {
				sb.WriteString(fmt.Sprintf("   Tags: %s\n", strings.Join(result.Tags, ", ")))
			}
			sb.WriteString("\n")
		}
	} else {
		sb.WriteString("No web results found.\n")
	}

	if response.HasMore {
		sb.WriteString("\nMore results are available.\n")
	}

	return sb.String()
}

// SearchFormatted performs a search and returns formatted results
func (b *BraveSearchClient) SearchFormatted(query string) (string, error) {
	response, err := b.Search(query)
	if err != nil {
		return "", err
	}

	return FormatResults(response), nil
}
