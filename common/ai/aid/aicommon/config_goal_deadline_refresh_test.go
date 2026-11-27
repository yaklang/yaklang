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
	require.True(t, c.RefreshGoalDeadlineForTask())
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

// TestGoalTimePatchSetDurationReopensWindow covers the frontend flow: while the
// window is disabled or never ending, picking "1h" sends only SET_DURATION and
// the backend opens a fresh one-hour window from now.
func TestGoalTimePatchSetDurationReopensWindow(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*Config)
	}{
		{
			name: "from_disabled",
			setup: func(c *Config) {
				require.NoError(t, WithGoalDurationSeconds(0)(c))
			},
		},
		{
			name: "from_never_ending_via_patch",
			setup: func(c *Config) {
				require.NoError(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
					Action: ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_NEVER_ENDING,
				})(c))
			},
		},
		{
			name: "from_never_ending_via_strategy",
			setup: func(c *Config) {
				require.NoError(t, WithGoalDurationSeconds(-1)(c))
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			c := NewTestConfig(context.Background())
			test.setup(c)
			declaredBefore := c.GetGoalDurationSeconds()
			startedAt := time.Now()

			require.NoError(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
				Action:          ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DURATION,
				DurationSeconds: 3600,
			})(c))

			state := c.GetGoalTimeState()
			require.True(t, state.Started)
			require.WithinDuration(t, startedAt.Add(time.Hour), c.GetGoalDeadline(), 2*time.Second)
			require.Equal(t, declaredBefore, c.GetGoalDurationSeconds(),
				"SET_DURATION must only move the runtime window, not declared config")

			// The runtime view is the snapshot: declared config still carries the
			// old value, while the window is a finite deadline again.
			limit := c.BuildAgentPolicySnapshot().Goal.TimeLimit
			require.Equal(t, GoalTimeLimitModeDeadline, limit.Mode)
			require.True(t, limit.Enabled)
			require.Equal(t, c.GetGoalDeadline().Unix(), limit.DeadlineAt)
			require.Equal(t, declaredBefore, limit.ConfiguredDurationSeconds)
		})
	}
}

// TestGoalTimePatchSetDurationAlwaysRestarts keeps the imperative contract: the
// caller asked for "this much time from now", so an unchanged length still
// restarts the window instead of being treated as a repeated declaration.
func TestGoalTimePatchSetDurationAlwaysRestarts(t *testing.T) {
	c := NewTestConfig(context.Background())
	require.NoError(t, WithGoalDurationSeconds(3600)(c))
	c.m.Lock()
	c.GoalDeadline = time.Now().Add(time.Minute)
	c.m.Unlock()

	startedAt := time.Now()
	require.NoError(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action:          ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DURATION,
		DurationSeconds: 3600,
	})(c))
	require.WithinDuration(t, startedAt.Add(time.Hour), c.GetGoalDeadline(), 2*time.Second)
	require.EqualValues(t, 3600, c.GetGoalDurationSeconds())
}

// TestGoalTimePatchSetDurationRejectsNonPositive keeps the action unambiguous:
// disabled and never-ending have dedicated actions.
func TestGoalTimePatchSetDurationRejectsNonPositive(t *testing.T) {
	for _, seconds := range []int64{0, -1, -7200} {
		c := NewTestConfig(context.Background())
		require.NoError(t, WithGoalDurationSeconds(600)(c))
		before := c.GetGoalDeadline()

		require.Error(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
			Action:          ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DURATION,
			DurationSeconds: seconds,
		})(c), "seconds=%d must be rejected", seconds)
		require.Equal(t, before, c.GetGoalDeadline(), "a rejected command must not move the window")
	}
}

// TestGoalTimePatchExtendDoesNotWriteDeclaredDuration pins the whole family:
// none of the runtime commands may rewrite the user's configured length.
func TestGoalTimePatchExtendDoesNotWriteDeclaredDuration(t *testing.T) {
	c := NewTestConfig(context.Background())
	require.NoError(t, WithGoalDurationSeconds(600)(c))

	require.NoError(t, WithGoalTimePatch(&ypb.AIGoalTimePatch{
		Action:        ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_EXTEND,
		ExtendSeconds: 300,
	})(c))
	require.WithinDuration(t, time.Now().Add(15*time.Minute), c.GetGoalDeadline(), 2*time.Second)
	require.EqualValues(t, 600, c.GetGoalDurationSeconds())
}
