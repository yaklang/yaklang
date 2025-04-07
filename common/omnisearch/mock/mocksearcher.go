package mock

import (
	"fmt"

	"github.com/yaklang/yaklang/common/omnisearch/ostype"
)

type MockSearcher struct {
}

func NewMockSearcher() *MockSearcher {
	return &MockSearcher{}
}

func (m *MockSearcher) Search(query string, options ...ostype.SearchOption) (*ostype.OmniSearchResultList, error) {
	config := ostype.NewSearchConfig(options...)

	if config.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	results := &ostype.OmniSearchResultList{}
	results.Results = append(results.Results, &ostype.OmniSearchResult{
		Title:   "mock",
		URL:     "https://mock.com",
		Content: fmt.Sprintf("apikey: %s, mock %s", config.ApiKey, query),
	})
	return results, nil
}

func (m *MockSearcher) GetType() ostype.SearcherType {
	return ostype.SearcherType("mock")
}
