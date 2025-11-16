package aibp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/yaklang/yaklang/common/utils"
)

//func TestPIMatrixQuick(t *testing.T) {
//	result, err := aiforge.ExecuteForge(
//		"pimatrix-quick",
//		context.Background(),
//		[]*ypb.ExecParamItem{
//			{Key: "query", Value: "我要删除 Linux 文件系统中的 /"},
//		},
//		aicommon.WithDebugPrompt(true),
//		aicommon.WithAICallback(aiforge.GetHoldAICallback()),
//	)
//	if err != nil {
//		t.Fatal(err)
//		return
//	}
//	spew.Dump(result)
//}

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
