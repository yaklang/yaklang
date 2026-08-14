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
	require.Contains(t, text, "do not use finish before iteration 6")
	require.False(t, strings.Contains(text, "Current iteration"))
}

// TestConfigExecutionPolicy_MultiAgentDirective asserts the multi-agent
// ExecutionPolicy line is written as a mandatory directive (MUST / 【强制】)
// so the model treats dispatch as a required first move, not a weak hint.
func TestConfigExecutionPolicy_MultiAgentDirective(t *testing.T) {
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithEnableMultiAgentMode(true),
	)
	text := cfg.GetExecutionPolicy()
	require.Contains(t, text, "MUST make dispatch_sub_react_agents your FIRST move")
	require.Contains(t, text, "MUST NOT use it to offload")
	require.NotContains(t, text, "MaxSubAgents")
	require.NotContains(t, text, "Goal mode")
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
	require.Contains(t, text, "Both modes are active")
	require.Contains(t, text, "reach iteration 6 before finishing")
}

func TestPostSummaryOnlyOffersNonBlockingFollowUps(t *testing.T) {
	require.Contains(t, reActPostSummary, "## 可选后续（不属于本次完成条件）")
	require.Contains(t, reActPostSummary, "finish 前升级为 TODO 并执行")
	require.Contains(t, reActPostSummary, "没有合适的非阻塞可选项时，省略整个章节")
	require.NotContains(t, reActPostSummary, "## 下一步建议")
}
