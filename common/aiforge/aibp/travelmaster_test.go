package aibp

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"testing"
)

func TestTravelMaster(t *testing.T) {
	yakit.LoadGlobalNetworkConfig()
	result, err := ExecuteForge("travelmaster",
		"帮我规划旅游计划，从成都到北京，1天，1人，预算中等，兴趣爱好是文化、美食、科技，我需要详细的每日行程安排，包括住宿、餐饮和交通方案",
		aicommon.WithAgreeAuto(),
		aicommon.WithDebugPrompt(true),
	)
	if err != nil {
		t.Fatal(err)
	}
	spew.Dump(result)
}
