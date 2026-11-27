package aicommon

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// TestRefreshGoalDeadlineRestartsWindowForNewTask locks the task-scoped
// semantics of the goal time window: the declared duration lives in Config, but
// each new task starts its own window instead of inheriting the previous one.
func TestRefreshGoalDeadlineRestartsWindowForNewTask(t *testing.T) {
	c := NewTestConfig(context.Background())
	require.NoError(t, WithGoalDurationSeconds(3600)(c))

	firstDeadline := time.Now().Add(-time.Minute)
	c.GoalDeadline = firstDeadline // simulate a window that already elapsed

	require.True(t, c.RefreshGoalDeadlineForTask())
	refreshed := c.GetGoalDeadline()
	require.WithinDuration(t, time.Now().Add(time.Hour), refreshed, 2*time.Second)
	require.False(t, refreshed.Equal(firstDeadline), "a new task must not inherit the previous window")

	// A second task restarts the window again, even before the first one ends.
	require.True(t, c.RefreshGoalDeadlineForTask())
	require.WithinDuration(t, time.Now().Add(time.Hour), c.GetGoalDeadline(), 2*time.Second)
	require.EqualValues(t, 3600, c.GetGoalDurationSeconds())
}

// TestRefreshGoalDeadlineSupersedesPatch keeps the rule simple: a new task
// always starts a fresh window, so a GoalTimePatch applied to the previous task
// never leaks into the next one.
func TestRefreshGoalDeadlineSupersedesPatch(t *testing.T) {
	c := NewTestConfig(context.Background())
	require.NoError(t, WithGoalDurationSeconds(3600)(c))

	patched := time.Now().Add(3 * time.Hour).Truncate(time.Second)
	require.NoError(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action:       ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DEADLINE,
		DeadlineUnix: patched.Unix(),
	})(c))
	require.Equal(t, patched, c.GetGoalDeadline())

	require.True(t, c.RefreshGoalDeadlineForTask())
	require.WithinDuration(t, time.Now().Add(time.Hour), c.GetGoalDeadline(), 2*time.Second)
	require.EqualValues(t, 3600, c.GetGoalDurationSeconds(), "a new task must not rewrite the declared duration")
}

// TestRefreshGoalDeadlineClearsDisabledWindow makes sure a disabled time window
// does not leave the previous task's deadline visible.
func TestRefreshGoalDeadlineClearsDisabledWindow(t *testing.T) {
	c := NewTestConfig(context.Background())
	require.NoError(t, WithGoalDurationSeconds(600)(c))
	require.False(t, c.GetGoalDeadline().IsZero())

	require.NoError(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action: ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_DISABLE,
	})(c))
	require.True(t, c.GetGoalDeadline().IsZero())

	// Simulate a stale deadline from an older window plus a disabled duration.
	c.GoalDeadline = time.Now().Add(time.Minute)
	c.GoalDurationSeconds = 0

	require.True(t, c.RefreshGoalDeadlineForTask())
	require.True(t, c.GetGoalDeadline().IsZero())
	require.False(t, c.GetGoalTimeState().Started)

	// Nothing left to clear: the refresh is idempotent.
	require.False(t, c.RefreshGoalDeadlineForTask())
}

// TestRefreshGoalDeadlineForNeverEndingKeepsSentinel verifies the -1 sentinel is
// re-applied per task rather than inheriting a finite deadline.
func TestRefreshGoalDeadlineForNeverEndingKeepsSentinel(t *testing.T) {
	c := NewTestConfig(context.Background())
	require.NoError(t, WithGoalDurationSeconds(-1)(c))

	c.GoalDeadline = time.Now().Add(-time.Hour) // stale finite window

	require.True(t, c.RefreshGoalDeadlineForTask())
	require.False(t, c.IsGoalDeadlinePassed())
	require.True(t, c.GetGoalTimeState().NeverEnding)
	require.False(t, c.GetGoalDeadline().IsZero())
	require.GreaterOrEqual(t, c.GetGoalDeadline().UTC().Year(), 9999)
}

// TestExecutionStrategyDurationChangeRebuildsWindow ensures a new declared
// duration is materialized and then owned by the next task's window.
func TestExecutionStrategyDurationChangeRebuildsWindow(t *testing.T) {
	c := NewTestConfig(context.Background())
	require.NoError(t, WithGoalDurationSeconds(3600)(c))
	require.NoError(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action:        ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_EXTEND,
		ExtendSeconds: 600,
	})(c))
	extended := c.GetGoalDeadline()
	require.WithinDuration(t, time.Now().Add(70*time.Minute), extended, 2*time.Second)

	require.NoError(t, WithExecutionStrategy(&ypb.AIExecutionStrategy{
		EnableGoalMode:      true,
		GoalDurationSeconds: 7200,
	})(c))

	require.True(t, c.RefreshGoalDeadlineForTask())
	require.WithinDuration(t, time.Now().Add(2*time.Hour), c.GetGoalDeadline(), 2*time.Second)
}

// TestRepeatedStrategyValueKeepsCurrentDeadline documents the idempotent
// duration rule: resending the same value must not restart the window by itself.
func TestRepeatedStrategyValueKeepsCurrentDeadline(t *testing.T) {
	c := NewTestConfig(context.Background())
	require.NoError(t, WithGoalDurationSeconds(3600)(c))
	patched := time.Now().Add(20 * time.Minute).Truncate(time.Second)
	require.NoError(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action:       ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DEADLINE,
		DeadlineUnix: patched.Unix(),
	})(c))

	require.NoError(t, WithExecutionStrategy(&ypb.AIExecutionStrategy{
		EnableGoalMode:      true,
		GoalDurationSeconds: 3600, // unchanged value
	})(c))

	require.Equal(t, patched, c.GetGoalDeadline(), "an unchanged duration must not restart the window")
}
