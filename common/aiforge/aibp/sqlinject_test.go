package aibp

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestSQLInject(t *testing.T) {
	aiforge.ExecuteForge("sqlinject", context.Background(), []*ypb.ExecParamItem{
		{Key: "target", Value: "http://www.example.com?a=1"},
	}, aid.WithAICallback(GetOpenRouterAICallbackWithProxy()), aid.WithDebugPrompt(true))
}
