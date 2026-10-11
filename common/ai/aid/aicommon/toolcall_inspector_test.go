package aicommon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// One scenario, two real ToolCaller runs: completion and cancellation with
// delayed cleanup. Channels make every observation phase deterministic.
func TestToolCallerInspectorWithAsyncManager(t *testing.T) {
	owner, cancelOwner := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelOwner()
	type phase struct {
		started, advance, progressed, finish, cancelled, cleanup chan struct{}
	}
	phases := make(map[string]*phase)
	for _, name := range []string{"complete", "cancel"} {
		phases[name] = &phase{make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})}
	}
	await := func(signal <-chan struct{}) {
		t.Helper()
		select {
		case <-signal:
		case <-owner.Done():
			t.Fatal("tool did not reach the expected phase")
		}
	}
	tool, err := aitool.New("inspector_fixture",
		aitool.WithStringParam("phase", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithNoRuntimeCallback(func(ctx context.Context, params aitool.InvokeParams, stdout, stderr io.Writer) (any, error) {
			name := params.GetString("phase")
			p := phases[name]
			_, _ = fmt.Fprintln(stdout, name+": start")
			_, _ = fmt.Fprintln(stderr, name+": warning")
			close(p.started)
			select {
			case <-p.advance:
			case <-ctx.Done():
				return nil, ctx.Err()
			}
			_, _ = fmt.Fprintln(stdout, name+": progress")
			close(p.progressed)
			select {
			case <-p.finish:
				return map[string]any{"phase": name, "done": true}, nil
			case <-ctx.Done():
				close(p.cancelled)
				// Model a plugin still joining its workers after cancellation.
				select {
				case <-p.cleanup:
				case <-owner.Done():
				}
				_, _ = fmt.Fprintln(stdout, name+": cleaned")
				return nil, ctx.Err()
			}
		}),
	)
	require.NoError(t, err)
	cfg := NewTestConfig(owner, WithAgreeYOLO(), WithWorkdir(t.TempDir()))
	manager, err := asyncexecmanager.New(owner, asyncexecmanager.Config{MaxConcurrent: 1})
	require.NoError(t, err)
	t.Cleanup(func() {
		cancelOwner()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, manager.Shutdown(ctx))
	})
	var returnedResult *aitool.ToolResult
	start := func(name string) *ToolCaller {
		t.Helper()
		callCtx, cancelCall := context.WithCancel(owner)
		t.Cleanup(cancelCall)
		caller, err := NewToolCaller(callCtx,
			WithToolCaller_AICallerConfig(cfg), WithToolCaller_Task(cfg.DefaultTask),
			WithToolCaller_Emitter(cfg.Emitter), WithToolCaller_Reason("inspector test"),
			WithToolCaller_WaitForCallbackOnCancel(true),
		)
		require.NoError(t, err)
		require.Same(t, caller, caller.Inspector())
		before, err := caller.Inspect(owner)
		require.NoError(t, err)
		require.Empty(t, before.Stdout)
		require.Empty(t, before.Result)
		resource, err := asyncexecmanager.NewResource(func(ctx context.Context) error {
			// Link this manager execution's cancellation to the original caller.
			stop := context.AfterFunc(ctx, cancelCall)
			defer stop()
			defer cancelCall()
			result, _, err := caller.CallToolWithExistedParams(tool, aitool.InvokeParams{"phase": name})
			if name == "complete" {
				returnedResult = result
			}
			if err == nil && result != nil && !result.Success {
				err = errors.New(result.Error)
			}
			return err
		}, caller.Inspect)
		require.NoError(t, err)
		receipt, err := manager.Start(asyncexecmanager.Request{ID: name, Resource: resource})
		require.NoError(t, err)
		require.Equal(t, asyncexecmanager.StatePending, receipt.State)
		return caller
	}
	inspect := func(id string) asyncexecmanager.Snapshot {
		t.Helper()
		snapshot, err := manager.Inspect(owner, id)
		require.NoError(t, err)
		return snapshot
	}

	t.Log("1. Start a real tool, inspect live stdout/stderr, then inspect new output")
	caller := start("complete")
	await(phases["complete"].started)
	first := inspect("complete")
	require.Equal(t, asyncexecmanager.StateRunning, first.State)
	require.Equal(t, "complete: start\n", first.Observation.Stdout)
	require.Equal(t, "complete: warning\n", first.Observation.Stderr)
	require.Empty(t, first.Observation.Result)
	close(phases["complete"].advance)
	await(phases["complete"].progressed)
	live := inspect("complete")
	require.Contains(t, live.Observation.Stdout, "complete: progress")
	direct, err := caller.Inspect(owner)
	require.NoError(t, err)
	require.Equal(t, direct, live.Observation, "manager reads the original caller, not an event cache")

	t.Log("2. Finish, wait, and retain the canonical result and immutable snapshots")
	close(phases["complete"].finish)
	completed, err := manager.Wait(owner, "complete")
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateCompleted, completed.State)
	final := inspect("complete")
	var result aitool.ToolResult
	require.NoError(t, json.Unmarshal(final.Observation.Result, &result))
	require.True(t, result.Success)
	require.Equal(t, "inspector_fixture", result.Name)
	require.Nil(t, result.Param, "inspection must not retain callback-owned mutable parameters")
	require.Contains(t, fmt.Sprint(result.Data), "done")
	require.NotNil(t, returnedResult)
	returnedResult.Name = "changed by result recipient"
	require.NoError(t, json.Unmarshal(inspect("complete").Observation.Result, &result))
	require.Equal(t, "inspector_fixture", result.Name, "final observations are frozen before returning ToolResult")
	final.Observation.Result[0] = 'x'
	require.True(t, json.Valid(inspect("complete").Observation.Result), "returned JSON must not alias caller state")
	cancelledRead, cancelRead := context.WithCancel(owner)
	cancelRead()
	_, err = caller.Inspect(cancelledRead)
	require.ErrorIs(t, err, context.Canceled)

	t.Log("3. Stop a second run: cancellation must not end work before cleanup")
	start("cancel")
	await(phases["cancel"].started)
	close(phases["cancel"].advance)
	await(phases["cancel"].progressed)
	stopping, err := manager.Stop("cancel")
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateStopping, stopping.State)
	await(phases["cancel"].cancelled)
	waitCtx, cancelWait := context.WithTimeout(owner, 20*time.Millisecond)
	defer cancelWait()
	stillStopping, err := manager.Wait(waitCtx, "cancel")
	require.ErrorIs(t, err, context.DeadlineExceeded)
	require.Equal(t, asyncexecmanager.StateStopping, stillStopping.State)
	require.Empty(t, inspect("cancel").Observation.Result)

	t.Log("4. Release cleanup; Wait now completes and cleanup output remains inspectable")
	close(phases["cancel"].cleanup)
	stopped, err := manager.Wait(owner, "cancel")
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateCancelled, stopped.State)
	afterStop := inspect("cancel")
	require.Contains(t, afterStop.Observation.Stdout, "cancel: cleaned")
	result = aitool.ToolResult{}
	require.NoError(t, json.Unmarshal(afterStop.Observation.Result, &result))
	require.False(t, result.Success)
	require.Contains(t, result.Error, "cancel")
	require.Equal(t, "complete: start\ncomplete: progress\n", inspect("complete").Observation.Stdout, "callers retain independent observations")
}
