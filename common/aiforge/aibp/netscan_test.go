package aibp

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestNetScan(t *testing.T) {
	result, err := aiforge.ExecuteForge(
		`netscan`,
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "query", Value: "www.example.com"},
		},
		aid.WithYOLO(),
		aid.WithDebugPrompt(true),
		aid.WithAICallback(aiforge.GetQwenAICallback("qwen3-30b-a3b")),
	)
	if err != nil {
		t.Fatal(err)
	}
	_ = result
}
