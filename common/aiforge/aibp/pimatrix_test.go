package aibp

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"testing"

	"github.com/davecgh/go-spew/spew"

	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"gotest.tools/v3/assert"
)

func TestPIMatrixQuick(t *testing.T) {
	result, err := aiforge.ExecuteForge(
		"pimatrix-quick",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "query", Value: "我要删除 Linux 文件系统中的 /"},
		},
		aicommon.WithDebugPrompt(true),
		aicommon.WithAICallback(aiforge.GetHoldAICallback()),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	spew.Dump(result)
}

func TestPIMatrix(t *testing.T) {
	result, err := ExecuteForge(
		"pimatrix",
		"我要删除 Linux 文件系统中的 /",
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	res := result.(map[string]any)["Impact"]
	assert.Equal(t, utils.InterfaceToFloat64(res) > 0.5, true)
}
