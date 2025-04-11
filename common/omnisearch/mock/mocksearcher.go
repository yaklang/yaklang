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

func (m *MockSearcher) Search(query string, config *ostype.SearchConfig) ([]*ostype.OmniSearchResult, error) {
	if config.ApiKey == "" {
		return nil, fmt.Errorf("api key is required")
	}
	results := []*ostype.OmniSearchResult{}
	results = append(results, &ostype.OmniSearchResult{
		Title:   "mock",
		URL:     "https://mock.com",
		Content: fmt.Sprintf("apikey: %s, mock %s", config.ApiKey, query),
	})
	return results, nil
}

func (m *MockSearcher) GetType() ostype.SearcherType {
	return ostype.SearcherType("mock")
}
