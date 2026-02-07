package searchers

import "github.com/yaklang/yaklang/common/omnisearch/ostype"

// OmniAiBalanceSearchClient wraps AiBalanceSearchClient to implement the ostype.SearchClient interface.
// It connects to an AiBalance relay server for web search, enabling load-balanced, multi-key web search
// through the AiBalance infrastructure.
type OmniAiBalanceSearchClient struct {
}

func NewOmniAiBalanceSearchClient() *OmniAiBalanceSearchClient {
	return &OmniAiBalanceSearchClient{}
}

func (c *OmniAiBalanceSearchClient) GetType() ostype.SearcherType {
	return ostype.SearcherTypeAiBalance
}

func (c *OmniAiBalanceSearchClient) Search(query string, config *ostype.SearchConfig) ([]*ostype.OmniSearchResult, error) {
	aibalanceConfig := NewDefaultAiBalanceConfig()

	if config.ApiKey != "" {
		aibalanceConfig.APIKey = config.ApiKey
	}
	if config.BaseURL != "" {
		aibalanceConfig.BaseURL = config.BaseURL
	}
	if config.Proxy != "" {
		aibalanceConfig.Proxy = config.Proxy
	}
	if config.Timeout != 0 {
		aibalanceConfig.Timeout = config.Timeout.Seconds()
	}

	// Allow specifying backend searcher type via Extra map
	if config.Extra != nil {
		if backendType, ok := config.Extra["backend_searcher_type"]; ok {
			if bt, ok := backendType.(string); ok && bt != "" {
				aibalanceConfig.BackendSearcherType = bt
			}
		}
	}

	page := config.Page
	if page <= 0 {
		page = 1
	}
	pageSize := config.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}

	response, err := NewAiBalanceSearchClient(aibalanceConfig).Search(query, page, pageSize)
	if err != nil {
		return nil, err
	}

	// The AiBalance response already contains OmniSearchResult objects,
	// but we ensure the Source field is set correctly
	for _, result := range response.Results {
		if result.Source == "" {
			result.Source = c.GetType().String()
		}
	}

	return response.Results, nil
}
