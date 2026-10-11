package aireact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/schema"
)

// One scenario through the production action -> loop -> runtime -> ToolCaller
// -> manager chain. Only model input is simulated; tools and observers are real.
func TestDirectToolAsyncActionIntegration(t *testing.T) {
	for _, protocol := range []string{"text_root", "text_next_action", "native"} {
		t.Run(protocol, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			started := map[string]chan struct{}{}
			finished := map[string]chan struct{}{}
			for _, mode := range []string{"sync", "finish", "cancel", "fail"} {
				started[mode], finished[mode] = make(chan struct{}), make(chan struct{})
			}
			var callbacks atomic.Int32
			tool, err := aitool.New("async_action_fixture",
				aitool.WithStringParam("mode", aitool.WithParam_Required(true)),
				aitool.WithStringParam("value", aitool.WithParam_Required(true)),
				// A business flag does not opt into framework scheduling.
				aitool.WithBoolParam("async"),
				aitool.WithDangerousNoNeedUserReview(true),
				aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtime *aitool.ToolRuntimeConfig, stdout, stderr io.Writer) (any, error) {
					callbacks.Add(1)
					mode := params.GetString("mode")
					_, _ = fmt.Fprintln(stdout, params.GetString("value"))
					_, _ = fmt.Fprintln(stderr, "warning")
					close(started[mode])
					if mode == "fail" {
						return nil, errors.New("fixture execution failed")
					}
					select {
					case <-finished[mode]:
						return map[string]any{"value": params.GetString("value"), "owner": runtime.CurrentTaskUserInput}, nil
					case <-ctx.Done():
						_, _ = fmt.Fprintln(stdout, "cleaned")
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
					return nil, errors.New("this action integration must not call AI")
				}))
			runtime := &ReAct{config: cfg, Emitter: cfg.Emitter}
			loop, err := reactloops.NewReActLoop("async-action-test", runtime,
				reactloops.WithFunctionCallMode(protocol == "native"), reactloops.WithFunctionCallActionVariants(), reactloops.WithAllowAIForge(false),
				reactloops.WithAllowRAG(false), reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowUserInteract(false),
				reactloops.WithDisableLoopPerception(true), reactloops.WithDisablePeriodicVerification(true),
				reactloops.WithToolInvokeGuard(func(_ string, params aitool.InvokeParams) (bool, string) {
					return params.GetString("mode") != "blocked", "owner rejected this call"
				}))
			require.NoError(t, err)
			defer loop.Release()
			owner := aicommon.NewStatefulTaskBase("async-action-owner", "original owner query", ctx, cfg.Emitter, true)
			loop.SetCurrentTask(owner)
			handler, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
			require.NoError(t, err)
			management, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_MANAGE_ASYNC_EXECUTION_RUNTIME)
			require.NoError(t, err)
			await := func(signal <-chan struct{}) {
				t.Helper()
				select {
				case <-signal:
				case <-ctx.Done():
					t.Fatal("action or callback did not reach the expected phase")
				}
			}
			makeAction := func(fields aitool.InvokeParams) *aicommon.Action {
				t.Helper()
				if protocol == "native" {
					// Native transport hands the handler a complete arguments object.
					return aicommon.NewSimpleAction(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL, fields)
				}
				var payload any = fields
				if protocol == "text_next_action" {
					payload = map[string]any{"@action": schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL, "next_action": fields}
				} else {
					fields["@action"] = schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL
				}
				encoded, err := json.Marshal(payload)
				require.NoError(t, err)
				action, err := aicommon.ExtractAction(string(encoded), schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
				require.NoError(t, err)
				return action
			}
			fields := func(mode string, async any) aitool.InvokeParams {
				params := aitool.InvokeParams{"mode": mode, "value": "original value", "async": true}
				call := aitool.InvokeParams{"directly_call_tool_name": tool.Name, "directly_call_tool_params": params,
					"directly_call_reason": "action integration", "identifier": "async_action"}
				if async != nil {
					call["async_directly_call_tool"] = async
				}
				return call
			}
			begin := func(action *aicommon.Action) (*reactloops.LoopActionHandlerOperator, <-chan struct{}) {
				t.Helper()
				require.NoError(t, handler.ActionVerifier(loop, action))
				op := reactloops.NewActionHandlerOperator(owner)
				done := make(chan struct{})
				go func() { handler.ActionHandler(loop, action, op); close(done) }()
				return op, done
			}
			run := func(call aitool.InvokeParams) *reactloops.LoopActionHandlerOperator {
				t.Helper()
				op, done := begin(makeAction(call))
				await(done)
				require.True(t, op.IsContinued())
				return op
			}
			idFrom := func(op *reactloops.LoopActionHandlerOperator) string {
				t.Helper()
				_, tail, ok := strings.Cut(op.GetFeedback().String(), "execute_id: ")
				require.True(t, ok)
				id, err := strconv.Unquote(strings.SplitN(tail, "\n", 2)[0])
				require.NoError(t, err)
				require.NotEmpty(t, id)
				return id
			}
			manage := func(op, id string) map[string]json.RawMessage {
				t.Helper()
				action := aicommon.NewSimpleAction(schema.AI_REACT_LOOP_ACTION_MANAGE_ASYNC_EXECUTION_RUNTIME,
					aitool.InvokeParams{"op": op, "execute_id": id, "timeout_seconds": 1})
				require.NoError(t, management.ActionVerifier(loop, action))
				operator := reactloops.NewActionHandlerOperator(owner)
				management.ActionHandler(loop, action, operator)
				var feedback map[string]json.RawMessage
				require.NoError(t, json.Unmarshal(operator.GetFeedback().Bytes(), &feedback))
				require.NotContains(t, feedback, "error")
				return feedback
			}

			t.Log("1. An omitted scheduling flag stays synchronous, including a business async=true")
			op, done := begin(makeAction(fields("sync", nil)))
			await(started["sync"])
			select {
			case <-done:
				t.Fatal("default synchronous action returned before its callback finished")
			default:
			}
			close(finished["sync"])
			await(done)
			require.Equal(t, 1, op.GetExecutedToolCallCount())
			require.NotContains(t, op.GetFeedback().String(), "execute_id:")
			manager, err := loop.GetAsyncExecutionManager()
			require.NoError(t, err)
			require.Empty(t, manager.List())
			t.Cleanup(func() {
				cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				require.NoError(t, manager.Shutdown(cleanup))
			})

			t.Log("2. Async returns a receipt while the callback is blocked; inspect reads the original caller")
			if protocol == "text_root" {
				// Send the flag AFTER complete params, while the handler is already
				// running. A partial response must not silently become synchronous.
				reader, writer := io.Pipe()
				defer reader.Close()
				defer writer.Close()
				prefixWritten, sendFlag := make(chan struct{}), make(chan struct{})
				writeResult := make(chan error, 1)
				go func() {
					_, err := io.WriteString(writer, `{"@action":"directly_call_tool","directly_call_tool_name":"async_action_fixture","directly_call_tool_params":{"mode":"finish","value":"original value","async":true},"directly_call_reason":"action integration"`)
					close(prefixWritten)
					if err == nil {
						select {
						case <-sendFlag:
							_, err = io.WriteString(writer, `,"async_directly_call_tool":true}`)
						case <-ctx.Done():
							err = ctx.Err()
						}
					}
					writer.CloseWithError(err)
					writeResult <- err
				}()
				action, err := aicommon.ExtractActionFromStream(ctx, reader, schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
				require.NoError(t, err)
				op, done = begin(action)
				await(prefixWritten)
				require.Empty(t, manager.List(), "no submission before the scheduling flag and action are complete")
				close(sendFlag)
				await(done)
				require.NoError(t, <-writeResult)
			} else {
				op = run(fields("finish", true))
			}
			id := idFrom(op)
			require.Zero(t, op.GetExecutedToolCallCount(), "a receipt is not a completed callback")
			await(started["finish"])
			var snapshot asyncexecmanager.Snapshot
			require.NoError(t, json.Unmarshal(manage("inspect", id)["execution"], &snapshot))
			require.Equal(t, asyncexecmanager.StateRunning, snapshot.State)
			require.Equal(t, "original value\n", snapshot.Observation.Stdout)
			require.Equal(t, "warning\n", snapshot.Observation.Stderr)
			require.Empty(t, snapshot.Observation.Result)
			close(finished["finish"])
			var summary asyncexecmanager.Summary
			require.NoError(t, json.Unmarshal(manage("wait", id)["execution"], &summary))
			require.Equal(t, asyncexecmanager.StateCompleted, summary.State)
			require.NoError(t, json.Unmarshal(manage("inspect", id)["execution"], &snapshot))
			var result aitool.ToolResult
			require.NoError(t, json.Unmarshal(snapshot.Observation.Result, &result))
			require.True(t, result.Success)
			require.Contains(t, fmt.Sprint(result.Data), "original owner query")
			require.Len(t, owner.GetAllToolCallResults(), 2, "sync and async both archive to their original owner")

			t.Log("3. A later management action stops a real async tool and joins cleanup")
			id = idFrom(run(fields("cancel", " TRUE ")))
			await(started["cancel"])
			manage("stop", id)
			require.NoError(t, json.Unmarshal(manage("wait", id)["execution"], &summary))
			require.Equal(t, asyncexecmanager.StateCancelled, summary.State)
			require.NoError(t, json.Unmarshal(manage("inspect", id)["execution"], &snapshot))
			require.Contains(t, snapshot.Observation.Stdout, "cleaned\n")

			t.Log("4. Execution failure is retained; admission and parameter errors do not invoke tools")
			id = idFrom(run(fields("fail", true)))
			require.NoError(t, json.Unmarshal(manage("wait", id)["execution"], &summary))
			require.Equal(t, asyncexecmanager.StateFailed, summary.State)
			require.Contains(t, summary.Error, "fixture execution failed")
			before := callbacks.Load()
			op = run(fields("blocked", true))
			require.Contains(t, op.GetFeedback().String(), "owner rejected this call")
			require.NotContains(t, op.GetFeedback().String(), "execute_id:")
			require.Zero(t, op.GetExecutedToolCallCount())
			invalid := fields("invalid", true)
			invalid["directly_call_tool_params"] = aitool.InvokeParams{"mode": "invalid"}
			action := makeAction(invalid)
			if err := handler.ActionVerifier(loop, action); protocol == "native" {
				require.Error(t, err, "native validates before dispatch")
			} else {
				require.NoError(t, err)
				op := reactloops.NewActionHandlerOperator(owner)
				handler.ActionHandler(loop, action, op)
				require.Contains(t, op.GetFeedback().String(), "params invalid")
				require.True(t, op.IsContinued())
			}
			require.Equal(t, before, callbacks.Load())
			require.Len(t, manager.List(), 3)
		})
	}
}
