package aicommon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/yakit"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
	"google.golang.org/protobuf/proto"
)

func TestExecutionStrategyGoalModesReplaceZeros(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan *schema.AiOutputEvent, 16)
	c := NewTestConfig(ctx, WithPersistentSessionId("goal-mode-zero-values"), WithEventHandler(func(e *schema.AiOutputEvent) {
		if e.Type == schema.EVENT_TYPE_AGENT_POLICY_SNAPSHOT || e.Type == schema.EVENT_TYPE_AID_CONFIG {
			events <- e
		}
	}))
	require.NoError(t, c.GetDB().AutoMigrate(&schema.AISession{}).Error)
	readEvent := func() *schema.AiOutputEvent {
		t.Helper()
		select {
		case event := <-events:
			return event
		case <-time.After(time.Second):
			t.Fatal("missing final policy/config event")
			return nil
		}
	}
	for _, step := range []struct {
		name     string
		strategy *ypb.AIExecutionStrategy
	}{
		{"time", &ypb.AIExecutionStrategy{EnableGoalMode: true, GoalDurationSeconds: 10800}},
		{"criteria", &ypb.AIExecutionStrategy{EnableGoalMode: true, GoalAcceptanceCriteria: "deliver evidence"}},
		{"iterations", &ypb.AIExecutionStrategy{EnableGoalMode: true, GoalMinIterations: 12}},
		{"criteria_again", &ypb.AIExecutionStrategy{EnableGoalMode: true, GoalAcceptanceCriteria: "verify tests"}},
		{"time_again", &ypb.AIExecutionStrategy{EnableGoalMode: true, GoalDurationSeconds: 60}},
		{"all_gates_off", &ypb.AIExecutionStrategy{EnableGoalMode: true}},
		{"goal_off", &ypb.AIExecutionStrategy{}},
	} {
		t.Run(step.name, func(t *testing.T) {
			require.NoError(t, c.DispatchHotPatch(&ypb.AIInputEvent{
				IsConfigHotpatch: true, HotpatchType: HotPatchType_ExecutionStrategy,
				Params: &ypb.AIStartParams{Strategy: step.strategy},
			}))
			push := readEvent()
			require.Equal(t, schema.EVENT_TYPE_AGENT_POLICY_SNAPSHOT, push.Type)
			var snapshot AgentPolicySnapshot
			require.NoError(t, json.Unmarshal(push.Content, &snapshot))
			require.Equal(t, step.strategy.EnableGoalMode, snapshot.Goal.Enabled)
			require.Equal(t, step.strategy.GoalMinIterations, snapshot.Goal.MinIterations)
			require.Equal(t, step.strategy.GoalMinIterations, c.GetGoalMinIterations())
			require.Equal(t, step.strategy.GoalAcceptanceCriteria, snapshot.Goal.AcceptanceCriteria)
			require.Equal(t, step.strategy.GoalAcceptanceCriteria, c.GetGoalAcceptanceCriteria())
			require.Equal(t, step.strategy.GoalDurationSeconds, snapshot.Goal.TimeLimit.ConfiguredDurationSeconds)
			if step.strategy.GoalDurationSeconds > 0 {
				require.Equal(t, GoalTimeLimitModeDeadline, snapshot.Goal.TimeLimit.Mode)
				require.Zero(t, snapshot.Goal.TimeLimit.DeadlineAt,
					"a declared duration must not open a window by itself")
				require.True(t, c.GetGoalDeadline().IsZero())
			} else {
				require.Equal(t, GoalTimeLimitModeDisabled, snapshot.Goal.TimeLimit.Mode)
				require.Zero(t, snapshot.Goal.TimeLimit.DeadlineAt)
				require.True(t, c.GetGoalDeadline().IsZero(), "switching away from time mode clears its old deadline")
			}
			if step.strategy.GoalAcceptanceCriteria == "" {
				require.Contains(t, string(push.Content), `"acceptance_criteria":""`, "clients must receive the cleared value")
			}
			configEvent := readEvent()
			require.EqualValues(t, schema.EVENT_TYPE_AID_CONFIG, configEvent.Type)
			var info map[string]any
			require.NoError(t, json.Unmarshal(configEvent.Content, &info))
			require.EqualValues(t, step.strategy.GoalMinIterations, info["GoalMinIterations"])
			require.EqualValues(t, snapshot.Goal.TimeLimit.DeadlineAt, info["GoalDeadlineUnix"])
			require.NoError(t, c.HandleSyncAgentPolicySnapshotEvent(&ypb.AIInputEvent{SyncID: step.name}))
			syncEvent := readEvent()
			require.True(t, syncEvent.IsSync)
			require.Equal(t, step.name, syncEvent.SyncID)
			var synced AgentPolicySnapshot
			require.NoError(t, json.Unmarshal(syncEvent.Content, &synced))
			require.Equal(t, snapshot.Goal, synced.Goal)
			saved, err := yakit.GetAISessionMetaStartParamsBySessionID(c.GetDB(), c.PersistentSessionId)
			require.NoError(t, err)
			require.True(t, proto.Equal(step.strategy, saved.Strategy), "persisted strategy must retain zeros")
			select {
			case e := <-events:
				t.Fatalf("unexpected intermediate or duplicate event: %s", e.Type)
			default:
			}
		})
	}
}

func TestGoalTimeModeSwitchToCriteriaClearsTimeWindow(t *testing.T) {
	c := NewTestConfig(context.Background(), WithEnableGoalMode(true))
	require.NoError(t, c.DispatchHotPatch(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{Strategy: &ypb.AIExecutionStrategy{
			EnableGoalMode:      true,
			GoalDurationSeconds: 10800,
		}},
	}))
	// Declared config alone opens nothing; the window belongs to a task.
	require.True(t, c.GetGoalDeadline().IsZero())
	require.True(t, c.RefreshGoalDeadlineForTask())
	timeSnapshot := c.BuildAgentPolicySnapshot()
	require.Equal(t, GoalTimeLimitModeDeadline, timeSnapshot.Goal.TimeLimit.Mode)
	require.EqualValues(t, 10800, timeSnapshot.Goal.TimeLimit.ConfiguredDurationSeconds)
	require.False(t, c.GetGoalDeadline().IsZero())

	require.NoError(t, c.DispatchHotPatch(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{Strategy: &ypb.AIExecutionStrategy{
			EnableGoalMode:         true,
			GoalAcceptanceCriteria: "deliver evidence",
		}},
	}))
	criteriaSnapshot := c.BuildAgentPolicySnapshot()
	require.Equal(t, "deliver evidence", criteriaSnapshot.Goal.AcceptanceCriteria)
	require.Zero(t, criteriaSnapshot.Goal.TimeLimit.ConfiguredDurationSeconds)
	require.Zero(t, criteriaSnapshot.Goal.TimeLimit.DeadlineAt)
	require.False(t, criteriaSnapshot.Goal.TimeLimit.Enabled)
	require.Equal(t, GoalTimeLimitModeDisabled, criteriaSnapshot.Goal.TimeLimit.Mode)
	require.True(t, c.GetGoalDeadline().IsZero())
	require.Zero(t, c.GetGoalDurationSeconds())
}

// A GoalTimePatch owns the running window; a strategy only replaces the declared
// duration, so the patched deadline survives until the next task start.
func TestExecutionStrategyKeepsPatchUntilNextTaskStart(t *testing.T) {
	c := NewTestConfig(context.Background())
	patchedDeadline := time.Now().Add(10 * time.Minute).Unix()
	require.NoError(t, c.DispatchHotPatch(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_GoalTime,
		GoalTimePatch: &ypb.AIGoalTimePatch{
			Action:       ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DEADLINE,
			DeadlineUnix: patchedDeadline,
		},
	}))
	require.Equal(t, patchedDeadline, c.GetGoalDeadline().Unix())

	before := time.Now()
	require.NoError(t, c.DispatchHotPatch(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{Strategy: &ypb.AIExecutionStrategy{
			EnableGoalMode:      true,
			GoalMinIterations:   1,
			GoalDurationSeconds: 7200,
		}},
	}))
	snapshot := c.BuildAgentPolicySnapshot()
	require.True(t, snapshot.Goal.Enabled)
	require.EqualValues(t, 1, snapshot.Goal.MinIterations)
	require.EqualValues(t, 7200, snapshot.Goal.TimeLimit.ConfiguredDurationSeconds)
	require.Equal(t, GoalTimeLimitModeDeadline, snapshot.Goal.TimeLimit.Mode)
	require.Equal(t, patchedDeadline, snapshot.Goal.TimeLimit.DeadlineAt,
		"a strategy must not move the window that a patch owns")

	require.True(t, c.RefreshGoalDeadlineForTask())
	require.WithinDuration(t, before.Add(2*time.Hour), c.GetGoalDeadline(), time.Second)
	require.NotEqual(t, patchedDeadline, c.GetGoalDeadline().Unix(),
		"the next task start supersedes the patched window")
}

func TestExecutionStrategyAndGoalTimePatchApplyIndependently(t *testing.T) {
	c := NewTestConfig(context.Background(), WithEnableGoalMode(true), WithGoalMinIterations(9), WithGoalAcceptanceCriteria("keep evidence"))
	before := c.BuildAgentPolicySnapshot().Goal
	require.Error(t, c.DispatchHotPatch(&ypb.AIInputEvent{
		IsConfigHotpatch: true, HotpatchType: HotPatchType_ExecutionStrategy,
		Params:        &ypb.AIStartParams{Strategy: &ypb.AIExecutionStrategy{EnableGoalMode: true, GoalDurationSeconds: 10800}},
		GoalTimePatch: &ypb.AIGoalTimePatch{Action: ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DEADLINE},
	}))
	require.True(t, c.GetEnableGoalMode())
	require.Equal(t, int64(10800), c.GetGoalDurationSeconds())
	require.True(t, c.GetGoalDeadline().IsZero(),
		"a valid strategy must still apply when the following patch fails, without opening a window")

	before = c.BuildAgentPolicySnapshot().Goal
	require.Error(t, c.DispatchHotPatch(&ypb.AIInputEvent{
		IsConfigHotpatch: true, HotpatchType: HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{Strategy: &ypb.AIExecutionStrategy{GoalMinIterations: -1}},
	}))
	require.Equal(t, before, c.BuildAgentPolicySnapshot().Goal, "invalid strategy must be rejected before any option runs")
}
