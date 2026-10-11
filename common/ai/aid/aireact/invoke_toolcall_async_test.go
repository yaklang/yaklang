package aireact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
)

// Exercise the production chain from the loop's public entry, without manually
// creating a caller or a resource: prepare, register, observe, finish and cancel.
func TestReActLoopStartAsyncToolCall(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	started := map[string]chan struct{}{"finish": make(chan struct{}), "cancel": make(chan struct{})}
	finish := make(chan struct{})
	var finishOnce sync.Once
	releaseFinish := func() { finishOnce.Do(func() { close(finish) }) }
	defer releaseFinish()
	await := func(signal <-chan struct{}) {
		t.Helper()
		select {
		case <-signal:
		case <-ctx.Done():
			t.Fatal("tool did not reach the expected phase")
		}
	}
	tool, err := aitool.New("async_entry_fixture",
		aitool.WithStringParam("value", aitool.WithParam_Required(true)),
		aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtime *aitool.ToolRuntimeConfig, stdout, stderr io.Writer) (any, error) {
			value := params.GetString("value")
			_, _ = fmt.Fprintln(stdout, value)
			_, _ = fmt.Fprintln(stderr, "warning")
			phase := strings.Split(value, "/")[0]
			close(started[phase])
			if phase == "cancel" {
				<-ctx.Done()
				_, _ = fmt.Fprintln(stdout, "cleaned")
				return nil, ctx.Err()
			}
			select {
			case <-finish:
				return map[string]any{"value": value, "owner": runtime.CurrentTaskUserInput}, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}),
	)
	require.NoError(t, err)
	cfg := aicommon.NewConfig(ctx, aicommon.WithTools(tool), aicommon.WithAgreeYOLO(),
		aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true),
		aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
		aicommon.WithNoOpMemoryTriage(), aicommon.WithDisableToolCallerIntervalReview(true),
		aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			return nil, errors.New("this test must not invoke AI")
		}),
	)
	runtime := &ReAct{config: cfg, Emitter: cfg.Emitter}
	loop := reactloops.NewMinimalReActLoop(cfg, runtime)
	defer loop.Release()
	owner := aicommon.NewStatefulTaskBase("async-owner", "original owner query", ctx, cfg.Emitter, true)
	loop.SetCurrentTask(owner)
	reactloops.WithToolInvokeGuard(func(_ string, params aitool.InvokeParams) (bool, string) {
		return params == nil || params.GetString("value") != "blocked", "owner rejected this call"
	})(loop)
	reactloops.WithToolInvokeParamsMutator(func(_ string, params aitool.InvokeParams) aitool.InvokeParams {
		return aitool.InvokeParams{"value": params.GetString("value") + "/mutated"}
	})(loop)
	// A different foreground task must not supply guards, context or results.
	decoy := aicommon.NewStatefulTaskBase("decoy", "wrong query", ctx, cfg.Emitter, true)
	decoyLoop := reactloops.NewMinimalReActLoop(cfg, runtime)
	defer decoyLoop.Release()
	decoyLoop.SetCurrentTask(decoy)
	reactloops.WithToolInvokeGuard(func(string, aitool.InvokeParams) (bool, string) { return false, "wrong loop" })(decoyLoop)
	manager, err := loop.GetAsyncExecutionManager()
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, manager.Shutdown(cleanup))
	})

	t.Log("1. The loop uses the explicit owner's guards before registering any execution")
	_, err = loop.StartAsyncToolCall(tool.Name, aitool.InvokeParams{"value": "blocked"})
	require.ErrorContains(t, err, "owner rejected")
	require.Empty(t, manager.List())

	t.Log("2. The loop prepares a real caller and returns its manager receipt without waiting")
	prepared, advance := make(chan struct{}), make(chan struct{})
	params := aitool.InvokeParams{"value": "finish"}
	receipt, err := loop.StartAsyncToolCall(tool.Name, params,
		aicommon.WithToolCaller_Reason("async entry test"),
		aicommon.WithToolCaller_BeforeInvoke(func(ctx context.Context, _ *aitool.Tool, _ aitool.InvokeParams) (func(), error) {
			close(prepared)
			select {
			case <-advance:
				return nil, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}),
	)
	require.NoError(t, err)
	require.NotEmpty(t, receipt.ID)
	require.Equal(t, owner.GetId(), receipt.Metadata.Attributes["task_id"])
	await(prepared)
	params["value"] = "changed after submission"
	close(advance)
	await(started["finish"])
	live, err := manager.Inspect(ctx, receipt.ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateRunning, live.State)
	require.Equal(t, "finish/mutated\n", live.Observation.Stdout)
	require.Equal(t, "warning\n", live.Observation.Stderr)

	t.Log("3. The original task and timeline receive the same canonical result as Inspector")
	releaseFinish()
	completed, err := manager.Wait(ctx, receipt.ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateCompleted, completed.State)
	snapshot, err := manager.Inspect(ctx, receipt.ID)
	require.NoError(t, err)
	var result aitool.ToolResult
	require.NoError(t, json.Unmarshal(snapshot.Observation.Result, &result))
	require.True(t, result.Success)
	require.Contains(t, fmt.Sprint(result.Data), "original owner query")
	require.Len(t, owner.GetAllToolCallResults(), 1)
	require.Equal(t, owner.GetAllToolCallResults()[0].ID, result.ID)
	require.Contains(t, cfg.Timeline.Dump(), "original owner query")
	require.Empty(t, decoy.GetAllToolCallResults())

	t.Log("4. Release stops an actual async call and rejects later loop submissions")
	receipt, err = loop.StartAsyncToolCall(tool.Name, aitool.InvokeParams{"value": "cancel"}, aicommon.WithToolCaller_Reason("cancel test"))
	require.NoError(t, err)
	await(started["cancel"])
	loop.Release()
	cancelled, err := manager.Wait(ctx, receipt.ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateCancelled, cancelled.State)
	snapshot, err = manager.Inspect(ctx, receipt.ID)
	require.NoError(t, err)
	require.Contains(t, snapshot.Observation.Stdout, "cleaned")
	_, err = loop.StartAsyncToolCall(tool.Name, aitool.InvokeParams{"value": "unused"})
	require.ErrorIs(t, err, asyncexecmanager.ErrClosed)
}
