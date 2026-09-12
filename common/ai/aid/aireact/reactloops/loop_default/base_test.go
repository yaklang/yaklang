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
	require.Contains(t, text, "host-side completion gate")
	require.NotContains(t, text, "iteration 6")
	require.NotContains(t, text, "Current iteration")
	require.NotContains(t, text, "minimum iteration")
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
	require.Contains(t, text, "completion gate opens")
	require.NotContains(t, text, "iteration 6")
	require.NotContains(t, text, "minimum iteration")
}

func TestReactiveDataDoesNotExposeIterationDeadline(t *testing.T) {
	lower := strings.ToLower(reactiveDataTemplate)
	for _, forbidden := range []string{"islastiteration", "last iteration", "final iteration", "最后一次迭代", "最后一轮"} {
		require.NotContains(t, lower, strings.ToLower(forbidden))
	}
}

const testReviewedFinish = `{"@action":"finish","completion_review":{"goal_evidence":"The scripted note or answer has been emitted in the preceding action.","discovery_audit":"The scripted observation introduces no additional objects.","closure_audit":"No tool work or deferred blocker remains in this fixture."}}`

func TestPostSummaryAuditsAcceptanceAndUnfinishedWork(t *testing.T) {
	require.Contains(t, reActPostSummary, "## 验收结果与已关闭工作")
	require.Contains(t, reActPostSummary, "## 仍未完成的工作")
	require.Contains(t, reActPostSummary, "不把 deferred 算作执行完成")
	require.Contains(t, reActPostSummary, "未在 finish 前 add 并执行")
	require.NotContains(t, reActPostSummary, "## 可选后续")
	require.NotContains(t, reActPostSummary, "## 下一步建议")
}
