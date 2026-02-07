package searchers

import "github.com/yaklang/yaklang/common/omnisearch/ostype"

type OmniChatGLMSearchClient struct {
}

func NewOmniChatGLMSearchClient() *OmniChatGLMSearchClient {
	return &OmniChatGLMSearchClient{}
}

func (c *OmniChatGLMSearchClient) GetType() ostype.SearcherType {
	return ostype.SearcherTypeChatGLM
}

func (c *OmniChatGLMSearchClient) Search(query string, config *ostype.SearchConfig) ([]*ostype.OmniSearchResult, error) {
	glmConfig := NewDefaultChatGLMConfig()

	if config.ApiKey != "" {
		glmConfig.APIKey = config.ApiKey
	}
	if config.Timeout != 0 {
		glmConfig.Timeout = float64(config.Timeout)
	}
	if config.BaseURL != "" {
		glmConfig.BaseURL = config.BaseURL
	}
	if config.Proxy != "" {
		glmConfig.Proxy = config.Proxy
	}
	if config.PageSize > 0 {
		glmConfig.MaxResults = config.PageSize
	}

	// Support extra config fields
	if config.Extra != nil {
		if engine, ok := config.Extra["search_engine"].(string); ok && engine != "" {
			glmConfig.SearchEngine = engine
		}
		if contentSize, ok := config.Extra["content_size"].(string); ok && contentSize != "" {
			glmConfig.ContentSize = contentSize
		}
		if recency, ok := config.Extra["search_recency_filter"].(string); ok && recency != "" {
			glmConfig.SearchRecencyFilter = recency
		}
		if domain, ok := config.Extra["search_domain_filter"].(string); ok && domain != "" {
			glmConfig.SearchDomainFilter = domain
		}
		if intent, ok := config.Extra["search_intent"].(bool); ok {
			glmConfig.SearchIntent = intent
		}
	}

	// Handle pagination: ChatGLM API does not support offset/page,
	// so we request count = pageSize * page and slice the results
	count := config.PageSize
	if config.Page > 1 {
		count = config.PageSize * config.Page
	}
	if count > 50 {
		count = 50
	}

	response, err := NewChatGLMSearchClient(glmConfig).SearchWithCustomParams(query, &ChatGLMSearchRequest{
		Count: count,
	})
	if err != nil {
		return nil, err
	}

	searchResults := response.SearchResult

	// Slice for pagination (ChatGLM does not support offset natively)
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
		res = append(res, &ostype.OmniSearchResult{
			Title:      result.Title,
			URL:        result.Link,
			Content:    result.Content,
			Age:        result.PublishDate,
			FaviconURL: result.Icon,
			Source:     c.GetType().String(),
		})
	}
	return res, nil
}
