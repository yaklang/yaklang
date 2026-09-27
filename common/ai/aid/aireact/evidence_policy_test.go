package aireact

import (
	_ "embed"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestPromptPolicyRequiresDiscriminatingEvidenceBeforeVerificationClosure(t *testing.T) {
	highStatic := aicommon.SharedPlanAndExecHighStaticTemplate
	require.Contains(t, highStatic, "区分性实验")
	require.Contains(t, highStatic, "性价比优先")
	require.Contains(t, highStatic, "单次未命中只说明本次未命中, 不单独证明目标不存在")
	require.Contains(t, highStatic, "## 实验方法论")
	require.Contains(t, highStatic, "**语义停滞判定**")
	require.Contains(t, highStatic, "调用次数多、轮次增加或耗时变长本身都不是重复证据")
	require.Contains(t, highStatic, "同一工具用于不同目标、参数 / payload、对照、假设、观察通道或独立复核属于有效探索")
	require.Contains(t, highStatic, "确认语义停滞后至少改变一项假设 / 关键变量 / 工具 / 观察通道再试")
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

	require.NotContains(t, verificationDynamicTemplate, "HasRepeatedExecutionPath")
	require.NotContains(t, verificationDynamicTemplate, "当前子任务已多次经过相似执行路径")
	require.Contains(t, verificationOutputExampleText, "单次阴性尝试不等于验证完成")
}
