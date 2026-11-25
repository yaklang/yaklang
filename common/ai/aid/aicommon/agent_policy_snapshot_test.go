package aicommon

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestBuildAgentPolicySnapshot_ReportsModesAndGoalDeadline(t *testing.T) {
	cfg := NewConfig(context.Background(),
		WithDisableAutoSkills(true),
		WithEnableMultiAgentMode(true),
		WithMaxSubAgents(7),
		WithEnableGoalMode(true),
		WithGoalMinIterations(9),
		WithGoalDurationSeconds(1800),
		WithGoalAcceptanceCriteria("must include evidence"),
		WithEnablePlanAndExec(true),
		WithEnableDetachedPlan(true),
		WithPlanExecTaskConcurrency(3),
		WithAllowPlanUserInteract(true),
		WithPlanUserInteractMaxCount(4),
		WithEnableSubagentsInPlan(true),
	)
	cfg.StartGoalDeadline()
	deadline := cfg.GetGoalDeadline()

	snapshot := cfg.BuildAgentPolicySnapshot()
	require.NotNil(t, snapshot)
	require.Equal(t, AgentPolicySnapshotSchemaVersion, snapshot.SchemaVersion)
	require.Equal(t, uint64(1), snapshot.Revision)
	require.WithinDuration(t, time.Now(), time.Unix(snapshot.ServerTime, 0), 2*time.Second)
	require.True(t, snapshot.Goal.Enabled)
	require.True(t, snapshot.Goal.Effective)
	require.EqualValues(t, 9, snapshot.Goal.MinIterations)
	require.Equal(t, "must include evidence", snapshot.Goal.AcceptanceCriteria)
	require.Equal(t, GoalTimeLimitModeDeadline, snapshot.Goal.TimeLimit.Mode)
	require.Equal(t, deadline.Unix(), snapshot.Goal.TimeLimit.DeadlineAt)
	require.EqualValues(t, 1800, snapshot.Goal.TimeLimit.ConfiguredDurationSeconds)
	require.True(t, snapshot.MultiAgent.Enabled)
	require.True(t, snapshot.MultiAgent.PreferDispatch)
	require.EqualValues(t, 7, snapshot.MultiAgent.MaxConcurrency)
	require.True(t, snapshot.Plan.Enabled)
	require.True(t, snapshot.Plan.Detached)
	require.Equal(t, 3, snapshot.Plan.TaskConcurrency)
	require.True(t, snapshot.Plan.AllowUserInteract)
	require.EqualValues(t, 4, snapshot.Plan.MaxUserInteractions)
	require.True(t, snapshot.Plan.AllowSubagents)

	raw, err := json.Marshal(snapshot)
	require.NoError(t, err)
	require.Contains(t, string(raw), `"deadline_at":`)
	require.NotContains(t, string(raw), "remaining_seconds")
}

func TestEmitAgentPolicySnapshot_UsesDedicatedStructuredEvent(t *testing.T) {
	events := make(chan *schema.AiOutputEvent, 4)
	cfg := NewTestConfig(context.Background(), WithEventHandler(func(event *schema.AiOutputEvent) {
		events <- event
	}), WithEnableGoalMode(true), WithGoalDurationSeconds(60))

	cfg.EmitAgentPolicySnapshot()
	select {
	case event := <-events:
		require.Equal(t, schema.EVENT_TYPE_AGENT_POLICY_SNAPSHOT, event.Type)
		require.Equal(t, AgentPolicySnapshotNodeID, event.NodeId)
		require.True(t, event.IsSystem)
		require.True(t, event.IsJson)
		require.False(t, event.ShouldSave(), "runtime policy snapshots are replaceable state and must not be persisted")
		var snapshot AgentPolicySnapshot
		require.NoError(t, json.Unmarshal(event.Content, &snapshot))
		require.Equal(t, GoalTimeLimitModeDeadline, snapshot.Goal.TimeLimit.Mode)
		require.InDelta(t, 60, snapshot.Goal.TimeLimit.DeadlineAt-snapshot.ServerTime, 1)
	case <-time.After(time.Second):
		t.Fatal("agent policy snapshot event was not emitted")
	}
}

func TestHandleSyncAgentPolicySnapshotEvent(t *testing.T) {
	events := make(chan *schema.AiOutputEvent, 4)
	cfg := NewTestConfig(context.Background(), WithEventHandler(func(event *schema.AiOutputEvent) {
		events <- event
	}))

	require.NoError(t, cfg.HandleSyncAgentPolicySnapshotEvent(&ypb.AIInputEvent{SyncID: "policy-sync"}))
	select {
	case event := <-events:
		require.True(t, event.IsSync)
		require.Equal(t, "policy-sync", event.SyncID)
		require.Equal(t, schema.EVENT_TYPE_AGENT_POLICY_SNAPSHOT, event.Type)
		require.Equal(t, AgentPolicySnapshotNodeID, event.NodeId)
	case <-time.After(time.Second):
		t.Fatal("agent policy snapshot sync response was not emitted")
	}
}

func TestSettingGoalDurationNotifiesAgentPolicySnapshot(t *testing.T) {
	events := make(chan *schema.AiOutputEvent, 4)
	cfg := NewTestConfig(context.Background(),
		WithEventHandler(func(event *schema.AiOutputEvent) {
			events <- event
		}),
		WithEnableGoalMode(true),
	)
	cfg.SetAgentPolicySnapshotEmitHandler(func() {
		cfg.EmitAgentPolicySnapshot()
	})
	require.NoError(t, WithGoalDurationSeconds(60)(cfg))

	select {
	case event := <-events:
		var snapshot AgentPolicySnapshot
		require.NoError(t, json.Unmarshal(event.Content, &snapshot))
		require.Equal(t, GoalTimeLimitModeDeadline, snapshot.Goal.TimeLimit.Mode)
		require.Greater(t, snapshot.Goal.TimeLimit.DeadlineAt, snapshot.ServerTime)
	case <-time.After(time.Second):
		t.Fatal("starting the goal deadline did not emit a policy snapshot")
	}
}
