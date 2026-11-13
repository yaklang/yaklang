package reactloops

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
)

func TestAsyncExecutionSnapshotDelegatesToRuntime(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cfg := aicommon.NewConfig(ctx,
		aicommon.WithAgreeYOLO(),
		aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisallowMCPServers(true),
		aicommon.WithNoOpMemoryTriage(),
		aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, errors.New("async execution sync test must not invoke AI")
		}),
	)
	invoker := mock.NewMockInvoker(ctx)
	invoker.SetConfig(cfg)
	loop := NewMinimalReActLoop(cfg, invoker)
	task := aicommon.NewStatefulTaskBase("async-sync-task", "run background tool", ctx, cfg.GetEmitter(), true)
	loop.SetCurrentTask(task)
	t.Cleanup(loop.Release)

	withoutRuntime := NewMinimalReActLoop(cfg, nil)
	t.Cleanup(withoutRuntime.Release)
	require.False(t, withoutRuntime.HasAsyncExecutionManager())
	require.Empty(t, withoutRuntime.AsyncExecutionSnapshot().Executions)
	_, err := withoutRuntime.GetAsyncExecutionManager()
	require.ErrorContains(t, err, "requires an AI invoke runtime")

	manager, err := loop.GetAsyncExecutionManager()
	require.NoError(t, err)
	require.True(t, loop.HasAsyncExecutionManager())
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

	running := loop.AsyncExecutionSnapshot()
	require.Equal(t, 1, running.RunningCount)
	require.Len(t, running.Executions, 1)
	require.Equal(t, asyncexecmanager.StateRunning, running.Executions[0].State)
	require.Equal(t, task.GetId(), running.CurrentTaskID)
	require.NotZero(t, running.Revision)
	require.NotZero(t, running.UpdatedAt)

	_, err = manager.Stop(receipt.ID)
	require.NoError(t, err)
	stopping := loop.AsyncExecutionSnapshot()
	require.Equal(t, 1, stopping.RunningCount)
	require.Len(t, stopping.Executions, 1)
	require.Equal(t, asyncexecmanager.StateStopping, stopping.Executions[0].State)
	require.Greater(t, stopping.Revision, running.Revision)

	close(releaseCleanup)
	final, err := manager.Wait(ctx, receipt.ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateCancelled, final.State)
	terminal := loop.AsyncExecutionSnapshot()
	require.Zero(t, terminal.RunningCount)
	require.Len(t, terminal.Executions, 1)
	require.Equal(t, asyncexecmanager.StateCancelled, terminal.Executions[0].State)
	require.Greater(t, terminal.Revision, stopping.Revision)
}
