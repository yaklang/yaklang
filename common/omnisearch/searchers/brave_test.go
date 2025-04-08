package searchers

import (
	"testing"
)

func TestBraveSearch(t *testing.T) {
	b := NewDefaultBraveSearchClient()
	b.Config.APIKey = ""
	b.Config.Proxy = "http://127.0.0.1:8083"
	results, err := b.SearchFormatted("yaklang")
	if err != nil {
		t.Fatal(err)
	}
	println(results)
}
