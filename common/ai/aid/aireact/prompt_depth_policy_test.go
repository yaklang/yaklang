package aireact

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

//go:embed reactloops/loop_default/prompts/instruction.txt
var defaultLoopInstruction string

//go:embed reactloops/loop_default/prompts/output_example.txt
var defaultLoopOutputExample string

func TestPromptPolicyRequiresDiscriminatingEvidenceBeforeVerificationClosure(t *testing.T) {
	highStatic := aicommon.SharedPlanAndExecHighStaticTemplate
	require.Contains(t, highStatic, "区分性优先")
	require.Contains(t, highStatic, "性价比优先")
	require.Contains(t, highStatic, "阴性结论 / 歧义场景专用")
	require.Contains(t, defaultLoopInstruction, "`todo_delta` 是唯一写入通道")
	require.Contains(t, highStatic, "集中在任务指令段")
	require.Contains(t, highStatic, "`todo_delta` 与 Plan 子任务树服务不同尺度")
	require.Contains(t, defaultLoopInstruction, "兄弟任务的 TODO 只读可见")
	require.NotContains(t, highStatic, "严格优先级逆转")
	require.NotContains(t, highStatic, "当前工具与权限可以立即执行")
	require.Contains(t, highStatic, "后续行动不越界")
	require.Contains(t, highStatic, "单次未命中不等于不存在")
	require.Contains(t, highStatic, "任务漂移先纠偏")
	require.NotContains(t, highStatic, "任务漂移即完成")
	require.Contains(t, defaultLoopInstruction, "同一 CURRENT-TASK 中不携带有效 `todo_delta` 的 `directly_answer` 最多成功一次")
	require.Contains(t, defaultLoopInstruction, "`simple_query` 例外")
	require.Contains(t, highStatic, "无剩余工作时立即用 \"标记完成\" 收口")

	require.Contains(t, defaultLoopInstruction, "## TODO 状态维护（todo_delta）")
	require.Contains(t, defaultLoopInstruction, "Frontier（前沿）")
	require.Contains(t, defaultLoopInstruction, "第一条可执行动作就要建立初始 TODO 集合并显式指定 `current`")
	require.Contains(t, defaultLoopInstruction, "Observation 打开了新的分支")
	require.Contains(t, defaultLoopInstruction, "具体目标")
	require.Contains(t, defaultLoopInstruction, "来源证据")
	require.Contains(t, defaultLoopInstruction, "下一步动作")
	require.Contains(t, defaultLoopInstruction, "文本以\"待探索：\"开头")
	require.Contains(t, defaultLoopInstruction, "存在开放 TODO 时不得 `finish`")
	require.Contains(t, defaultLoopInstruction, "不得为清空列表伪造 `resolved`")
	require.Contains(t, defaultLoopInstruction, "一条一任务")
	require.Contains(t, defaultLoopOutputExample, "`todo_delta` 使用案例")
	require.Contains(t, defaultLoopOutputExample, "`save_evidence` 使用案例")

	require.Contains(t, verificationInstructionText, "安全测试阴性结论必须有区分力")
	require.Contains(t, verificationInstructionText, "漂移本身不能证明当前子任务完成")
	require.Contains(t, verificationInstructionText, "页面链接、表单 action、跳转、脚本路由、文档端点、响应字段")
	require.Contains(t, verificationInstructionText, "验证型路径再写可证伪假设")
	require.Contains(t, verificationInstructionText, "单次工具、参数、连接、认证、空响应或 payload 失败不能证明路径结束")
	require.Contains(t, verificationInstructionText, "必须先用 `todo_delta` 把全部合格分支加入或更新到 Frontier")
	require.Contains(t, verificationInstructionText, "你本人仍不得输出 `todo_delta`")
	require.NotContains(t, verificationInstructionText, "安全测试否定结果 = 子任务完成")
	require.NotContains(t, verificationInstructionText, "只有当工具执行完全失败或没有任何相关输出时")

	require.Contains(t, verificationDynamicTemplate, "漂移只用于纠偏")
	require.Contains(t, verificationDynamicTemplate, "单次失败、扫描未命中或无明显报错")
	require.Contains(t, verificationOutputExampleText, "单次阴性尝试不等于验证完成")
}

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
				"第一条可执行动作就要建立初始 TODO 集合并显式指定 `current`",
				"第一条动作同时建立初始 TODO 并指定 current",
			},
		},
		{
			name: "multiple website entry points are recorded before one is tested",
			required: []string{
				"Observation 打开了新的分支",
				"先用 `add` / `update` 把这些分支记为\"待探索\"",
				"不写就等于遗忘",
				"先用 `add` / `update` 把这些分支记为\"待探索\"再继续推进 `current`",
			},
		},
		{
			name: "current branch keeps depth while sibling branches remain resumable",
			required: []string{
				"`current` 是其中唯一被标记为\"当前主要矛盾\"的一项",
				"哪怕它暂时不是 `current`，也必须先用 `add` 落成一条 TODO",
				"收集齐\"待探索\"线索比机械地把单一路径走到底更重要",
			},
		},
		{
			name: "completed or blocked current hands off without an empty focus window",
			required: []string{
				"`current` 已经有结论、被证据排除、或被外部条件阻塞时",
				"先 `close`（写清 `outcome` + 非空 `reason`）",
				"再把 `current` 切换到下一条最有价值的开放项",
			},
		},
		{
			name: "one failure triggers recovery instead of abandonment",
			required: []string{
				"工具报错或协议异常不是停止条件",
				"改变请求方法、参数形态、输入通道、会话上下文或观察方式",
				"执行至少一条有实质差异的合理替代路径",
				"所有合理路径穷尽后才报告不可行",
			},
		},
		{
			name: "low value ideas do not inflate the frontier or justify finish",
			required: []string{
				"只要求范围内、具体、可追溯出处",
				"无目标、无来源、无下一步",
				"存在开放 TODO 时不得 `finish`",
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
