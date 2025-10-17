package imageutils

import (
	"context"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestPage2ImageExtractor(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}

	result, err := ExtractDocumentPagesContext(context.Background(), "/Users/v1ll4n/Projects/yaklang/vtestdata/demo1.pdf")
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
