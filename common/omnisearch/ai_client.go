package omnisearch

import (
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/omnisearch/ostype"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

type AIWebSearchClient struct {
	config *aispec.AIConfig

	searchClient *OmniSearchClient
	searcherType ostype.SearcherType
}

// BuildHTTPOptions implements aispec.AIClient.
func (a *AIWebSearchClient) BuildHTTPOptions() ([]poc.PocConfigOption, error) {
	opts := []poc.PocConfigOption{
		poc.WithReplaceAllHttpPacketHeaders(map[string]string{
			"Content-Type": "application/json; charset=UTF-8",
			"Accept":       "application/json",
		}),
	}
	if a.config.Proxy != "" {
		opts = append(opts, poc.WithProxy(a.config.Proxy))
	}
	if a.config.Context != nil {
		opts = append(opts, poc.WithContext(a.config.Context))
	}
	if a.config.Timeout > 0 {
		opts = append(opts, poc.WithConnectTimeout(a.config.Timeout))
	}
	opts = append(opts, poc.WithTimeout(600))
	return opts, nil
}

// Chat implements aispec.AIClient.
func (a *AIWebSearchClient) Chat(query string, function ...any) (string, error) {
	if a.searchClient == nil {
		return "", errors.New("search client is not initialized")
	}

	// Create search options using the API key and searcher type
	searchOpts := []ostype.SearchOption{
		ostype.WithSearchType(a.searcherType),
		ostype.WithApiKey(a.config.APIKey),
	}

	if a.config.Proxy != "" {
		searchOpts = append(searchOpts, ostype.WithProxy(a.config.Proxy))
	}

	if a.config.Timeout > 0 {
		searchOpts = append(searchOpts, ostype.WithTimeout(utils.FloatSecondDuration(a.config.Timeout)))
	}

	// Perform the search
	results, err := a.searchClient.Search(query, searchOpts...)
	if err != nil {
		return "", fmt.Errorf("search failed: %v", err)
	}

	// Format the results as a string
	var formattedResults strings.Builder
	for _, result := range results.Results {
		formattedResults.WriteString(fmt.Sprintf("Title: %s\n", result.Title))
		formattedResults.WriteString(fmt.Sprintf("URL: %s\n", result.URL))
		if result.Content != "" {
			formattedResults.WriteString(fmt.Sprintf("Content: %s\n", result.Content))
		}
		if result.Age != "" {
			formattedResults.WriteString(fmt.Sprintf("Age: %s\n", result.Age))
		}
		formattedResults.WriteString("\n")
	}

	return formattedResults.String(), nil
}

// ChatEx implements aispec.AIClient.
func (a *AIWebSearchClient) ChatEx(details []aispec.ChatDetail, function ...any) ([]aispec.ChatChoice, error) {
	if len(details) == 0 {
		return nil, errors.New("empty chat details")
	}

	// Extract the query from chat details
	var query string
	for _, detail := range details {
		if detail.Role == "user" {
			// Add type assertion for detail.Content
			if content, ok := detail.Content.(string); ok {
				query = content
				break
			}
		}
	}

	if query == "" {
		return nil, errors.New("no user query found in chat details")
	}

	// Use Chat to get search results
	result, err := a.Chat(query)
	if err != nil {
		return nil, err
	}

	// Format as ChatChoice
	return []aispec.ChatChoice{
		{
			Message: aispec.ChatDetail{
				Role:    "assistant",
				Content: result,
			},
		},
	}, nil
}

// ChatStream implements aispec.AIClient.
func (a *AIWebSearchClient) ChatStream(query string) (io.Reader, error) {
	// Search doesn't natively support streaming, so we'll get the results all at once
	// and then return them as a stream
	result, err := a.Chat(query)
	if err != nil {
		return nil, err
	}

	// Return the result as a reader
	return strings.NewReader(result), nil
}

// CheckValid implements aispec.AIClient.
func (a *AIWebSearchClient) CheckValid() error {
	if a.config.APIKey == "" {
		return errors.New("API key is required")
	}

	if a.searcherType == "" {
		return errors.New("search type is required")
	}

	return nil
}

// ExtractData implements aispec.AIClient.
func (a *AIWebSearchClient) ExtractData(data string, desc string, fields map[string]any) (map[string]any, error) {
	// Not directly applicable to a search client, but we can perform a search
	// with the data as a query and structure the results
	results, err := a.Chat(data)
	if err != nil {
		return nil, err
	}

	// Create a simple result map
	return map[string]any{
		"results": results,
		"query":   data,
	}, nil
}

// GetModelList implements aispec.AIClient.
func (a *AIWebSearchClient) GetModelList() ([]*aispec.ModelMeta, error) {
	// Return available searcher types as "models"
	return []*aispec.ModelMeta{
		{Id: string(ostype.SearcherTypeBrave)},
		{Id: string(ostype.SearcherTypeTavily)},
	}, nil
}

// LoadOption implements aispec.AIClient.
func (a *AIWebSearchClient) LoadOption(opt ...aispec.AIConfigOption) {
	config := aispec.NewDefaultAIConfig(opt...)
	a.config = config

	// The model name in AI config corresponds to the SearcherType
	if a.config.Model != "" {
		a.searcherType = ostype.SearcherType(a.config.Model)
	} else {
		// Default to Brave search if not specified
		a.searcherType = ostype.SearcherTypeBrave
	}

	// Create the OmniSearchClient
	searchKey := &SearchKeyInfo{
		ApiKey: a.config.APIKey,
		Type:   a.searcherType,
	}

	a.searchClient = NewOmniSearchClient(WithSearchKeys(searchKey))
}

// SupportedStructuredStream implements aispec.AIClient.
func (a *AIWebSearchClient) SupportedStructuredStream() bool {
	// OmniSearch doesn't natively support structured streaming
	return false
}

// StructuredStream implements aispec.AIClient.
func (a *AIWebSearchClient) StructuredStream(query string, function ...any) (chan *aispec.StructuredData, error) {
	// Since we don't natively support structured streaming,
	// we'll perform the search and send the results as structured data
	result, err := a.Chat(query)
	if err != nil {
		return nil, err
	}

	// Create a channel and send the results
	ch := make(chan *aispec.StructuredData, 1)
	go func() {
		defer close(ch)
		ch <- &aispec.StructuredData{
			Id:         "1",
			Event:      "data",
			OutputText: result,
		}
	}()

	return ch, nil
}

var _ aispec.AIClient = (*AIWebSearchClient)(nil)
