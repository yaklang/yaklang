package searchers

import (
	"testing"
)

func TestTravilySearch(t *testing.T) {
	client := NewDefaultTavilySearchClient()
	client.Config.APIKey = "xxx"
	results, err := client.SearchFormatted("yaklang")
	if err != nil {
		t.Fatal(err)
	}
	println(results)
}
