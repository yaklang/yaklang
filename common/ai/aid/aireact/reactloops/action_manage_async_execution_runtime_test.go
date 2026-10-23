package reactloops

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
)

// One scenario per protocol: submit -> list/inspect -> wait -> stop/stop_all.
// The mock model uses the real action parser; resources use the real manager.
func TestManageAsyncExecutionRuntime(t *testing.T) {
	for _, protocol := range []string{"text_root", "text_next_action", "native"} {
		t.Run(protocol, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			native := protocol == "native"
			const name = schema.AI_REACT_LOOP_ACTION_MANAGE_ASYNC_EXECUTION_RUNTIME
			var payload string
			cfg := aicommon.NewConfig(ctx, aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
				aicommon.WithDisableCreateDBRuntime(true), aicommon.WithWorkdir(t.TempDir()), aicommon.WithNoOpMemoryTriage(),
				aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
					resp := c.NewAIResponse()
					if native {
						wire.ToolCallCallback([]*aispec.ToolCall{{ID: "manage", Type: "function",
							Function: aispec.FuncReturn{Name: name, Arguments: payload}}})
						wire.FinishReasonCallback("tool_calls", nil)
					} else {
						resp.EmitOutputStream(strings.NewReader(payload))
						wire.FinishReasonCallback("stop", nil)
					}
					resp.Close()
					return resp, nil
				}))
			invoker := mock.NewMockInvoker(ctx)
			invoker.SetConfig(cfg)
			loop, err := NewReActLoop("async-management-test", invoker, WithFunctionCallMode(native),
				WithAllowToolCall(false), WithAllowAIForge(false), WithAllowRAG(false), WithAllowPlanAndExec(false),
				WithAllowUserInteract(false), WithRegisterLoopAction("require_tool", "unused routing", nil, nil, nil))
			require.NoError(t, err)
			defer loop.Release()
			task := mock.NewMockStatefulTask(ctx, "async-management-task", "manage background work")
			loop.SetCurrentTask(task)
			handler, err := loop.GetActionHandler(name)
			require.NoError(t, err, "core action exists even with tool calls disabled")
			loop.actionFilters = []func(*LoopAction) bool{func(action *LoopAction) bool { return action.ActionType == "finish" }}
			loop.initActionDisabled = []string{name}
			textSchema, nativeSchema, err := loop.prepareLoopActionSchemas(nil)
			require.NoError(t, err)
			require.Contains(t, textSchema+nativeSchema, name, "disable and filter narrowing still advertise management")
			narrow := NewActionHandlerOperator(task)
			narrow.RemoveNextAction(name)
			require.Contains(t, loop.getFilteredActions(false, narrow), handler, "management survives disable narrowing")
			require.Nil(t, loop.asyncExecutionManager, "registration and rendering must not create a manager")

			run := func(params aitool.InvokeParams) map[string]json.RawMessage {
				t.Helper()
				params["identifier"] = "manage_background"
				var wireParams any = params
				if !native {
					params["@action"] = name
					if protocol == "text_next_action" {
						wireParams = map[string]any{"@action": name, "next_action": params}
					}
				}
				encoded, err := json.Marshal(wireParams)
				require.NoError(t, err)
				payload = string(encoded)
				var streams sync.WaitGroup
				calls, _, _, err := loop.callAILoopTransaction(&streams, textSchema+nativeSchema, "test", nil,
					func(io.Reader, io.Reader) {}, func(string, string, io.Reader, io.Reader) {})
				require.NoError(t, err)
				streams.Wait()
				require.Len(t, calls, 1)
				op := NewActionHandlerOperator(task)
				calls[0].LoopAction.ActionHandler(loop, calls[0].Action, op)
				loop.ClearActionExecutionValues(calls[0].Action)
				require.True(t, op.IsContinued())
				require.Zero(t, op.GetExecutedToolCallCount(), "management does not execute another tool")
				var feedback map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(op.GetFeedback().Bytes(), &feedback))
				return feedback
			}
			manage := func(op, id string) map[string]json.RawMessage {
				t.Helper()
				return run(aitool.InvokeParams{"op": op, "execute_id": id})
			}

			t.Log("1. Empty list is valid; repeated actions reuse the same owner manager")
			require.JSONEq(t, `[]`, string(run(aitool.InvokeParams{})["executions"]))
			manager, err := loop.GetAsyncExecutionManager()
			require.NoError(t, err)
			t.Cleanup(func() {
				cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), time.Second)
				defer cleanupCancel()
				require.NoError(t, manager.Shutdown(cleanupCtx))
			})
			var inspections atomic.Int32
			finished := make(chan struct{})
			started := make(chan struct{})
			resource, err := asyncexecmanager.NewResource(func(ctx context.Context) error {
				close(started)
				select {
				case <-finished:
					return nil
				case <-ctx.Done():
					return ctx.Err()
				}
			}, func(context.Context) (asyncexecmanager.Observation, error) {
				inspections.Add(1)
				return asyncexecmanager.Observation{Stdout: "live output", Stderr: "diagnostic",
					PartialResult: json.RawMessage(`{"progress":1}`)}, nil
			})
			require.NoError(t, err)
			receipt, err := manager.Start(asyncexecmanager.Request{Resource: resource, Metadata: asyncexecmanager.Metadata{Name: "mock-tool"}})
			require.NoError(t, err)
			select {
			case <-started:
			case <-ctx.Done():
				t.Fatal("execution did not start")
			}

			t.Log("2. List reads metadata; inspect reaches the original observer")
			require.Contains(t, string(manage("list", "")["executions"]), receipt.ID)
			require.Zero(t, inspections.Load())
			var snapshot asyncexecmanager.Snapshot
			require.NoError(t, json.Unmarshal(manage(" INSPECT ", " "+receipt.ID+" ")["execution"], &snapshot))
			require.Equal(t, receipt.ID, snapshot.ID)
			require.Equal(t, "live output", snapshot.Observation.Stdout)
			require.Equal(t, "diagnostic", snapshot.Observation.Stderr)
			require.JSONEq(t, `{"progress":1}`, string(snapshot.Observation.PartialResult))
			require.EqualValues(t, 1, inspections.Load())
			listAction := aicommon.NewSimpleAction(name, aitool.InvokeParams{"op": "list"})
			inspectAction := aicommon.NewSimpleAction(name, aitool.InvokeParams{"op": "inspect", "execute_id": receipt.ID})
			require.NoError(t, handler.ActionVerifier(loop, listAction))
			require.NoError(t, handler.ActionVerifier(loop, inspectAction))
			require.Equal(t, "list", loop.GetActionExecutionValue(listAction, manageAsyncRequestKey).(manageAsyncRequest).op,
				"verifying the next call must not overwrite an earlier call in the same batch")
			loop.ClearActionExecutionValues(listAction)
			loop.ClearActionExecutionValues(inspectAction)

			t.Log("3. A zero wait returns running state without cancellation; completed wait is terminal")
			feedback := run(aitool.InvokeParams{"op": "wait", "execute_id": receipt.ID, "timeout_seconds": "0"})
			require.JSONEq(t, `true`, string(feedback["wait_timed_out"]))
			require.NotContains(t, feedback, "error")
			var summary asyncexecmanager.Summary
			require.NoError(t, json.Unmarshal(feedback["execution"], &summary))
			require.Equal(t, asyncexecmanager.StateRunning, summary.State)
			close(finished)
			require.NoError(t, json.Unmarshal(manage("wait", receipt.ID)["execution"], &summary))
			require.Equal(t, asyncexecmanager.StateCompleted, summary.State)
			require.EqualValues(t, 1, inspections.Load(), "wait must not sample output implicitly")

			t.Log("4. Stop requests cancellation; stop_all is scoped and keeps admission open")
			blocked, err := asyncexecmanager.NewResource(func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() },
				func(context.Context) (asyncexecmanager.Observation, error) {
					return asyncexecmanager.Observation{}, nil
				})
			require.NoError(t, err)
			second, err := manager.Start(asyncexecmanager.Request{Resource: blocked})
			require.NoError(t, err)
			require.NotContains(t, manage("stop", second.ID), "error")
			require.NoError(t, json.Unmarshal(manage("wait", second.ID)["execution"], &summary))
			require.Equal(t, asyncexecmanager.StateCancelled, summary.State)
			third, err := manager.Start(asyncexecmanager.Request{Resource: blocked})
			require.NoError(t, err)
			require.Contains(t, string(manage("stop_all", "")["executions"]), third.ID)
			require.NoError(t, json.Unmarshal(manage("wait", third.ID)["execution"], &summary))
			require.Equal(t, asyncexecmanager.StateCancelled, summary.State)
			_, err = manager.Start(asyncexecmanager.Request{Resource: blocked})
			require.NoError(t, err, "stop_all must not close the manager")
			require.Contains(t, string(manage("inspect", "missing")["error"]), asyncexecmanager.ErrNotFound.Error())
			same, err := loop.GetAsyncExecutionManager()
			require.NoError(t, err)
			require.Same(t, manager, same)

			t.Log("5. Invalid requests leave no stale per-action state")
			for _, params := range []aitool.InvokeParams{
				{"op": "unknown"}, {"op": "stop", "execute_id": " "},
				{"op": "wait", "execute_id": receipt.ID, "timeout_seconds": -1},
				{"op": "wait", "execute_id": receipt.ID, "timeout_seconds": 61},
				{"op": "wait", "execute_id": receipt.ID, "timeout_seconds": "bad"},
			} {
				action := aicommon.NewSimpleAction(name, params)
				loop.SetActionExecutionValue(action, manageAsyncRequestKey, manageAsyncRequest{op: "stop_all"})
				require.Error(t, handler.ActionVerifier(loop, action))
				require.Nil(t, loop.GetActionExecutionValue(action, manageAsyncRequestKey))
			}
			require.Error(t, handler.ActionVerifier(loop, nil))
			require.Error(t, handler.ActionVerifier(nil, aicommon.NewSimpleAction(name, nil)))
			require.NoError(t, ctx.Err(), "scenario stayed within its owner deadline")
		})
	}
}
