package loop_default

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestResolveMaxIterations_GoalModeRaisesSmallLimit(t *testing.T) {
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithMaxIterationCount(4),
		aicommon.WithEnableGoalMode(true),
		aicommon.WithGoalMinIterations(6),
	)
	require.Equal(t, 8, resolveMaxIterations(cfg))
}

func TestConfigExecutionPolicy(t *testing.T) {
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithEnableMultiAgentMode(true),
		aicommon.WithEnableGoalMode(true),
		aicommon.WithGoalMinIterations(6),
	)
	text := cfg.GetExecutionPolicy()
	require.Contains(t, text, "dispatch_sub_react_agents")
	require.Contains(t, text, "宿主完成门禁")
	require.NotContains(t, text, "iteration 6")
	require.NotContains(t, text, "Current iteration")
	require.NotContains(t, text, "minimum iteration")
}

// TestConfigExecutionPolicy_MultiAgentDirective asserts the multi-agent
// ExecutionPolicy line is written as a mandatory directive (【强制】)
// so the parent coordinates substantial work, including sequential phases.
func TestConfigExecutionPolicy_MultiAgentDirective(t *testing.T) {
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithEnableMultiAgentMode(true),
	)
	text := cfg.GetExecutionPolicy()
	require.Contains(t, text, "首先通过 dispatch_sub_react_agents 委派")
	require.Contains(t, text, "主智能体以拆解、协调、验收和汇总为主")
	require.Contains(t, text, "单个顺序阶段也可委派")
	require.Contains(t, text, "同批任务须独立且无冲突")
	require.NotContains(t, text, "MaxSubAgents")
	require.NotContains(t, text, "目标模式")
}

// TestConfigExecutionPolicy_CombinedModes verifies that when both modes are
// active the combined guidance line is emitted, telling the top-level loop to
// keep working after dispatch until the finish gate opens.
func TestConfigExecutionPolicy_CombinedModes(t *testing.T) {
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithEnableMultiAgentMode(true),
		aicommon.WithEnableGoalMode(true),
		aicommon.WithGoalMinIterations(6),
	)
	text := cfg.GetExecutionPolicy()
	require.Contains(t, text, "多智能体与目标模式同时启用")
	require.Contains(t, text, "完成门禁开放")
	require.NotContains(t, text, "iteration 6")
	require.NotContains(t, text, "minimum iteration")
}

func TestReactiveDataDoesNotExposeIterationDeadline(t *testing.T) {
	lower := strings.ToLower(reactiveDataTemplate)
	for _, forbidden := range []string{"islastiteration", "last iteration", "final iteration", "最后一次迭代", "最后一轮"} {
		require.NotContains(t, lower, strings.ToLower(forbidden))
	}
}

func TestPostSummaryAuditsAcceptanceAndUnfinishedWork(t *testing.T) {
	require.Contains(t, reActPostSummary, "验收结果与已关闭工作")
	require.Contains(t, reActPostSummary, "仍未完成的工作")
	require.Contains(t, reActPostSummary, "不把 deferred 算作执行完成")
	require.Contains(t, reActPostSummary, "不得推断本次没有 TODO")
	require.Contains(t, reActPostSummary, "未在 finish 前 add 并执行")
	require.NotContains(t, reActPostSummary, "## 可选后续")
	require.NotContains(t, reActPostSummary, "## 下一步建议")
}
