package reactloops

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/schema"
)

func TestAsyncExecutionLifecyclePushEvents(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan *schema.AiOutputEvent, 32)
	cfg := aicommon.NewConfig(ctx,
		aicommon.WithAgreeYOLO(),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithNoOpMemoryTriage(),
		aicommon.WithEventHandler(func(event *schema.AiOutputEvent) {
			events <- event
		}),
		aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, errors.New("async execution sync test must not invoke AI")
		}),
	)
	loop := NewMinimalReActLoop(cfg, nil)
	task := aicommon.NewStatefulTaskBase("async-sync-task", "run background tool", ctx, cfg.GetEmitter(), true)
	loop.SetCurrentTask(task)
	t.Cleanup(loop.Release)

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
		ID: "async-lifecycle",
		Metadata: asyncexecmanager.Metadata{
			Name: "blocked_tool",
		},
		Resource: resource,
	})
	require.NoError(t, err)
	select {
	case <-started:
	case <-ctx.Done():
		t.Fatal("background resource did not start")
	}

	running := waitAsyncExecutionPush(t, events, func(snapshot AsyncExecutionSnapshot) bool {
		return snapshot.RunningCount == 1 && len(snapshot.Executions) == 1 &&
			snapshot.Executions[0].State == asyncexecmanager.StateRunning
	})
	require.Equal(t, task.GetId(), running.TaskID)
	require.Equal(t, loop.GetLoopName(), running.LoopName)
	require.NotZero(t, running.Revision)
	require.NotZero(t, running.UpdatedAt)

	_, err = manager.Stop(receipt.ID)
	require.NoError(t, err)
	stopping := waitAsyncExecutionPush(t, events, func(snapshot AsyncExecutionSnapshot) bool {
		return snapshot.RunningCount == 1 && len(snapshot.Executions) == 1 &&
			snapshot.Executions[0].State == asyncexecmanager.StateStopping
	})
	require.Greater(t, stopping.Revision, running.Revision)

	close(releaseCleanup)
	final, err := manager.Wait(ctx, receipt.ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateCancelled, final.State)
	terminal := waitAsyncExecutionPush(t, events, func(snapshot AsyncExecutionSnapshot) bool {
		return snapshot.RunningCount == 0 && len(snapshot.Executions) == 1 &&
			snapshot.Executions[0].State == asyncexecmanager.StateCancelled
	})
	require.Greater(t, terminal.Revision, stopping.Revision)
}

func waitAsyncExecutionPush(t *testing.T, events <-chan *schema.AiOutputEvent, match func(AsyncExecutionSnapshot) bool) AsyncExecutionSnapshot {
	t.Helper()
	timeout := time.NewTimer(3 * time.Second)
	defer timeout.Stop()
	for {
		select {
		case event := <-events:
			if event == nil || event.IsSync || event.Type != schema.EVENT_TYPE_STRUCTURED ||
				event.NodeId != AsyncExecutionSyncNodeID {
				continue
			}
			var snapshot AsyncExecutionSnapshot
			require.NoError(t, json.Unmarshal(event.Content, &snapshot))
			if match(snapshot) {
				return snapshot
			}
		case <-timeout.C:
			t.Fatal("timed out waiting for async execution push event")
		}
	}
}
