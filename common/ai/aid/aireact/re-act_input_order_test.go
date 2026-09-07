package aireact

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/utils/chanx"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestSendInputEventAndWaitAcceptedDispatchesHotpatchBeforeReturning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hotpatchChan := chanx.NewUnlimitedChan[aicommon.ConfigOption](ctx, 1)
	cfg := aicommon.NewConfig(ctx,
		aicommon.WithHotPatchOptionChan(hotpatchChan),
		aicommon.WithEnablePlanAndExec(true),
	)
	react := &ReAct{config: cfg}

	err := react.SendInputEventAndWaitAccepted(&ypb.AIInputEvent{
		IsConfigHotpatch: true,
		HotpatchType:     aicommon.HotPatchType_EnablePlan,
		Params:           &ypb.AIStartParams{EnablePlan: false},
	})
	require.NoError(t, err)

	select {
	case option := <-hotpatchChan.OutputChannel():
		require.NotNil(t, option)
		require.NoError(t, option(cfg))
		require.False(t, cfg.GetEnablePlanAndExec())
	case <-time.After(time.Second):
		t.Fatal("hot-patch was not dispatched before SendInputEventAndWaitAccepted returned")
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
