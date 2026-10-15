package reactloops

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// One real call: acquire the loop's shared manager, observe the original caller,
// then Release the loop and observe cancellation plus the final cleanup output.
func TestReActLoopAsyncExecutionToolCallerIntegration(t *testing.T) {
	owner, cancelOwner := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelOwner()
	cfg := aicommon.NewConfig(owner,
		aicommon.WithAgreeYOLO(), aicommon.WithWorkdir(t.TempDir()),
		aicommon.WithDisableCreateDBRuntime(true), aicommon.WithDisableAutoSkills(true),
		aicommon.WithDisallowMCPServers(true), aicommon.WithNoOpMemoryTriage(),
		aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, errors.New("this integration test must not invoke AI")
		}),
	)
	loop := NewMinimalReActLoop(cfg, nil)
	defer loop.Release()
	_, err := loop.GetAsyncExecutionManager()
	require.Error(t, err, "an owning task is required before creating the manager")
	loop.SetCurrentTask(cfg.DefaultTask)

	t.Log("1. Concurrent access initializes exactly one manager for this loop")
	var managers [8]*asyncexecmanager.Manager
	var errs [8]error
	var wg sync.WaitGroup
	for i := range managers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			managers[i], errs[i] = loop.GetAsyncExecutionManager()
		}(i)
	}
	wg.Wait()
	manager := managers[0]
	for i := range managers {
		require.NoError(t, errs[i])
		require.Same(t, manager, managers[i])
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, manager.Shutdown(ctx))
	})

	started := make(chan struct{})
	cancelled := make(chan struct{})
	cleanup := make(chan struct{})
	var cleanupOnce sync.Once
	releaseCleanup := func() { cleanupOnce.Do(func() { close(cleanup) }) }
	defer releaseCleanup()
	tool, err := aitool.New("loop_inspector_fixture",
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithNoRuntimeCallback(func(ctx context.Context, _ aitool.InvokeParams, stdout, stderr io.Writer) (any, error) {
			_, _ = fmt.Fprintln(stdout, "started")
			_, _ = fmt.Fprintln(stderr, "warning")
			close(started)
			<-ctx.Done()
			close(cancelled)
			select {
			case <-cleanup:
			case <-owner.Done():
			}
			_, _ = fmt.Fprintln(stdout, "cleaned")
			return nil, ctx.Err()
		}),
	)
	require.NoError(t, err)
	caller, err := aicommon.NewToolCaller(cfg.DefaultTask.GetContext(),
		aicommon.WithToolCaller_AICallerConfig(cfg), aicommon.WithToolCaller_Task(cfg.DefaultTask),
		aicommon.WithToolCaller_Emitter(cfg.Emitter), aicommon.WithToolCaller_Reason("loop integration test"),
	)
	require.NoError(t, err)
	resource, err := caller.NewAsyncResource(tool, aitool.InvokeParams{})
	require.NoError(t, err)
	require.Same(t, caller.Inspector(), resource.Inspector())
	receipt, err := manager.Start(asyncexecmanager.Request{Resource: resource})
	require.NoError(t, err)
	select {
	case <-started:
	case <-owner.Done():
		t.Fatal("tool did not start")
	}

	t.Log("2. The loop manager observes live stdout/stderr through ToolCaller Inspector")
	live, err := manager.Inspect(owner, receipt.ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateRunning, live.State)
	require.Equal(t, "started\n", live.Observation.Stdout)
	require.Equal(t, "warning\n", live.Observation.Stderr)
	direct, err := caller.Inspect(owner)
	require.NoError(t, err)
	require.Equal(t, direct, live.Observation)

	t.Log("3. Release cancels the tool, closes admission and retains final observations")
	loop.Release()
	loop.Release()
	require.NoError(t, owner.Err(), "Release must not cancel the owning task itself")
	select {
	case <-cancelled:
	case <-owner.Done():
		t.Fatal("tool did not receive cancellation")
	}
	waitCtx, cancelWait := context.WithTimeout(owner, 20*time.Millisecond)
	defer cancelWait()
	stopping, err := manager.Wait(waitCtx, receipt.ID)
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, asyncexecmanager.StateStopping, stopping.State)
	releaseCleanup()
	final, err := manager.Wait(owner, receipt.ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateCancelled, final.State)
	snapshot, err := manager.Inspect(owner, receipt.ID)
	require.NoError(t, err)
	require.Equal(t, "started\ncleaned\n", snapshot.Observation.Stdout)
	var result aitool.ToolResult
	require.NoError(t, json.Unmarshal(snapshot.Observation.Result, &result))
	require.Equal(t, tool.Name, result.Name)
	require.False(t, result.Success)
	retained, err := loop.GetAsyncExecutionManager()
	require.NoError(t, err)
	require.Same(t, manager, retained)
	_, err = retained.Start(asyncexecmanager.Request{Resource: resource})
	require.ErrorIs(t, err, asyncexecmanager.ErrClosed)
	unused := NewMinimalReActLoop(cfg, nil)
	unused.SetCurrentTask(cfg.DefaultTask)
	unused.Release()
	_, err = unused.GetAsyncExecutionManager()
	require.ErrorIs(t, err, asyncexecmanager.ErrClosed, "a released loop must not create a new manager")
}
