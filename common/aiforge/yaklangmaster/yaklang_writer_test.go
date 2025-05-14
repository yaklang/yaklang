package yaklangmaster

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
	"time"
)

func TestYaklangWriter(t *testing.T) {
	result, err := aiforge.ExecuteForge(
		"yaklang-writer",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "query", Value: "帮我写一个检查/tmp下所有文件和敏感信息的脚本"},
		},
		aid.WithAICallback(aiforge.GetOpenRouterAICallback()),
		aid.WithAgreeAuto(10*time.Second),
		aid.WithDebugPrompt(),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	spew.Dump(result)
}
