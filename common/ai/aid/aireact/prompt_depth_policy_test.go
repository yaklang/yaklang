package aireact

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/promptloader"
)

var defaultLoopInstruction = promptloader.MustLoad("ai/aid/aireact/reactloops/loop_default/prompts/instruction.txt")

var defaultLoopOutputExample = promptloader.MustLoad("ai/aid/aireact/reactloops/loop_default/prompts/output_example.txt")

func TestFrontierCurrentPromptPolicyCoversExecutionScenarios(t *testing.T) {
	policy := aicommon.SharedPlanAndExecHighStaticTemplate + "\n" + defaultLoopInstruction + "\n" + defaultLoopOutputExample
	tests := []struct {
		name     string
		required []string
	}{
		{
			name: "multi-step work bootstraps todo delta before execution",
			required: []string{
				"`todo_delta` 是唯一写入通道",
				"第一条可执行动作就要建立初始待办集合并显式指定 `current`",
				"第一条动作同时建立初始 TODO 并指定 current",
			},
		},
		{
			name: "multiple website entry points are recorded before one is tested",
			required: []string{
				"Observation 打开新分支",
				"先用 `add` 落成\"待探索\"条目",
				"不写就等于遗忘",
				"再继续推进 `current`",
			},
		},
		{
			name: "current branch keeps depth while sibling branches remain resumable",
			required: []string{
				"唯一被标记为\"当前主要矛盾\"的一项",
				"哪怕新分支暂时不是 `current`, 也必须先 `add`",
				"收集齐\"待探索\"线索比机械地把单一路径走到底更重要",
			},
		},
		{
			name: "completed or blocked current hands off without an empty focus window",
			required: []string{
				"`current` 已经有结论、被证据排除、或被外部条件阻塞时",
				"先 `close` (写清 `outcome` + `reason`)",
				"再把 `current` 切换到下一条最有价值的开放项",
			},
		},
		{
			name: "one failure triggers recovery instead of abandonment",
			required: []string{
				"工具报错或协议异常不是停止条件",
				"改变请求方法 / 参数形态 / 输入通道 / 观察方式",
				"执行至少一条有实质差异的替代路径",
				"所有合理路径穷尽后才报告不可行",
			},
		},
		{
			name: "low value ideas do not inflate the frontier or justify finish",
			required: []string{
				"只要求范围内、具体、可追溯出处",
				"不机械展开无关文件或穷举猜测",
				"性价比不用于取消验收目标、降级开放项或决定 finish",
				"存在开放待办时以工具推进 `current`, 不得终结任务",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, fragment := range tt.required {
				require.Contains(t, policy, fragment)
			}
		})
	}
}
