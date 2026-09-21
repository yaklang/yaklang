package aiengine

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
)

func TestGoalModeEngineOptionsExportAndPropagation(t *testing.T) {
	enableGoalMode, ok := Exports["enableGoalMode"].(func(bool) AIEngineConfigOption)
	require.True(t, ok)
	goalMinIterations, ok := Exports["goalMinIterations"].(func(int64) AIEngineConfigOption)
	require.True(t, ok)
	goalDurationSeconds, ok := Exports["goalDurationSeconds"].(func(int64) AIEngineConfigOption)
	require.True(t, ok)
	goalAcceptanceCriteria, ok := Exports["goalAcceptanceCriteria"].(func(string) AIEngineConfigOption)
	require.True(t, ok)

	config := NewAIEngineConfig(
		enableGoalMode(true),
		goalMinIterations(8),
		goalDurationSeconds(3600),
		goalAcceptanceCriteria("produce three evidence-backed findings"),
	)

	require.True(t, config.EnableGoalMode)
	require.Equal(t, int64(8), config.GoalMinIterations)
	require.Equal(t, int64(3600), config.GoalDurationSeconds)
	require.Equal(t, "produce three evidence-backed findings", config.GoalAcceptanceCriteria)

	options := buildReActOptions(context.Background(), config, make(chan *schema.AiOutputEvent, 1))
	options = append(options, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true))
	reactConfig := aicommon.NewConfig(context.Background(), options...)
	require.True(t, reactConfig.GetEnableGoalMode())
	require.Equal(t, int64(8), reactConfig.GetGoalMinIterations())
	require.Equal(t, int64(3600), reactConfig.GetGoalDurationSeconds())
	require.Equal(t, "produce three evidence-backed findings", reactConfig.GetGoalAcceptanceCriteria())

	params := config.ConvertToYPBAIStartParams()
	require.NotNil(t, params.GetStrategy())
	require.True(t, params.GetStrategy().GetEnableGoalMode())
	require.Equal(t, int64(8), params.GetStrategy().GetGoalMinIterations())
	require.Equal(t, int64(3600), params.GetStrategy().GetGoalDurationSeconds())
	require.Equal(t, "produce three evidence-backed findings", params.GetStrategy().GetGoalAcceptanceCriteria())
}

func TestGoalModeEngineOptionNeverEnding(t *testing.T) {
	config := NewAIEngineConfig(
		WithEnableGoalMode(true),
		WithGoalDurationSeconds(-1),
	)

	options := buildReActOptions(context.Background(), config, make(chan *schema.AiOutputEvent, 1))
	options = append(options, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true))
	reactConfig := aicommon.NewConfig(context.Background(), options...)
	require.Equal(t, int64(-1), reactConfig.GetGoalDurationSeconds())
	require.Equal(t, int64(-1), config.ConvertToYPBAIStartParams().GetStrategy().GetGoalDurationSeconds())
}

func TestGoalModeEngineOptionsDisabledByDefault(t *testing.T) {
	config := NewAIEngineConfig()
	require.False(t, config.EnableGoalMode)
	require.Nil(t, config.ConvertToYPBAIStartParams().GetStrategy())
}
