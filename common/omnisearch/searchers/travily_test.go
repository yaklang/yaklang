package searchers

import (
	"testing"
)

func TestTravilySearch(t *testing.T) {
	client := NewDefaultTavilySearchClient()
	client.Config.APIKey = "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"
	client.Config.Proxy = "http://127.0.0.1:8083"
	results, err := client.SearchFormatted("yaklang")
	if err != nil {
		t.Fatal(err)
	}
	println(results)
}
