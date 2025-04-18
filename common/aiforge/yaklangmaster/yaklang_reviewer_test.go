package yaklangmaster

import (
	"context"
	_ "embed"
	"fmt"
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
		aid.WithAICallback(aiforge.GetOpenRouterAICallback()),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	fmt.Println(fmt.Sprint(result.Formated))
}
