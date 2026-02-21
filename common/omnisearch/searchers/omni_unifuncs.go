package searchers

import "github.com/yaklang/yaklang/common/omnisearch/ostype"

type OmniUnifuncsSearchClient struct {
}

func NewOmniUnifuncsSearchClient() *OmniUnifuncsSearchClient {
	return &OmniUnifuncsSearchClient{}
}

func (c *OmniUnifuncsSearchClient) GetType() ostype.SearcherType {
	return ostype.SearcherTypeUnifuncs
}

func (c *OmniUnifuncsSearchClient) Search(query string, config *ostype.SearchConfig) ([]*ostype.OmniSearchResult, error) {
	unifuncsConfig := NewDefaultUnifuncsConfig()

	if config.ApiKey != "" {
		unifuncsConfig.APIKey = config.ApiKey
	}
	if config.Timeout != 0 {
		unifuncsConfig.Timeout = config.Timeout.Seconds()
	}
	if config.BaseURL != "" {
		unifuncsConfig.BaseURL = config.BaseURL
	}
	if config.Proxy != "" {
		unifuncsConfig.Proxy = config.Proxy
	}
	if config.PageSize > 0 {
		unifuncsConfig.MaxResults = config.PageSize
	}

	if config.Extra != nil {
		if freshness, ok := config.Extra["freshness"].(string); ok && freshness != "" {
			unifuncsConfig.Freshness = freshness
		}
	}

	customParams := &UnifuncsSearchRequest{
		Count: config.PageSize,
		Page:  config.Page,
	}
	if customParams.Count <= 0 {
		customParams.Count = unifuncsConfig.MaxResults
	}

	response, err := NewUnifuncsSearchClient(unifuncsConfig).SearchWithCustomParams(query, customParams)
	if err != nil {
		return nil, err
	}

	if response.Data == nil {
		return nil, nil
	}

	var res []*ostype.OmniSearchResult
	for _, result := range response.Data.WebPages {
		content := result.Snippet
		if result.Summary != "" {
			content = result.Summary
		}
		age := ""
		if result.DatePublished != nil {
			age = *result.DatePublished
		}
		res = append(res, &ostype.OmniSearchResult{
			Title:      result.Name,
			URL:        result.URL,
			Content:    content,
			Age:        age,
			FaviconURL: result.SiteIcon,
			Source:     c.GetType().String(),
		})
	}
	return res, nil
}
