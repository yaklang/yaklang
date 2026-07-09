package reactloops

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/utils/omap"
)

// registerFinishAndAnswer installs the finish + directly_answer actions on a
// minimal loop so generateSchemaString has real actions to filter.
func registerFinishAndAnswer(loop *ReActLoop) {
	if loop.actions == nil {
		loop.actions = omap.NewEmptyOrderedMap[string, *LoopAction]()
	}
	loop.actions.Set(loopAction_Finish.ActionType, loopAction_Finish)
	loop.actions.Set(loopAction_DirectlyAnswer.ActionType, loopAction_DirectlyAnswer)
}

// TestGenerateSchemaString_GoalModeRemovesFinish verifies the schema-level gate:
// when disallowExit is true the finish action must be absent from the generated
// schema, and present when false. This is the first line of defense for goal
// mode (the runtime handler block is the second).
func TestGenerateSchemaString_GoalModeRemovesFinish(t *testing.T) {
	loop, _, _, _ := newTodoGateTestLoop(t, nil)
	registerFinishAndAnswer(loop)

	// "finish: Mark the current task" is the unique description prefix of the
	// finish action entry; the word "finish" alone also appears in
	// directly_answer's description, so assert on the entry, not the bare word.
	const finishEntry = "finish: Mark the current task"

	off, err := loop.generateSchemaString(false)
	require.NoError(t, err)
	require.True(t, strings.Contains(off, finishEntry),
		"finish action must appear in schema when exit is allowed")

	on, err := loop.generateSchemaString(true)
	require.NoError(t, err)
	require.False(t, strings.Contains(on, finishEntry),
		"finish action must be removed from schema when exit is disallowed (goal mode gate)")
	// directly_answer must survive the gate; only finish is gated.
	require.True(t, strings.Contains(on, "directly_answer"))
}

// TestApplyGoalModeGate_Wiring locks the single application point: the gate must
// set disallowLoopExit for iterations below GoalMinIterations and leave it clear
// at/above the threshold. This guards against regressions if the operator
// lifecycle in exec.go is restructured.
func TestApplyGoalModeGate_Wiring(t *testing.T) {
	loop, _, cfg, task := newTodoGateTestLoop(t, nil)
	registerFinishAndAnswer(loop)
	cfg.enableGoalMode = true
	cfg.goalMinIterations = 6

	cases := []struct {
		iter     int
		disallow bool
	}{
		{1, true},
		{3, true},
		{5, true},
		{6, false}, // finish allowed at >= GoalMinIterations
		{7, false},
	}
	for _, c := range cases {
		op := NewActionHandlerOperator(task)
		loop.ApplyGoalModeGate(op, c.iter)
		require.Equalf(t, c.disallow, op.GetDisallowLoopExit(),
			"iteration %d: expected disallow=%v", c.iter, c.disallow)
	}
}

// TestApplyGoalModeGate_NoopWhenGoalModeDisabled ensures the gate is inert when
// goal mode is off, so non-goal sessions keep finish available.
func TestApplyGoalModeGate_NoopWhenGoalModeDisabled(t *testing.T) {
	loop, _, cfg, task := newTodoGateTestLoop(t, nil)
	registerFinishAndAnswer(loop)
	cfg.enableGoalMode = false
	cfg.goalMinIterations = 6

	op := NewActionHandlerOperator(task)
	loop.ApplyGoalModeGate(op, 1)
	require.False(t, op.GetDisallowLoopExit())
}

// TestApplyGoalModeGate_NilSafe keeps the nil-guard contract explicit.
func TestApplyGoalModeGate_NilSafe(t *testing.T) {
	var loop *ReActLoop
	loop.ApplyGoalModeGate(nil, 1) // must not panic

	loop, _, _, _ = newTodoGateTestLoop(t, nil)
	registerFinishAndAnswer(loop)
	loop.ApplyGoalModeGate(nil, 1) // nil operator must not panic
}

// TestIsGoalModeEnabled_SubAgentForcedFalse verifies the defense-in-depth guard:
// even if a sub-agent config somehow had EnableGoalMode set, the loop-level
// IsSubAgent check forces goal mode off so the finish gate never applies to a
// forked sub agent.
func TestIsGoalModeEnabled_SubAgentForcedFalse(t *testing.T) {
	loop, _, cfg, _ := newTodoGateTestLoop(t, nil)
	registerFinishAndAnswer(loop)
	cfg.enableGoalMode = true
	cfg.goalMinIterations = 6

	// Top-level: goal mode is active and blocks finish early.
	require.True(t, loop.IsGoalModeEnabled())
	require.True(t, loop.ShouldBlockFinishAtIteration(3))

	// Mark the loop as a forked sub agent (depth 1, as buildSubReactLoopOptions
	// does via WithVar(SubAgentDepthLoopVar, 1)).
	loop.Set(SubAgentDepthLoopVar, 1)
	require.True(t, loop.IsSubAgent())
	require.False(t, loop.IsGoalModeEnabled(),
		"sub agent must not be subject to goal mode even when config has it enabled")
	require.False(t, loop.ShouldBlockFinishAtIteration(3))
	require.False(t, loop.ShouldBlockFinishAtIteration(1),
		"sub agent must be free to finish as soon as its goal is done")
}
