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
		aid.WithAICallback(aiforge.GetOpenRouterAICallback()),
		//aid.WithAICallback(func(config *aid.Config, req *aid.AIRequest) (*aid.AIResponse, error) {
		//	return aiforge.GetOpenRouterAICallback()(config, req)
		//}),
		// aid.WithPlanAICallback(aiforge.GetQwenAICallback("deepseek-r1")),
		aid.WithAgreeAuto(true, 100*time.Millisecond),
		aid.WithDebugPrompt(true),
		aid.WithAITransactionRetry(5),
	)
}
