package reactloops

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
)

// TestAdvanceEffectiveIteration_NoActiveTodo counts every iteration when there
// are no active TODOs (planning/recon phase — nothing to advance yet).
func TestAdvanceEffectiveIteration_NoActiveTodo(t *testing.T) {
	invoker := mock.NewMockInvoker(context.Background())
	cfg, ok := invoker.GetConfig().(*mock.MockedAIConfig)
	require.True(t, ok)
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("task-no-todo", "planning phase", context.Background(), cfg.GetEmitter(), true)
	loop.SetCurrentTask(task)

	// No active TODOs: every iteration should count as effective.
	for i := 0; i < 5; i++ {
		loop.advanceEffectiveIteration(task, nil)
	}
	require.Equal(t, 5, loop.effectiveIterationCount)
}

// TestAdvanceEffectiveIteration_WithActiveTodo_NoDelta does NOT count stall
// iterations (active TODOs exist but the action carries no todo_delta).
func TestAdvanceEffectiveIteration_WithActiveTodo_NoDelta(t *testing.T) {
	invoker := mock.NewMockInvoker(context.Background())
	cfg, ok := invoker.GetConfig().(*mock.MockedAIConfig)
	require.True(t, ok)
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("task-stall", "stall test", context.Background(), cfg.GetEmitter(), true)
	loop.SetCurrentTask(task)

	// Seed an active TODO via a delta with "add" + "current".
	addAction, err := aicommon.ExtractAction(`{
		"@action":"write_file",
		"todo_delta":{
			"add":[{"id":"todo-1","text":"verify token reuse"}],
			"current":"todo-1"
		}
	}`, "write_file")
	require.NoError(t, err)
	delta := applyTodoDeltaBottomLine(loop, task, 1, addAction)
	require.NotNil(t, delta)
	loop.advanceEffectiveIteration(task, delta)

	// Now there is one active TODO. Subsequent iterations with no delta are stalls.
	require.Equal(t, 1, loop.effectiveIterationCount)

	for i := 0; i < 10; i++ {
		loop.advanceEffectiveIteration(task, nil)
	}
	// Stall iterations must not consume the effective iteration budget.
	require.Equal(t, 1, loop.effectiveIterationCount)
}

// TestAdvanceEffectiveIteration_WithActiveTodo_WithDelta counts iterations that
// carry a todo_delta with real changes even when active TODOs exist.
func TestAdvanceEffectiveIteration_WithActiveTodo_WithDelta(t *testing.T) {
	invoker := mock.NewMockInvoker(context.Background())
	cfg, ok := invoker.GetConfig().(*mock.MockedAIConfig)
	require.True(t, ok)
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("task-progress", "progress test", context.Background(), cfg.GetEmitter(), true)
	loop.SetCurrentTask(task)

	// First iteration: add a TODO.
	addAction, err := aicommon.ExtractAction(`{
		"@action":"write_file",
		"todo_delta":{
			"add":[{"id":"todo-1","text":"step one"}],
			"current":"todo-1"
		}
	}`, "write_file")
	require.NoError(t, err)
	delta1 := applyTodoDeltaBottomLine(loop, task, 1, addAction)
	loop.advanceEffectiveIteration(task, delta1)
	require.Equal(t, 1, loop.effectiveIterationCount)

	// Stall: no delta.
	loop.advanceEffectiveIteration(task, nil)
	require.Equal(t, 1, loop.effectiveIterationCount)

	// Progress: close the TODO.
	closeAction, err := aicommon.ExtractAction(`{
		"@action":"write_file",
		"todo_delta":{
			"close":[{"id":"todo-1","outcome":"resolved","reason":"done"}]
		}
	}`, "write_file")
	require.NoError(t, err)
	delta2 := applyTodoDeltaBottomLine(loop, task, 3, closeAction)
	loop.advanceEffectiveIteration(task, delta2)
	require.Equal(t, 2, loop.effectiveIterationCount)
}
