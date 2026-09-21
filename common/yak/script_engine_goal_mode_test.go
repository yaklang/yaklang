package yak

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/aiengine"
	"github.com/yaklang/yaklang/common/yak/antlr4yak"
)

func TestScriptEngineAIMGoalModeOptions(t *testing.T) {
	var captured *aiengine.AIEngineConfig
	capture := func(options ...aiengine.AIEngineConfigOption) {
		captured = aiengine.NewAIEngineConfig(options...)
	}

	engine := NewScriptEngine(1)
	engine.RegisterEngineHooks(func(engine *antlr4yak.Engine) error {
		engine.SetVars(map[string]any{"capture": capture})
		return nil
	})
	_, err := engine.ExecuteExWithContext(context.Background(), `
capture(
    aim.enableGoalMode(true),
    aim.goalMinIterations(8),
    aim.goalDurationSeconds(10800),
    aim.goalAcceptanceCriteria("produce an evidence-backed report"),
)
`, map[string]any{})
	require.NoError(t, err)
	require.NotNil(t, captured)
	require.True(t, captured.EnableGoalMode)
	require.Equal(t, int64(8), captured.GoalMinIterations)
	require.Equal(t, int64(10800), captured.GoalDurationSeconds)
	require.Equal(t, "produce an evidence-backed report", captured.GoalAcceptanceCriteria)
}
