package aibp

import (
	"context"
	"testing"
	"time"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestTravelMaster(t *testing.T) {
	yakit.LoadGlobalNetworkConfig()
	result, err := aiforge.ExecuteForge("travelmaster", context.Background(), []*ypb.ExecParamItem{
		{Key: "query", Value: "帮我规划旅游计划，从成都到北京，7天，预算中等，兴趣爱好是文化、美食、科技，我需要详细的每日行程安排，包括住宿、餐饮和交通方案"}},
		aid.WithAICallback(aiforge.GetOpenRouterAICallback()),
		aid.WithAgreeAuto(100*time.Millisecond),
		aid.WithDebugPrompt(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(result.Formated)
}
