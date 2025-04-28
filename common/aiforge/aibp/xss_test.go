package aibp

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
	"time"
)

func TestXSS(t *testing.T) {
	aiforge.ExecuteForge(
		"xss",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "query", Value: `http://127.0.0.1:8787/xss/js/in-str?name=admin`},
		},
		//aid.WithAICallback(aiforge.GetQwenAICallback("qwen-max")),
		aid.WithAICallback(aiforge.GetOpenRouterAICallback()),
		aid.WithPlanAICallback(aiforge.GetQwenAICallback("deepseek-r1")),
		aid.WithAgreeAuto(true, 100*time.Millisecond),
		aid.WithDebugPrompt(true),
	)
}
