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

func TestToolCallResourceWithAsyncManager(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cfg := NewTestConfig(ctx, WithAgreeYOLO(), WithWorkdir(t.TempDir()), WithDisableCreateDBRuntime(true))
	manager, err := asyncexecmanager.New(ctx, asyncexecmanager.Config{})
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, manager.Shutdown(cleanup))
	})
	newCaller := func() *ToolCaller {
		t.Helper()
		caller, err := NewToolCaller(ctx,
			WithToolCaller_AICallerConfig(cfg), WithToolCaller_Task(cfg.DefaultTask),
			WithToolCaller_Emitter(cfg.Emitter), WithToolCaller_Reason("resource test"),
		)
		require.NoError(t, err)
		t.Cleanup(caller.cancel)
		return caller
	}
	callbackCount := 0
	tool, err := aitool.New("resource_fixture",
		aitool.WithStringParam("value", aitool.WithParam_Required(true)),
		aitool.WithStructParam("payload", nil, aitool.WithStringParam("nested")),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithNoRuntimeCallback(func(_ context.Context, params aitool.InvokeParams, stdout, _ io.Writer) (any, error) {
			callbackCount++
			_, _ = fmt.Fprintln(stdout, params.GetString("value"))
			if params.GetString("value") == "fail" {
				return nil, errors.New("fixture failure")
			}
			return params["payload"], nil
		}),
	)
	require.NoError(t, err)

	t.Log("1. Preparation rejects invalid input, captures parameters and does not run the tool")
	var missing *ToolCaller
	_, err = missing.NewAsyncResource(tool, aitool.InvokeParams{})
	require.ErrorIs(t, err, asyncexecmanager.ErrInvalidResource)
	noTask, err := NewToolCaller(ctx, WithToolCaller_AICallerConfig(cfg))
	require.NoError(t, err)
	t.Cleanup(noTask.cancel)
	_, err = noTask.NewAsyncResource(tool, aitool.InvokeParams{})
	require.ErrorIs(t, err, asyncexecmanager.ErrInvalidResource)
	caller := newCaller()
	_, err = caller.NewAsyncResource(nil, aitool.InvokeParams{})
	require.ErrorIs(t, err, asyncexecmanager.ErrInvalidResource)
	_, err = caller.NewAsyncResource(tool, nil)
	require.ErrorIs(t, err, asyncexecmanager.ErrInvalidResource)
	params := aitool.InvokeParams{"value": "ok", "payload": map[string]any{"nested": "captured"}}
	resource, err := caller.NewAsyncResource(tool, params)
	require.NoError(t, err)
	require.Same(t, caller.Inspector(), resource.Inspector())
	require.Zero(t, callbackCount)
	params["value"] = "changed"
	params["payload"].(map[string]any)["nested"] = "changed"
	_, err = caller.NewAsyncResource(tool, params)
	require.ErrorIs(t, err, asyncexecmanager.ErrInvalidResource, "one caller belongs to one resource")

	t.Log("2. Submit through the manager; observe original stdout and a successful result")
	receipt, err := manager.Start(asyncexecmanager.Request{Resource: resource})
	require.NoError(t, err)
	completed, err := manager.Wait(ctx, receipt.ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateCompleted, completed.State)
	observer, err := manager.Inspector(receipt.ID)
	require.NoError(t, err)
	require.Same(t, caller.Inspector(), observer)
	snapshot, err := manager.Inspect(ctx, receipt.ID)
	require.NoError(t, err)
	require.Equal(t, "ok\n", snapshot.Observation.Stdout)
	var result aitool.ToolResult
	require.NoError(t, json.Unmarshal(snapshot.Observation.Result, &result))
	require.True(t, result.Success)
	require.Contains(t, fmt.Sprint(result.Data), "captured")
	require.NotContains(t, fmt.Sprint(result.Data), "changed")
	require.ErrorContains(t, resource.Execute(ctx), "already executed")
	require.Equal(t, 1, callbackCount, "reusing a resource cannot invoke the tool again")

	t.Log("3. A failed ToolResult becomes a failed execution, with the original result retained")
	failing, err := newCaller().NewAsyncResource(tool, aitool.InvokeParams{"value": "fail"})
	require.NoError(t, err)
	receipt, err = manager.Start(asyncexecmanager.Request{Resource: failing})
	require.NoError(t, err)
	failed, err := manager.Wait(ctx, receipt.ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateFailed, failed.State)
	require.Contains(t, failed.Error, "fixture failure")
	snapshot, err = manager.Inspect(ctx, receipt.ID)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(snapshot.Observation.Result, &result))
	require.False(t, result.Success)
	require.Contains(t, result.Error, "fixture failure")
	require.Equal(t, 2, callbackCount)

	t.Log("4. A cancelled execution context never invokes the callback")
	unstarted, err := newCaller().NewAsyncResource(tool, aitool.InvokeParams{"value": "unused"})
	require.NoError(t, err)
	require.ErrorContains(t, unstarted.Execute(nil), "context is required")
	stopped, stop := context.WithCancel(ctx)
	stop()
	require.ErrorIs(t, unstarted.Execute(stopped), context.Canceled)
	require.Equal(t, 2, callbackCount)

	t.Log("5. The original parameter validation still rejects missing required arguments")
	invalid, err := newCaller().NewAsyncResource(tool, aitool.InvokeParams{})
	require.NoError(t, err)
	receipt, err = manager.Start(asyncexecmanager.Request{Resource: invalid})
	require.NoError(t, err)
	rejected, err := manager.Wait(ctx, receipt.ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateFailed, rejected.State)
	require.NotEmpty(t, rejected.Error)
	require.Equal(t, 2, callbackCount, "invalid arguments never reach the plugin callback")
}
