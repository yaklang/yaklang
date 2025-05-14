package aibp

import (
	"context"
	"testing"

	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestRecon(t *testing.T) {
	yakit.LoadGlobalNetworkConfig()
	result, err := aiforge.ExecuteForge(
		"recon",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "target", Value: "www.example.com"},
		},
		aid.WithAgreeYOLO(),
		aid.WithDebugPrompt(true),
		aid.WithAICallback(aiforge.GetOpenRouterAICallbackWithProxy()),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = result
}
