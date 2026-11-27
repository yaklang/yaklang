package aireact

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
)

func newGoalTimeRefreshTask(t *testing.T, id string) *aicommon.AIStatefulTaskBase {
	t.Helper()
	return aicommon.NewStatefulTaskBase(id, "goal window test", context.Background(), nil, true)
}

// TestRefreshGoalTimeForTaskStartsWindowAtTaskStart locks the intended
// production contract: the goal deadline is computed when a top-level task
// begins executing, not when the strategy was configured much earlier.
func TestRefreshGoalTimeForTaskStartsWindowAtTaskStart(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := aicommon.NewConfig(ctx, aicommon.WithGoalDurationSeconds(3600))
	r := &ReAct{config: cfg}

	// Simulate a window that was configured long before any task ran.
	cfg.GoalDeadline = time.Now().Add(-time.Hour)

	task := newGoalTimeRefreshTask(t, "task-window-start")
	r.refreshGoalTimeForTask(task)

	require.WithinDuration(t, time.Now().Add(time.Hour), cfg.GetGoalDeadline(), 2*time.Second)
	require.False(t, cfg.IsGoalDeadlinePassed())
}

// TestRefreshGoalTimeForTaskSkipsNonGoalTasks ensures sub-agents and recovery
// tasks cannot create or move the mainline window.
func TestRefreshGoalTimeForTaskSkipsNonGoalTasks(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := aicommon.NewConfig(ctx, aicommon.WithGoalDurationSeconds(3600))
	r := &ReAct{config: cfg}
	stale := time.Now().Add(-time.Hour)
	cfg.GoalDeadline = stale

	subAgentTask := newGoalTimeRefreshTask(t, "task-sub-agent")
	subAgentTask.SetSubAgent(true)
	r.refreshGoalTimeForTask(subAgentTask)
	require.Equal(t, stale, cfg.GetGoalDeadline(), "a sub-agent must not restart the mainline window")

	recoveryTask := newGoalTimeRefreshTask(t, "task-recovery")
	recoveryTask.SetTaskKind(aicommon.AITaskKind_Recovery)
	r.refreshGoalTimeForTask(recoveryTask)
	require.Equal(t, stale, cfg.GetGoalDeadline(), "a recovery task must not restart the mainline window")
}

// TestRefreshGoalTimeForTaskRestartsWindowForEachNewTask verifies consecutive
// tasks each get their own window instead of sharing the first task's deadline.
func TestRefreshGoalTimeForTaskRestartsWindowForEachNewTask(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := aicommon.NewConfig(ctx, aicommon.WithGoalDurationSeconds(600))
	r := &ReAct{config: cfg}

	firstTask := newGoalTimeRefreshTask(t, "task-first")
	r.refreshGoalTimeForTask(firstTask)
	firstDeadline := cfg.GetGoalDeadline()
	require.WithinDuration(t, time.Now().Add(10*time.Minute), firstDeadline, 2*time.Second)

	// The first task ends and the second one starts after the window elapsed.
	cfg.GoalDeadline = time.Now().Add(-time.Minute)

	secondTask := newGoalTimeRefreshTask(t, "task-second")
	r.refreshGoalTimeForTask(secondTask)
	require.WithinDuration(t, time.Now().Add(10*time.Minute), cfg.GetGoalDeadline(), 2*time.Second)
	require.True(t, cfg.GetGoalDeadline().After(firstDeadline.Add(-time.Second)))
}
