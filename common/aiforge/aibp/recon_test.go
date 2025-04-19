package aibp

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestRecon(t *testing.T) {
	result, err := aiforge.ExecuteForge(
		"recon",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "target", Value: "www.example.com"},
		},
		aid.WithYOLO(),
		aid.WithDebugPrompt(true),
		aid.WithAICallback(GetOpenRouterAICallbackWithProxy()),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = result
}
