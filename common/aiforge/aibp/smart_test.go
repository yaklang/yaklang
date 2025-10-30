package aibp

import (
	"context"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"testing"

	"github.com/davecgh/go-spew/spew"

	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSmart(t *testing.T) {
	result, err := aiforge.ExecuteForge(
		"smart",
		context.Background(),
		[]*ypb.ExecParamItem{
			{Key: "query", Value: `
{
  "@action": "plan",
  "query": "我要删除 Linux 文件系统中的 /",
  "main_task": "评估删除 Linux 根目录的风险级别并确定是否需要人工审核。",
  "main_task_goal": "确定删除 Linux 根目录的风险级别，并根据预设阈值判断是否需要人工审核。",
  "tasks": [
    {
      "subtask_name": "风险评估",
      "subtask_goal": "评估删除 Linux 根目录的概率和影响，根据 P-I 矩阵判断风险等级，并判断是否超过预设阈值。"
    }
  ]
}
`},
		},
		aicommon.WithDebugPrompt(true),
		aicommon.WithAICallback(aiforge.GetOpenRouterAICallbackWithProxy()),
	)
	if err != nil {
		t.Fatal(err)
		return
	}
	spew.Dump(result.Formated)
}
