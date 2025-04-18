package yaklangmaster

import (
	"context"
	_ "embed"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

//go:embed test.yak
var testYak string

func TestYaklangMaster(t *testing.T) {
	result, err := aiforge.ExecuteForge(
		"yaklang-reviewer",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "code", Value: testYak},
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
