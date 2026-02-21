package ostype

type SearcherType string

const (
	SearcherTypeBrave     SearcherType = "brave"
	SearcherTypeTavily    SearcherType = "tavily"
	SearcherTypeAiBalance SearcherType = "aibalance"
	SearcherTypeChatGLM   SearcherType = "chatglm"
	SearcherTypeBocha     SearcherType = "bocha"
	SearcherTypeUnifuncs  SearcherType = "unifuncs"
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
	Data       any    `json:"data,omitempty"`
}

type YakitOmniSearchKeyConfig struct {
	APIKey string `app:"name:api_key,verbose:API Key,required:true"`
	Proxy  string `app:"name:proxy,verbose:Proxy,required:false"`
}

type OmniSearchResultList struct {
	Results []*OmniSearchResult
	Total   int
}

type SearchClient interface {
	Search(query string, config *SearchConfig) ([]*OmniSearchResult, error)
	GetType() SearcherType
}
