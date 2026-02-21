package omnisearch

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/omnisearch/ostype"
)

func TestUnifuncsIntegration(t *testing.T) {
	keyBytes, err := os.ReadFile("/tmp/unifuncs.txt")
	if err != nil {
		t.Skipf("skip: cannot read api key file: %v", err)
	}
	apiKey := strings.TrimSpace(string(keyBytes))
	if apiKey == "" {
		t.Skip("skip: api key is empty")
	}

	results, err := Search("yaklang",
		ostype.WithSearchType(ostype.SearcherTypeUnifuncs),
		ostype.WithExtra("apikeys", []string{apiKey}),
		ostype.WithPageSize(5),
	)
	if err != nil {
		t.Fatalf("Search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("Search returned no results")
	}

	t.Logf("omnisearch.Search() returned %d results via unifuncs", len(results))
	for i, r := range results {
		t.Logf("  [%d] title=%s url=%s source=%s", i+1, r.Title, r.URL, r.Source)
		if r.Title == "" {
			t.Errorf("result %d has empty title", i+1)
		}
		if r.URL == "" {
			t.Errorf("result %d has empty url", i+1)
		}
		if r.Source != "unifuncs" {
			t.Errorf("result %d has wrong source: %s, expected unifuncs", i+1, r.Source)
		}
	}
}
