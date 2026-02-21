package searchers

import (
	"os"
	"strings"
	"testing"

	"github.com/yaklang/yaklang/common/omnisearch/ostype"
)

func TestUnifuncsSearch(t *testing.T) {
	keyBytes, err := os.ReadFile("/tmp/unifuncs.txt")
	if err != nil {
		t.Skipf("skip unifuncs test: cannot read api key file: %v", err)
	}
	apiKey := strings.TrimSpace(string(keyBytes))
	if apiKey == "" {
		t.Skip("skip unifuncs test: api key is empty")
	}

	client := NewUnifuncsSearchClient(&UnifuncsSearchConfig{
		APIKey:     apiKey,
		BaseURL:    "https://api.unifuncs.com/api/web-search/search",
		Timeout:    15,
		MaxResults: 5,
	})

	resp, err := client.Search("yaklang")
	if err != nil {
		t.Fatalf("unifuncs search failed: %v", err)
	}

	if resp.Code != 0 {
		t.Fatalf("unifuncs api returned error code: %d, message: %s", resp.Code, resp.Message)
	}

	if resp.Data == nil || len(resp.Data.WebPages) == 0 {
		t.Fatal("unifuncs search returned no results")
	}

	t.Logf("got %d results", len(resp.Data.WebPages))
	for i, r := range resp.Data.WebPages {
		t.Logf("  [%d] %s - %s", i+1, r.Name, r.URL)
		if r.Name == "" {
			t.Errorf("result %d has empty name", i+1)
		}
		if r.URL == "" {
			t.Errorf("result %d has empty url", i+1)
		}
	}
}

func TestOmniUnifuncsSearch(t *testing.T) {
	keyBytes, err := os.ReadFile("/tmp/unifuncs.txt")
	if err != nil {
		t.Skipf("skip unifuncs test: cannot read api key file: %v", err)
	}
	apiKey := strings.TrimSpace(string(keyBytes))
	if apiKey == "" {
		t.Skip("skip unifuncs test: api key is empty")
	}

	client := NewOmniUnifuncsSearchClient()

	results, err := client.Search("yaklang", &ostype.SearchConfig{
		ApiKey:   apiKey,
		PageSize: 5,
	})
	if err != nil {
		t.Fatalf("omni unifuncs search failed: %v", err)
	}

	if len(results) == 0 {
		t.Fatal("omni unifuncs search returned no results")
	}

	t.Logf("got %d results via omni adapter", len(results))
	for i, r := range results {
		t.Logf("  [%d] title=%s url=%s source=%s", i+1, r.Title, r.URL, r.Source)
		if r.Title == "" {
			t.Errorf("result %d has empty title", i+1)
		}
		if r.URL == "" {
			t.Errorf("result %d has empty url", i+1)
		}
		if r.Source != "unifuncs" {
			t.Errorf("result %d has wrong source: %s", i+1, r.Source)
		}
	}
}
