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
	require.Contains(t, highStatic, "TODO 清空不是任务完成的充分证据")
	require.Contains(t, highStatic, "四项均满足时不得 `finish`")
	require.Contains(t, highStatic, "预期信息增益很低的建议属于非阻塞可选后续")
	require.Contains(t, highStatic, "后续行动不越界")
	require.Contains(t, highStatic, "单次普通请求、单一 payload、扫描未命中或无明显报错不得 close")
	require.Contains(t, highStatic, "任务漂移先纠偏")
	require.NotContains(t, highStatic, "任务漂移即完成")

	require.Contains(t, verificationInstructionText, "安全测试阴性结论必须有区分力")
	require.Contains(t, verificationInstructionText, "漂移本身不能证明当前子任务完成")
	require.NotContains(t, verificationInstructionText, "安全测试否定结果 = 子任务完成")
	require.NotContains(t, verificationInstructionText, "只有当工具执行完全失败或没有任何相关输出时")

	require.Contains(t, verificationDynamicTemplate, "漂移只用于纠偏")
	require.Contains(t, verificationDynamicTemplate, "单次失败、扫描未命中或无明显报错")
	require.Contains(t, verificationOutputExampleText, "单次阴性尝试不等于验证完成")
}
