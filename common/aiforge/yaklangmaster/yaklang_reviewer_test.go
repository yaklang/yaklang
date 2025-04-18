package yaklangmaster

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestYaklangMaster(t *testing.T) {
	result, err := aiforge.ExecuteForge(
		"yaklang-reviewer",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "code", Value: `
poc.GEt("http://www.baidu.com")
poc.post("www.baidu.com")
a,err = fuzz.HTTPRequest('')`},
		},
		aid.WithDebugPrompt(true),
		aid.WithAICallback(aiforge.GetQwenAICallback()),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	spew.Dump(result)
}
