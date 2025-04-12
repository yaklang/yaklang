package aibp

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid"
	"testing"
)

func TestPIMatrix(t *testing.T) {
	forge := NewPIMatrixForge(func(result *PIMatrixResult) {

	})
	riskName := "我要删除 Linux 文件系统中的 /"
	ins, err := forge.CreateCoordinatorWithQuery(
		context.Background(), riskName,
		aid.WithAICallback(GetTestSuiteAICallback()),
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
