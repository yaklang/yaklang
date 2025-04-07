package example

import (
	"os"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/omnisearch"
	types "github.com/yaklang/yaklang/common/omnisearch/ostype"
)

// TestSearchByBrave 测试使用Brave引擎进行搜索
func TestSearchByBrave(t *testing.T) {
	osearch := omnisearch.NewOmniSearchClient()
	result, err := osearch.Search("test", types.WithSearchType(types.SearcherTypeBrave), types.WithApiKey(os.Getenv("BRAVE_API_KEY")),
		types.WithProxy("http://127.0.0.1:8083"), types.WithPageSize(3))
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(result)
}

// TestSearchByTavily 测试使用Tavily引擎进行搜索
func TestSearchByTavily(t *testing.T) {
	osearch := omnisearch.NewOmniSearchClient()
	result, err := osearch.Search("test123", types.WithSearchType(types.SearcherTypeTavily), types.WithApiKey(os.Getenv("TAVILY_API_KEY")),
		types.WithProxy("http://127.0.0.1:8083"), types.WithPageSize(3))
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(result)
}
