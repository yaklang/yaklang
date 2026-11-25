package aireact

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSendInputEventAndWaitAcceptedDispatchesHotpatchBeforeReturning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hotpatchChan := chanx.NewUnlimitedChan[aicommon.ConfigOption](ctx, 1)
	cfg := aicommon.NewConfig(ctx,
		aicommon.WithHotPatchOptionChan(hotpatchChan),
	)
	react := &ReAct{config: cfg}

	err := react.SendInputEventAndWaitAccepted(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     aicommon.HotPatchType_RuntimeContextSizeLimit,
		Params:           &ypb.AIStartParams{RuntimeContextSizeLimit: 32768},
	})
	require.NoError(t, err)

	select {
	case option := <-hotpatchChan.OutputChannel():
		require.NotNil(t, option)
		require.NoError(t, option(cfg))
		require.EqualValues(t, 32768, cfg.GetTimelineContentSizeLimit())
	case <-time.After(time.Second):
		t.Fatal("hot-patch was not dispatched before SendInputEventAndWaitAccepted returned")
	}
}

func TestSendInputEventAndWaitAcceptedAppliesAgentPolicySynchronously(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg := aicommon.NewConfig(ctx)
	react := &ReAct{config: cfg}
	deadline := time.Now().Add(time.Hour).Unix()

	err := react.SendInputEventAndWaitAccepted(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     aicommon.HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableGoalMode:      true,
				GoalMinIterations:   1,
				GoalDurationSeconds: 3600,
			},
		},
		GoalTimePatch: &ypb.AIGoalTimePatch{
			Action:       ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DEADLINE,
			DeadlineUnix: deadline,
		},
	})
	require.NoError(t, err)
	require.True(t, cfg.GetEnableGoalMode())
	require.Equal(t, deadline, cfg.GetGoalDeadline().Unix())
	require.Equal(t, aicommon.GoalTimeLimitModeDeadline, cfg.BuildAgentPolicySnapshot().Goal.TimeLimit.Mode)
}

func TestReActStrategyHotpatchWithoutGoalTimePatchReflectsDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events := make(chan *schema.AiOutputEvent, 16)
	in := make(chan *ypb.AIInputEvent, 8)
	runtime, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			events <- event
		}),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithWorkdir(t.TempDir()),
	)
	require.NoError(t, err)
	t.Cleanup(runtime.WaitLifecycleStopped)

	in <- &ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     aicommon.HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableGoalMode:      true,
				GoalMinIterations:   1,
				GoalDurationSeconds: 7200,
			},
		},
	}
	pushed := waitForAgentPolicySnapshot(t, events, "")
	require.True(t, pushed.Goal.Enabled)
	require.EqualValues(t, 1, pushed.Goal.MinIterations)
	require.Equal(t, aicommon.GoalTimeLimitModeDeadline, pushed.Goal.TimeLimit.Mode)
	require.EqualValues(t, 7200, pushed.Goal.TimeLimit.ConfiguredDurationSeconds)
	require.InDelta(t, 7200, pushed.Goal.TimeLimit.DeadlineAt-pushed.ServerTime, 1)

	const syncID = "strategy-without-time-patch"
	require.NoError(t, runtime.handleSyncMessage(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aicommon.SYNC_TYPE_AGENT_POLICY_SNAPSHOT,
		SyncID:        syncID,
	}))
	synced := waitForAgentPolicySnapshot(t, events, syncID)
	require.Equal(t, pushed.Goal, synced.Goal)
}

func TestReActGoalTimeModeSwitchToCriteriaClearsTimeWindow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events := make(chan *schema.AiOutputEvent, 16)
	in := make(chan *ypb.AIInputEvent, 8)
	runtime, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			events <- event
		}),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithWorkdir(t.TempDir()),
	)
	require.NoError(t, err)
	t.Cleanup(runtime.WaitLifecycleStopped)

	in <- &ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     aicommon.HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableGoalMode:      true,
				GoalDurationSeconds: 10800,
			},
		},
	}
	timeSnapshot := waitForAgentPolicySnapshot(t, events, "")
	require.Equal(t, aicommon.GoalTimeLimitModeDeadline, timeSnapshot.Goal.TimeLimit.Mode)
	require.False(t, timeSnapshot.Goal.TimeLimit.DeadlineAt == 0)

	in <- &ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     aicommon.HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableGoalMode:         true,
				GoalAcceptanceCriteria: "deliver evidence",
			},
		},
	}
	criteriaSnapshot := waitForAgentPolicySnapshot(t, events, "")
	require.Equal(t, "deliver evidence", criteriaSnapshot.Goal.AcceptanceCriteria)
	require.Zero(t, criteriaSnapshot.Goal.TimeLimit.ConfiguredDurationSeconds)
	require.Zero(t, criteriaSnapshot.Goal.TimeLimit.DeadlineAt)
	require.False(t, criteriaSnapshot.Goal.TimeLimit.Enabled)
	require.Equal(t, aicommon.GoalTimeLimitModeDisabled, criteriaSnapshot.Goal.TimeLimit.Mode)

	const syncID = "time-mode-to-criteria"
	require.NoError(t, runtime.handleSyncMessage(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aicommon.SYNC_TYPE_AGENT_POLICY_SNAPSHOT,
		SyncID:        syncID,
	}))
	synced := waitForAgentPolicySnapshot(t, events, syncID)
	require.Equal(t, criteriaSnapshot.Goal, synced.Goal)
}

func TestReActStrategyHotpatchOverridesGoalTimePatchDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events := make(chan *schema.AiOutputEvent, 16)
	in := make(chan *ypb.AIInputEvent, 8)
	runtime, err := NewTestReAct(
		aicommon.WithContext(ctx),
		aicommon.WithEventInputChan(in),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			events <- event
		}),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithWorkdir(t.TempDir()),
	)
	require.NoError(t, err)
	t.Cleanup(runtime.WaitLifecycleStopped)

	patchedDeadline := time.Now().Add(10 * time.Minute).Unix()
	in <- &ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     aicommon.HotPatchType_GoalTime,
		GoalTimePatch: &ypb.AIGoalTimePatch{
			Action:       ypb.AIGoalTimeAction_AI_GOAL_TIME_ACTION_SET_DEADLINE,
			DeadlineUnix: patchedDeadline,
		},
	}
	patched := waitForAgentPolicySnapshot(t, events, "")
	require.Equal(t, patchedDeadline, patched.Goal.TimeLimit.DeadlineAt)

	beforeOverride := time.Now()
	in <- &ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     aicommon.HotPatchType_ExecutionStrategy,
		Params: &ypb.AIStartParams{
			Strategy: &ypb.AIExecutionStrategy{
				EnableGoalMode:      true,
				GoalMinIterations:   1,
				GoalDurationSeconds: 7200,
			},
		},
	}
	overridden := waitForAgentPolicySnapshot(t, events, "")
	require.NotEqual(t, patchedDeadline, overridden.Goal.TimeLimit.DeadlineAt)
	require.Equal(t, aicommon.GoalTimeLimitModeDeadline, overridden.Goal.TimeLimit.Mode)
	require.EqualValues(t, 7200, overridden.Goal.TimeLimit.ConfiguredDurationSeconds)
	require.InDelta(t, beforeOverride.Add(2*time.Hour).Unix(), overridden.Goal.TimeLimit.DeadlineAt, 1)

	const syncID = "strategy-overrides-time-patch"
	require.NoError(t, runtime.handleSyncMessage(&ypb.AIInputEvent{
		IsSyncMessage: true,
		SyncType:      aicommon.SYNC_TYPE_AGENT_POLICY_SNAPSHOT,
		SyncID:        syncID,
	}))
	synced := waitForAgentPolicySnapshot(t, events, syncID)
	require.Equal(t, overridden.Goal, synced.Goal)
}

func waitForAgentPolicySnapshot(t *testing.T, events <-chan *schema.AiOutputEvent, syncID string) aicommon.AgentPolicySnapshot {
	t.Helper()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case event := <-events:
			if event == nil || event.Type != schema.EVENT_TYPE_AGENT_POLICY_SNAPSHOT || event.NodeId != aicommon.AgentPolicySnapshotNodeID {
				continue
			}
			if syncID == "" {
				if event.IsSync {
					continue
				}
			} else if !event.IsSync || event.SyncID != syncID {
				continue
			}
			var snapshot aicommon.AgentPolicySnapshot
			require.NoError(t, json.Unmarshal(event.Content, &snapshot))
			return snapshot
		case <-timeout.C:
			t.Fatal("timed out waiting for agent policy snapshot")
			return aicommon.AgentPolicySnapshot{}
		}
	}
}

func TestWaitTaskByUserInputUUIDStoppedWaitsForRuntimeCleanup(t *testing.T) {
	task := aicommon.NewStatefulTaskBase("task", "input", context.Background(), nil)
	task.SetUserInputUUID("scheduled-input")
	task.SetStatus(aicommon.AITaskState_Completed)
	react := &ReAct{RuntimeTasks: []aicommon.AIStatefulTask{task}, taskQueue: NewTaskQueue("test")}

	waited := make(chan error, 1)
	go func() {
		waited <- react.WaitTaskByUserInputUUIDStopped(context.Background(), "scheduled-input")
	}()
	select {
	case err := <-waited:
		t.Fatalf("task wait returned before runtime cleanup: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	react.updateRuntimeTasks()
	select {
	case err := <-waited:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("task wait did not finish after runtime cleanup")
	}
	require.Empty(t, react.GetRuntimeTasks(), "skipped/completed/aborted tasks must all be pruned")
}

func TestWaitTaskByUserInputUUIDStoppedWaitsForQueueRuntimeHandoff(t *testing.T) {
	react := &ReAct{taskQueue: NewTaskQueue("test")}
	finishHandoff := react.beginTaskQueueHandoff()

	waited := make(chan error, 1)
	go func() {
		waited <- react.WaitTaskByUserInputUUIDStopped(context.Background(), "scheduled-input")
	}()
	select {
	case err := <-waited:
		t.Fatalf("task wait returned during queue/runtime handoff: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	finishHandoff()
	select {
	case err := <-waited:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("task wait did not finish after queue/runtime handoff")
	}
}

func TestCancelTaskByUserInputUUIDAndWaitRetriesAcrossHandoff(t *testing.T) {
	task := aicommon.NewStatefulTaskBase("task", "input", context.Background(), aicommon.NewDummyEmitter())
	task.SetUserInputUUID("scheduled-input")
	task.SetStatus(aicommon.AITaskState_Processing)
	react := &ReAct{Emitter: aicommon.NewDummyEmitter(), taskQueue: NewTaskQueue("test")}
	finishHandoff := react.beginTaskQueueHandoff()

	cancelled := make(chan error, 1)
	go func() {
		cancelled <- react.CancelTaskByUserInputUUIDAndWait(context.Background(), "scheduled-input")
	}()
	select {
	case err := <-cancelled:
		t.Fatalf("cancel returned while the task was hidden by handoff: %v", err)
	case <-time.After(50 * time.Millisecond):
	}

	react.UpdateRuntimeTaskMutex.Lock()
	react.RuntimeTasks = append(react.RuntimeTasks, task)
	react.UpdateRuntimeTaskMutex.Unlock()
	finishHandoff()
	require.Eventually(t, task.IsFinished, time.Second, 10*time.Millisecond,
		"cancellation was not retried after runtime publication")
	react.updateRuntimeTasks()
	select {
	case err := <-cancelled:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("cancel did not finish after task cleanup")
	}
}
