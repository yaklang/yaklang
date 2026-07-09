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

// TestConfigExecutionPolicy_MultiAgentSoftPreference asserts the multi-agent
// line is marked as a non-enforced preference so callers don't assume the
// framework forces dispatch.
func TestConfigExecutionPolicy_MultiAgentSoftPreference(t *testing.T) {
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithEnableMultiAgentMode(true),
	)
	text := cfg.GetExecutionPolicy()
	require.Contains(t, text, "preference, not hard-enforced")
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
