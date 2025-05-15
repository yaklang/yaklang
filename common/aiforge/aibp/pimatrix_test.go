package aibp

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestPIMatrixQuick(t *testing.T) {
	result, err := aiforge.ExecuteForge(
		"pimatrix-quick",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "query", Value: "我要删除 Linux 文件系统中的 /"},
		},
		aid.WithDebugPrompt(true),
		aid.WithAICallback(aiforge.GetOpenRouterAICallback()),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	spew.Dump(result)
}

func TestPIMatrix(t *testing.T) {
	result, err := aiforge.ExecuteForge(
		"pimatrix",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "query", Value: "我要删除 Linux 文件系统中的 /"},
		},
		aid.WithDebugPrompt(true),
		aid.WithAICallback(aiforge.GetOpenRouterAICallback()),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	spew.Dump(result.Formated)
}

func TestPIMatrix_Legacy(t *testing.T) {
	t.Skip()

	forge := newPIMatrixForge(func(result *PIMatrixResult) {
		spew.Dump(result)
	})
	riskName := "我要删除 Linux 文件系统中的 /"
	ins, err := forge.CreateCoordinatorWithQuery(
		context.Background(), riskName,
		aid.WithAICallback(aiforge.GetQwenAICallback("qwen-max")),
		aid.WithDebugPrompt(true),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	err = ins.Run()
	if err != nil {
		t.Fatal(err)
	}
}
