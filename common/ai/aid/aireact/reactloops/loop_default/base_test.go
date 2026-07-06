package loop_default

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func TestDefaultLoopMaxIterations_GoalModeRaisesSmallLimit(t *testing.T) {
	cfg := aicommon.NewConfig(
		context.Background(),
		aicommon.WithMaxIterationCount(4),
		aicommon.WithEnableGoalMode(true),
		aicommon.WithGoalMinIterations(6),
	)
	require.Equal(t, 8, defaultLoopMaxIterations(cfg))
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
