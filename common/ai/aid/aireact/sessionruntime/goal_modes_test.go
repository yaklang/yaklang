package sessionruntime

import (
	"context"
	"testing"
	"time"

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
		if strategy.GoalDurationSeconds > 0 {
			require.Equal(t, aicommon.GoalTimeLimitModeDeadline, snapshot.Goal.TimeLimit.Mode)
			require.WithinDuration(t, time.Now().Add(3*time.Hour), cfg.GetGoalDeadline(), time.Second)
		} else {
			require.Zero(t, snapshot.Goal.TimeLimit.DeadlineAt)
		}
		cancel()
	}
}
