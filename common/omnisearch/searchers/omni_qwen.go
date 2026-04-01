package searchers

import "github.com/yaklang/yaklang/common/omnisearch/ostype"

type OmniQwenSearchClient struct {
}

func NewOmniQwenSearchClient() *OmniQwenSearchClient {
	return &OmniQwenSearchClient{}
}

func (c *OmniQwenSearchClient) GetType() ostype.SearcherType {
	return ostype.SearcherTypeQwen
}

func (c *OmniQwenSearchClient) Search(query string, config *ostype.SearchConfig) ([]*ostype.OmniSearchResult, error) {
	qwenConfig := NewDefaultQwenConfig()

	if config.ApiKey != "" {
		qwenConfig.APIKey = config.ApiKey
	}
	if config.Timeout > 0 && config.Timeout.Seconds() >= 1 {
		qwenConfig.Timeout = config.Timeout.Seconds()
	}
	if config.BaseURL != "" {
		qwenConfig.BaseURL = config.BaseURL
	}
	if config.Proxy != "" {
		qwenConfig.Proxy = config.Proxy
	}

	if config.Extra != nil {
		if model, ok := config.Extra["model"].(string); ok && model != "" {
			qwenConfig.Model = model
		}
		if strategy, ok := config.Extra["search_strategy"].(string); ok && strategy != "" {
			qwenConfig.SearchStrategy = strategy
		}
		if forced, ok := config.Extra["forced_search"].(bool); ok {
			qwenConfig.ForcedSearch = forced
		}
	}

	response, err := NewQwenSearchClient(qwenConfig).Search(query)
	if err != nil {
		return nil, err
	}

	var results []*ostype.OmniSearchResult
	for _, sr := range response.SearchResults {
		results = append(results, &ostype.OmniSearchResult{
			Title:      sr.Title,
			URL:        sr.URL,
			FaviconURL: sr.Icon,
			Source:     c.GetType().String(),
			Summary:    response.Summary,
		})
	}

	// When no search_info results but we have a summary, return a single result with the summary
	if len(results) == 0 && response.Summary != "" {
		results = append(results, &ostype.OmniSearchResult{
			Content: response.Summary,
			Source:  c.GetType().String(),
			Summary: response.Summary,
		})
	}

	return results, nil
}
