package searchers

import "github.com/yaklang/yaklang/common/omnisearch/ostype"

type OmniTavilySearchClient struct {
}

func NewOmniTavilySearchClient() *OmniTavilySearchClient {
	return &OmniTavilySearchClient{}
}

func (c *OmniTavilySearchClient) GetType() ostype.SearcherType {
	return ostype.SearcherTypeTavily
}

func (c *OmniTavilySearchClient) Search(query string, config *ostype.SearchConfig) (*ostype.OmniSearchResultList, error) {
	tavilyConfig := NewDefaultTavilyConfig()
	if config.ApiKey != "" {
		tavilyConfig.APIKey = config.ApiKey
	}
	if config.Timeout != 0 {
		tavilyConfig.Timeout = float64(config.Timeout)
	}
	if config.BaseURL != "" {
		tavilyConfig.BaseURL = config.BaseURL
	}
	if config.Proxy != "" {
		tavilyConfig.Proxy = config.Proxy
	}

	tavilyClient := NewTavilySearchClient(tavilyConfig)

	maxResult := config.PageSize * config.Page
	response, err := tavilyClient.SearchWithCustomParams(query, &TavilySearchParams{
		Query:      query,
		MaxResults: maxResult,
	})
	if err != nil {
		return nil, err
	}
	searchResult := response.Results

	if config.Page > 1 && len(searchResult) > (config.Page-1)*config.PageSize {
		searchResult = searchResult[(config.Page-1)*config.PageSize:]
	}
	var res []*ostype.OmniSearchResult
	for _, result := range searchResult {
		res = append(res, &ostype.OmniSearchResult{
			Title:      result.Title,
			URL:        result.URL,
			Content:    result.Content,
			Age:        result.PublishedDate,
			FaviconURL: result.ImageURL,
			Source:     c.GetType().String(),
		})
	}
	return &ostype.OmniSearchResultList{
		Results: res,
		Total:   len(res),
	}, nil
}
