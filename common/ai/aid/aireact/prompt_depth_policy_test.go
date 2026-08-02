package aireact

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestPromptPolicyRequiresDiscriminatingEvidenceBeforeVerificationClosure(t *testing.T) {
	highStatic := aicommon.SharedPlanAndExecHighStaticTemplate
	require.Contains(t, highStatic, "区分性优先")
	require.Contains(t, highStatic, "候选升级")
	require.Contains(t, highStatic, "焦点锁定")
	require.Contains(t, highStatic, "行动一致性")
	require.Contains(t, highStatic, "TODO LIST 只是持久状态的只读投影")
	require.Contains(t, highStatic, "`todo_delta` 是唯一写入通道")
	require.Contains(t, highStatic, "状态变化必写")
	require.Contains(t, highStatic, "强制初始化")
	require.Contains(t, highStatic, "第一条可执行的正常动作必须同时携带 `todo_delta`")
	require.Contains(t, highStatic, "主次矛盾")
	require.Contains(t, highStatic, "不改写 Plan 子任务树和 coordinator 调度")
	require.Contains(t, highStatic, "显式 Survey 阶段可按专用提示完成广度建索引")
	require.Contains(t, highStatic, "分叉原子落档")
	require.Contains(t, highStatic, "深度优先")
	require.Contains(t, highStatic, "闭环交接")
	require.Contains(t, highStatic, "先用 todo_delta 记录分叉，再执行 Current")
	require.Contains(t, highStatic, "全部合格分支")
	require.Contains(t, highStatic, "严格优先级逆转")
	require.Contains(t, highStatic, "同一 `todo_delta` 中 close 旧项")
	require.Contains(t, highStatic, "具体目标、触发证据、可证伪假设和恢复后的第一步")
	require.Contains(t, highStatic, "TODO 清空不是任务完成的充分证据")
	require.Contains(t, highStatic, "发现一个或多个满足四项的行动时不得 `finish`")
	require.Contains(t, highStatic, "只有空泛猜测且预期信息增益很低的建议属于非阻塞可选后续")
	require.Contains(t, highStatic, "分支可以在当前主线闭环或前置条件满足后执行")
	require.Contains(t, highStatic, "不要求其工具已经在本轮暴露")
	require.NotContains(t, highStatic, "当前工具与权限可以立即执行")
	require.Contains(t, highStatic, "后续行动不越界")
	require.Contains(t, highStatic, "单次普通请求、单一 payload、扫描未命中或无明显报错不得 close")
	require.Contains(t, highStatic, "任务漂移先纠偏")
	require.NotContains(t, highStatic, "任务漂移即完成")
	require.Contains(t, highStatic, "不得重复或换句话再次交付同一答案")
	require.Contains(t, highStatic, "同一 CURRENT-TASK 中，不携带有效 `todo_delta` 的 `directly_answer` 最多成功一次")
	require.Contains(t, highStatic, "intent classifier 明确标记的 `simple_query` 例外")
	require.Contains(t, highStatic, "无剩余工作时立即用 \"标记完成\" 收口")

	require.Contains(t, verificationInstructionText, "安全测试阴性结论必须有区分力")
	require.Contains(t, verificationInstructionText, "漂移本身不能证明当前子任务完成")
	require.Contains(t, verificationInstructionText, "一条或多条合格后续路径")
	require.Contains(t, verificationInstructionText, "必须先用 `todo_delta` 把全部合格分支加入或更新到 Frontier")
	require.Contains(t, verificationInstructionText, "你本人仍不得输出 `todo_delta`")
	require.NotContains(t, verificationInstructionText, "安全测试否定结果 = 子任务完成")
	require.NotContains(t, verificationInstructionText, "只有当工具执行完全失败或没有任何相关输出时")

	require.Contains(t, verificationDynamicTemplate, "漂移只用于纠偏")
	require.Contains(t, verificationDynamicTemplate, "单次失败、扫描未命中或无明显报错")
	require.Contains(t, verificationOutputExampleText, "单次阴性尝试不等于验证完成")
}

func TestFrontierCurrentPromptPolicyCoversExecutionScenarios(t *testing.T) {
	policy := aicommon.SharedPlanAndExecHighStaticTemplate
	tests := []struct {
		name     string
		required []string
	}{
		{
			name: "multi-step work bootstraps todo delta before execution",
			required: []string{
				"TODO LIST 只是持久状态的只读投影",
				"`todo_delta` 是唯一写入通道",
				"第一条可执行的正常动作必须同时携带 `todo_delta`",
				"同一动作中执行它",
			},
		},
		{
			name: "multiple website entry points are recorded before one is tested",
			required: []string{
				"Observation 打开一条或多条稍后需要回访的有效路径时",
				"`add` / `update` 全部合格分支",
				"先用 todo_delta 记录分叉，再执行 Current",
			},
		},
		{
			name: "current branch keeps depth while sibling branches remain resumable",
			required: []string{
				"执行默认沿唯一 `current` 深度推进",
				"同级路径先落入 Frontier",
				"触发证据、可证伪假设和恢复后的第一步",
			},
		},
		{
			name: "completed or blocked current hands off without an empty focus window",
			required: []string{
				"切换前必须用已有 Observation",
				"阶段结果、阻塞原因和恢复条件",
				"在同一 `todo_delta` 中 close 旧项",
				"避免无 current 空窗",
			},
		},
		{
			name: "low value ideas do not inflate the frontier or justify finish",
			required: []string{
				"明确范围外、没有任何事实或弱信号支持、无法形成具体后续动作和重复的分支",
				"只有空泛猜测且预期信息增益很低的建议属于非阻塞可选后续",
				"存在开放关键项时不收口",
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
