package imageutils

import (
	"context"
	"fmt"
	"github.com/yaklang/yaklang/common/utils"
	"testing"
)

func TestExtractVideoFrameContext(t *testing.T) {
	if utils.InGithubActions() {
		t.Skip()
		return
	}
	ch, err := ExtractVideoFrameContext(context.Background(), "/Users/v1ll4n/Projects/yaklang/vtestdata/demo.mp4")
	if err != nil {
		t.Fatal(err)
	}

	for frame := range ch {
		fmt.Println(frame.String())
		//t.Log(frame.String())
	}
}
