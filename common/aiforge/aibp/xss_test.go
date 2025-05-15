package aibp

import (
	"testing"

	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestXSS(t *testing.T) {
	yakit.CallPostInitDatabase()
	yak.ExecuteForge("xss",
		[]*ypb.ExecParamItem{
			{Key: "query", Value: `http://127.0.0.1:8787/xss/js/in-str?name=admin`},
		},
		yak.WithAICallback(aiforge.GetOpenRouterAICallback()),
	)
	// aiforge.ExecuteForge(
	// 	"xss",
	// 	context.Background(),
	// 	[]*ypb.ExecParamItem{
	// 		{Key: "query", Value: `http://127.0.0.1:8787/xss/js/in-str?name=admin`},
	// 	},
	// 	aid.WithAICallback(aiforge.GetOpenRouterAICallback()),
	// 	//aid.WithAICallback(func(config *aid.Config, req *aid.AIRequest) (*aid.AIResponse, error) {
	// 	//	return aiforge.GetOpenRouterAICallback()(config, req)
	// 	//}),
	// 	// aid.WithPlanAICallback(aiforge.GetQwenAICallback("deepseek-r1")),
	// 	aid.WithAgreeAuto(100*time.Millisecond),
	// 	aid.WithDebugPrompt(true),
	// 	aid.WithAITransactionRetry(5),
	// 	aid.WithTimeLineLimit(5),
	// 	aid.WithTimelineContentLimit(15000),
	// )
}
