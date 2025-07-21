package imageutils

import (
	"context"
	"testing"
)

func TestPage2ImageExtractor(t *testing.T) {
	result, err := ExtractDocumentPagesContext(context.Background(), "/Users/v1ll4n/Projects/yaklang/vtestdata/demo.pdf")
	if err != nil {
		panic(err)
	}

	for imgResult := range result {
		if imgResult == nil {
			t.Error("Received nil ImageResult")
			continue
		}
		t.Log(imgResult.String())
	}
}
