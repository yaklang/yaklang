package searchers

import "github.com/yaklang/yaklang/common/omnisearch/ostype"

type OmniBraveSearchClient struct {
}

func NewOmniBraveSearchClient() *OmniBraveSearchClient {
	return &OmniBraveSearchClient{}
}

func (c *OmniBraveSearchClient) GetType() ostype.SearcherType {
	return ostype.SearcherTypeBrave
}

func (c *OmniBraveSearchClient) Search(query string, options ...ostype.SearchOption) (*ostype.OmniSearchResultList, error) {
	braveConfig := NewDefaultBraveConfig()
	config := ostype.NewSearchConfig(options...)
	if config.ApiKey != "" {
		braveConfig.APIKey = config.ApiKey
	}
	if config.Timeout != 0 {
		braveConfig.Timeout = float64(config.Timeout)
	}
	if config.BaseURL != "" {
		braveConfig.BaseURL = config.BaseURL
	}
	if config.Proxy != "" {
		braveConfig.Proxy = config.Proxy
	}

	var offset int
	if config.Page > 1 {
		offset = (config.Page - 1) * config.PageSize
	}
	response, err := NewBraveSearchClient(braveConfig).SearchWithCustomParams(query, &BraveSearchParams{
		Count:  config.PageSize,
		Offset: offset,
	})
	if err != nil {
		return nil, err
	}
	var res []*ostype.OmniSearchResult
	for _, result := range response.Web.Results {
		res = append(res, &ostype.OmniSearchResult{
			Title:      result.Title,
			URL:        result.URL,
			Content:    result.Description,
			Age:        result.Age,
			FaviconURL: result.FaviconURL,
			Source:     c.GetType().String(),
		})
	}
	return &ostype.OmniSearchResultList{
		Results: res,
		Total:   response.Web.TotalCount,
	}, nil
}
