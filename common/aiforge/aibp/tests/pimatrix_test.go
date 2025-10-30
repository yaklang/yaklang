package tests

import (
	"embed"
	"fmt"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"testing"

	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"gotest.tools/v3/assert"
)

//go:embed testsdata/*
var allForgeTestData embed.FS

func DebugExecuteForge(forgeName string, i any, iopts ...any) (any, error) {
	yakit.CallPostInitDatabase()
	var aiCallback aicommon.AICallbackType
	data, err := allForgeTestData.ReadFile(fmt.Sprintf("testsdata/%s.json", forgeName))
	if err != nil {
		return nil, err
	}
	aiCallback = aiforge.MockAICallbackByRecord(data)
	iopts = append(iopts, aicommon.WithAICallback(aiCallback))
	iopts = append(iopts, aicommon.WithDebug(true))
	iopts = append(iopts, aicommon.WithAgreeYOLO())
	return yak.ExecuteForge(forgeName, i, iopts...)
}

func TestPIMatrix(t *testing.T) {
	result, err := DebugExecuteForge(
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
