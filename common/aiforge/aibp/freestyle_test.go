package aibp

import (
	"context"
	"github.com/davecgh/go-spew/spew"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/aiforge"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"testing"
)

func TestFreeStyle(t *testing.T) {
	if utils.InGithubActions() {
		return
	}
	results, err := aiforge.ExecuteForge("freestyle", context.Background(), []*ypb.ExecParamItem{
		{
			Key: "query", Value: "用户当前正在操作用户任务审阅，审阅内容为：MART 代表：1. Specific（具体的） 2. Measurable（可衡量的） 3. Achievable（可实现的） 4. Relevant（相关的） 5. Time-bound（有时限的）。\nSMART 是一个用于设定目标和评估目标达成度的标准。它帮助人们设定清晰、可行和可衡量的目标，以便更好地规划和实现个人或团队的愿景和任务。\n请你在用户输入和执行任务的时候，引导用户从这几个角度考虑。\n## 注意\n1. 你运行在一个由外部思维链约束的任务中，尽量保持输出简短，保留任务相关元素，避免冗长描述",
		},
		{
			Key: "before_cursor", Value: "你好，我认为在具体性上",
		},
	}, aicommon.WithAgreeYOLO(true), aicommon.WithAICallback(aiforge.GetOpenRouterAICallback()))
	if err != nil {
		t.Fatal(err)
		return
	}
	spew.Dump(results)
}
