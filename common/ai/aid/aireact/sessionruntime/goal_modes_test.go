package sessionruntime

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestStartParamsGoalModesPreserveZeroValues(t *testing.T) {
	for _, strategy := range []*ypb.AIExecutionStrategy{
		{},
		{EnableGoalMode: true, GoalMinIterations: 8},
		{EnableGoalMode: true, GoalDurationSeconds: 10800},
		{EnableGoalMode: true, GoalAcceptanceCriteria: "verify evidence"},
	} {
		ctx, cancel := context.WithCancel(context.Background())
		options := ConvertStartParamsToReActConfig(&ypb.AIStartParams{Strategy: strategy, DisableAISearchForge: true})
		options = append(options, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true), aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir()))
		cfg := aicommon.NewConfig(ctx, options...)
		snapshot := cfg.BuildAgentPolicySnapshot()
		require.Equal(t, strategy.EnableGoalMode, snapshot.Goal.Enabled)
		require.Equal(t, strategy.GoalMinIterations, cfg.GetGoalMinIterations())
		require.Equal(t, strategy.GoalMinIterations, snapshot.Goal.MinIterations)
		require.Equal(t, strategy.GoalAcceptanceCriteria, snapshot.Goal.AcceptanceCriteria)
		require.Equal(t, strategy.GoalDurationSeconds, snapshot.Goal.TimeLimit.ConfiguredDurationSeconds)
		// Starting a session only carries the declared strategy: the time window
		// opens when a task starts, so no deadline exists yet.
		require.Equal(t, aicommon.GoalTimeLimitModeDisabled, snapshot.Goal.TimeLimit.Mode)
		require.False(t, snapshot.Goal.TimeLimit.Enabled)
		require.Zero(t, snapshot.Goal.TimeLimit.DeadlineAt)
		require.True(t, cfg.GetGoalDeadline().IsZero())
		if strategy.GoalDurationSeconds > 0 {
			// The declared length is still carried by the configured value.
			require.EqualValues(t, strategy.GoalDurationSeconds, snapshot.Goal.TimeLimit.ConfiguredDurationSeconds)
		}
		cancel()
	}
}
