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
	require.Contains(t, highStatic, "故障恢复")
	require.Contains(t, highStatic, "焦点流动")
	require.Contains(t, highStatic, "先用 todo_delta 记录分叉，再执行 Current")
	require.Contains(t, highStatic, "全部合格分支")
	require.Contains(t, highStatic, "再在同一 `todo_delta` 中设置下一 current")
	require.Contains(t, highStatic, "具体入口本身就是合格的覆盖分支")
	require.Contains(t, highStatic, "不要求先出现漏洞信号")
	require.Contains(t, highStatic, "验证型分支再补可证伪假设")
	require.Contains(t, highStatic, "单次工具失败、参数校验失败、超时、连接失败、认证拒绝")
	require.Contains(t, highStatic, "不能据此 close、deferred 或 finish")
	require.Contains(t, highStatic, "执行有区分力的替代实验")
	require.NotContains(t, highStatic, "严格优先级逆转")
	require.Contains(t, highStatic, "TODO 清空不是任务完成的充分证据")
	require.Contains(t, highStatic, "发现一个或多个满足四项的行动时不得 `finish`")
	require.Contains(t, highStatic, "只有空泛猜测且预期信息增益很低的建议属于非阻塞可选后续")
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
				"页面链接、表单 action、跳转、脚本路由、接口文档或响应字段",
				"具体入口本身就是合格的覆盖分支",
				"不要求先出现漏洞信号",
				"`add` / `update` 全部合格分支",
				"先用 todo_delta 记录分叉，再执行 Current",
			},
		},
		{
			name: "current branch keeps depth while sibling branches remain resumable",
			required: []string{
				"执行沿唯一 `current` 做连续实验闭环",
				"可独立测试的同级入口先入 Frontier",
				"当前项闭环或暂时不再产出信息后必须返回它们",
			},
		},
		{
			name: "completed or blocked current hands off without an empty focus window",
			required: []string{
				"`current` 仍有不同且有信息增益的下一步时继续向深处",
				"update 阶段结果、已尝试变化和恢复条件",
				"再在同一 `todo_delta` 中设置下一 current",
			},
		},
		{
			name: "one failure triggers recovery instead of abandonment",
			required: []string{
				"单次工具失败、参数校验失败、超时、连接失败、认证拒绝",
				"不能据此 close、deferred 或 finish",
				"改变方法、编码、参数通道、请求形态、身份/会话、基线与观察通道",
				"执行有区分力的替代实验",
			},
		},
		{
			name: "low value ideas do not inflate the frontier or justify finish",
			required: []string{
				"明确范围外、重复、没有观察依据或无法形成动作的空泛想法",
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
