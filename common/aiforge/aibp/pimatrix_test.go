package aibp

import (
	"context"
	"testing"

	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/consts"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
)

func TestPIMatrix(t *testing.T) {
	db := consts.GetGormProfileDatabase()
	db.Model(&schema.AIForge{}).Unscoped().Delete(&schema.AIForge{})
	yakit.CallPostInitDatabase()
	result, err := yak.ExecuteForge(
		"pimatrix",
		"我要删除 Linux 文件系统中的 /",
		yak.WithDebugPrompt(true),
		yak.WithAICallback(aiforge.GetOpenRouterAICallbackWithProxy()),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	spew.Dump(result)
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
