package aibp

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"testing"

	"github.com/yaklang/yaklang/common/ai"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/aiforge/aibp/tool_mocker"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSQLInject(t *testing.T) {
	yakit.LoadGlobalNetworkConfig()
	mockServer := tool_mocker.NewAiToolMockServer(aispec.WithDebugStream(true))
	aiforge.ExecuteForge("sqlinject", context.Background(), []*ypb.ExecParamItem{
		{Key: "target", Value: "http://www.example.com?a=1"},
	}, aid.WithAICallback(aicommon.AIChatToAICallbackType(ai.Chat)), aid.WithDebugPrompt(true),
		aid.WithAgreeYOLO(),
		aid.WithToolManager(mockServer.GetToolManager()))
}
