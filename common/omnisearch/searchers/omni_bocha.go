package searchers

import "github.com/yaklang/yaklang/common/omnisearch/ostype"

type OmniBochaSearchClient struct {
}

func NewOmniBochaSearchClient() *OmniBochaSearchClient {
	return &OmniBochaSearchClient{}
}

func (c *OmniBochaSearchClient) GetType() ostype.SearcherType {
	return ostype.SearcherTypeBocha
}

func (c *OmniBochaSearchClient) Search(query string, config *ostype.SearchConfig) ([]*ostype.OmniSearchResult, error) {
	bochaConfig := NewDefaultBochaConfig()

	if config.ApiKey != "" {
		bochaConfig.APIKey = config.ApiKey
	}
	if config.Timeout != 0 {
		bochaConfig.Timeout = float64(config.Timeout)
	}
	if config.BaseURL != "" {
		bochaConfig.BaseURL = config.BaseURL
	}
	if config.Proxy != "" {
		bochaConfig.Proxy = config.Proxy
	}
	if config.PageSize > 0 {
		bochaConfig.MaxResults = config.PageSize
	}

	// Support extra config fields
	if config.Extra != nil {
		if freshness, ok := config.Extra["freshness"].(string); ok && freshness != "" {
			bochaConfig.Freshness = freshness
		}
		if summary, ok := config.Extra["summary"].(bool); ok {
			bochaConfig.Summary = summary
		}
	}

	// Handle pagination: Bocha API does not support offset/page,
	// so we request count = pageSize * page and slice the results
	count := config.PageSize
	if config.Page > 1 {
		count = config.PageSize * config.Page
	}
	if count > 50 {
		count = 50
	}

	response, err := NewBochaSearchClient(bochaConfig).SearchWithCustomParams(query, &BochaSearchRequest{
		Count: count,
	})
	if err != nil {
		return nil, err
	}

	if response.Data == nil || response.Data.WebPages == nil {
		return nil, nil
	}

	searchResults := response.Data.WebPages.Value

	// Slice for pagination (Bocha does not support offset natively)
	if config.Page > 1 {
		startIdx := (config.Page - 1) * config.PageSize
		if startIdx < len(searchResults) {
			searchResults = searchResults[startIdx:]
		} else {
			searchResults = nil
		}
	}

	var res []*ostype.OmniSearchResult
	for _, result := range searchResults {
		content := result.Snippet
		if result.Summary != "" {
			content = result.Summary
		}
		res = append(res, &ostype.OmniSearchResult{
			Title:      result.Name,
			URL:        result.URL,
			Content:    content,
			Age:        result.DateLastCrawled,
			FaviconURL: result.SiteIcon,
			Source:     c.GetType().String(),
		})
	}
	return res, nil
}
