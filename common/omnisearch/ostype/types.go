package ostype

type SearcherType string

const (
	SearcherTypeBrave  SearcherType = "brave"
	SearcherTypeTavily SearcherType = "tavily"
)

func (s SearcherType) String() string {
	return string(s)
}

type OmniSearchResult struct {
	Title      string `json:"title"`
	URL        string `json:"url"`
	Age        string `json:"age,omitempty"`
	FaviconURL string `json:"favicon_url,omitempty"`
	Content    string `json:"content,omitempty"`
	Source     string `json:"source,omitempty"`
}

type OmniSearchResultList struct {
	Results []*OmniSearchResult
	Total   int
}

type SearchClient interface {
	Search(query string, options ...SearchOption) (*OmniSearchResultList, error)
	GetType() SearcherType
}
