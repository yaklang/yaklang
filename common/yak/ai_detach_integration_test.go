package yak_test

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools/yakscripttools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yak"
	"strings"
	"testing"
	"time"
)

// Real Yak script -> runtime Detach -> original ToolCaller -> owning manager.
// Cancellation runs Yak defer code before the execution can become terminal.
func TestYakAIDetachExecutionAndCleanup(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	source := `__DESC__ = "detachment integration"
__ASYNC__ = false
println("before detach")
first = ai.Detach()~
second = ai.Detach()~
assert(first == second)
println("after detach")
defer func(){ println("yak cleanup") }()
for { sleep(0.01) }
`
	spec := yakscripttools.LoadYakScriptToAiTools("yak_detach_fixture", source)
	require.NotNil(t, spec)
	tools := yak.YakTool2AITool([]*schema.AIYakTool{spec})
	require.Len(t, tools, 1)
	aitool.WithDangerousNoNeedUserReview(true)(tools[0])
	runtime, err := aireact.NewTestReAct(aicommon.WithContext(ctx), aicommon.WithTools(tools[0]), aicommon.WithAgreeYOLO(), aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableToolCallerIntervalReview(true), aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		return nil, errors.New("unexpected AI request")
	}))
	require.NoError(t, err)
	loop, err := reactloops.NewReActLoop("yak-detach-test", runtime, reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableLoopPerception(true))
	require.NoError(t, err)
	defer loop.Release()
	task := aicommon.NewStatefulTaskBase("yak-owner", "run the Yak service", ctx, runtime.GetConfig().GetEmitter(), true)
	loop.SetCurrentTask(task)
	action := aicommon.NewSimpleAction(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL, aitool.InvokeParams{"directly_call_tool_name": tools[0].Name, "directly_call_tool_params": aitool.InvokeParams{}, "directly_call_reason": "verify Yak detachment"})
	handler, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
	require.NoError(t, err)
	require.NoError(t, handler.ActionVerifier(loop, action))
	op := reactloops.NewActionHandlerOperator(task)
	done := make(chan struct{})
	go func() { defer close(done); handler.ActionHandler(loop, action, op) }()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("script never detached")
	}
	manager, err := loop.GetAsyncExecutionManager()
	require.NoError(t, err)
	entries := manager.List()
	require.Len(t, entries, 1)
	require.Eventually(t, func() bool {
		snapshot, err := manager.Inspect(ctx, entries[0].ID)
		return err == nil && strings.Contains(snapshot.Observation.Stdout, "after detach")
	}, time.Second, 5*time.Millisecond)
	_, err = manager.Stop(entries[0].ID)
	require.NoError(t, err)
	final, err := manager.Wait(ctx, entries[0].ID)
	require.NoError(t, err)
	require.Equal(t, asyncexecmanager.StateCancelled, final.State)
	snapshot, err := manager.Inspect(ctx, entries[0].ID)
	require.NoError(t, err)
	require.Contains(t, snapshot.Observation.Stdout, "before detach")
	require.Contains(t, snapshot.Observation.Stdout, "yak cleanup")
	require.Equal(t, 1, strings.Count(snapshot.Observation.Stdout, "before detach"), "the Yak script must never restart")
	var result aitool.ToolResult
	require.NoError(t, json.Unmarshal(snapshot.Observation.Result, &result))
	require.Equal(t, tools[0].Name, result.Name)
}
