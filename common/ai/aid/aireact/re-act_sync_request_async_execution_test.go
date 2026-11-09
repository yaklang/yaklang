package aireact

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

func TestAsyncExecutionSyncQueryAndManualStop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan *schema.AiOutputEvent, 32)
	react, err := NewTestReAct(aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
		events <- event
	}))
	require.NoError(t, err)

	loop := reactloops.NewMinimalReActLoop(react.config, react)
	task := aicommon.NewStatefulTaskBase("async-sync-sync-task", "run in background", ctx, react.config.GetEmitter(), true)
	loop.SetCurrentTask(task)
	t.Cleanup(loop.Release)

	empty, err := readAsyncExecutionSyncEvent(t, events, func(event *schema.AiOutputEvent) bool {
		return event.IsSync && event.SyncID == "query-empty"
	}, func() error {
		return react.handleSyncMessage(&ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      aicommon.SYNC_TYPE_ASYNC_EXECUTION,
			SyncID:        "query-empty",
		})
	})
	require.NoError(t, err)
	require.Equal(t, 0, empty.RunningCount)
	require.Empty(t, empty.Executions)
	require.NotNil(t, empty.Executions, "the wire payload must use [] rather than null")
	require.Nil(t, loop.PeekAsyncExecutionManager(), "query must not create a manager")

	manager, err := loop.GetAsyncExecutionManager()
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
		defer cleanupCancel()
		require.NoError(t, manager.Shutdown(cleanupCtx))
	})

	started := make(chan struct{})
	releaseCleanup := make(chan struct{})
	resource, err := asyncexecmanager.NewResource(func(ctx context.Context) error {
		close(started)
		<-ctx.Done()
		<-releaseCleanup
		return ctx.Err()
	}, func(context.Context) (asyncexecmanager.Observation, error) {
		return asyncexecmanager.Observation{}, nil
	})
	require.NoError(t, err)
	receipt, err := manager.Start(asyncexecmanager.Request{
		ID:       "manual-stop-target",
		Metadata: asyncexecmanager.Metadata{Name: "long_running_tool"},
		Resource: resource,
	})
	require.NoError(t, err)
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("background resource did not start")
	}

	query, err := readAsyncExecutionSyncEvent(t, events, func(event *schema.AiOutputEvent) bool {
		return event.IsSync && event.SyncID == "query-running"
	}, func() error {
		return react.handleSyncMessage(&ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      aicommon.SYNC_TYPE_ASYNC_EXECUTION,
			SyncID:        "query-running",
		})
	})
	require.NoError(t, err)
	require.Equal(t, 1, query.RunningCount)
	require.Empty(t, query.Error)
	require.Len(t, query.Executions, 1)
	require.Equal(t, receipt.ID, query.Executions[0].ID)
	require.Equal(t, asyncexecmanager.StateRunning, query.Executions[0].State)

	stop, err := readAsyncExecutionSyncEvent(t, events, func(event *schema.AiOutputEvent) bool {
		return event.IsSync && event.SyncID == "manual-stop"
	}, func() error {
		return react.handleSyncMessage(&ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      aicommon.SYNC_TYPE_STOP_ASYNC_EXECUTION,
			SyncID:        "manual-stop",
			SyncJsonInput: `{"execute_id":"manual-stop-target"}`,
		})
	})
	require.NoError(t, err)
	require.Empty(t, stop.Error)
	require.Equal(t, 1, stop.RunningCount, "stopping still owns the execution slot")
	require.NotNil(t, stop.Execution)
	require.Equal(t, receipt.ID, stop.Execution.ID)
	require.Equal(t, asyncexecmanager.StateStopping, stop.Execution.State)

	close(releaseCleanup)
	final, err := manager.Wait(ctx, receipt.ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateCancelled, final.State)
	terminal, err := readAsyncExecutionSyncEvent(t, events, func(event *schema.AiOutputEvent) bool {
		return event.IsSync && event.SyncID == "query-terminal"
	}, func() error {
		return react.handleSyncMessage(&ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      aicommon.SYNC_TYPE_ASYNC_EXECUTION,
			SyncID:        "query-terminal",
		})
	})
	require.NoError(t, err)
	require.Equal(t, 0, terminal.RunningCount)
	require.Len(t, terminal.Executions, 1)
	require.Equal(t, asyncexecmanager.StateCancelled, terminal.Executions[0].State)

	invalid, err := readAsyncExecutionSyncEvent(t, events, func(event *schema.AiOutputEvent) bool {
		return event.IsSync && event.SyncID == "manual-stop-missing-id"
	}, func() error {
		return react.handleSyncMessage(&ypb.AIInputEvent{
			IsSyncMessage: true,
			SyncType:      aicommon.SYNC_TYPE_STOP_ASYNC_EXECUTION,
			SyncID:        "manual-stop-missing-id",
			SyncJsonInput: `{}`,
		})
	})
	require.NoError(t, err)
	require.Contains(t, invalid.Error, "execute_id is required")
	require.Equal(t, 0, invalid.RunningCount)
}

func readAsyncExecutionSyncEvent(
	t *testing.T,
	events <-chan *schema.AiOutputEvent,
	match func(*schema.AiOutputEvent) bool,
	request func() error,
) (reactloops.AsyncExecutionSnapshot, error) {
	t.Helper()
	requestErr := request()
	if requestErr != nil {
		return reactloops.AsyncExecutionSnapshot{}, requestErr
	}
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case event := <-events:
			if event == nil || !match(event) {
				continue
			}
			require.Equal(t, schema.EVENT_TYPE_STRUCTURED, event.Type)
			require.Equal(t, reactloops.AsyncExecutionSyncNodeID, event.NodeId)
			var snapshot reactloops.AsyncExecutionSnapshot
			if err := json.Unmarshal(event.Content, &snapshot); err != nil {
				return reactloops.AsyncExecutionSnapshot{}, err
			}
			return snapshot, nil
		case <-timeout.C:
			t.Fatal("timed out waiting for async execution sync response")
		}
	}
}
