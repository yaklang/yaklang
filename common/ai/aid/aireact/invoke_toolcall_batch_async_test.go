package aireact

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

// Same real batch scenario for each action protocol: all sync -> mixed -> all
// async -> partial admission -> execution failure -> owner cancellation.
func TestDirectToolBatchAsyncIntegration(t *testing.T) {
	for _, protocol := range []string{"text_root", "text_next_action", "native"} {
		t.Run(protocol, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			ownerCtx, cancelOwner := context.WithCancel(ctx)
			defer cancelOwner()
			var started, finish [16]chan struct{}
			for i := range started {
				started[i], finish[i] = make(chan struct{}), make(chan struct{})
			}
			tool, err := aitool.New("mixed_batch_fixture",
				aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
				aitool.WithStringParam("mode", aitool.WithParam_Required(true)),
				aitool.WithBoolParam("async"),
				aitool.WithDangerousNoNeedUserReview(true),
				aitool.WithCallback(func(ctx context.Context, params aitool.InvokeParams, runtime *aitool.ToolRuntimeConfig, stdout, stderr io.Writer) (any, error) {
					id := params.GetInt("id")
					_, _ = fmt.Fprintf(stdout, "job_%d\n", id)
					_, _ = fmt.Fprintln(stderr, "warning")
					close(started[id])
					if params.GetString("mode") == "fail" {
						return nil, errors.New("fixture execution failed")
					}
					if params.GetString("mode") == "hold" {
						select {
						case <-finish[id]:
						case <-ctx.Done():
							_, _ = fmt.Fprintln(stdout, "cleaned")
							return nil, ctx.Err()
						}
					}
					return map[string]any{"id": id, "owner": runtime.CurrentTaskUserInput}, nil
				}),
			)
			require.NoError(t, err)
			cfg := aicommon.NewConfig(ctx, aicommon.WithTools(tool), aicommon.WithAgreeYOLO(),
				aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithDisableAutoSkills(true), aicommon.WithDisallowMCPServers(true),
				aicommon.WithNoOpMemoryTriage(), aicommon.WithDisableToolCallerIntervalReview(true),
				aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					return nil, errors.New("batch integration must not call AI")
				}))
			runtime := &ReAct{config: cfg, Emitter: cfg.Emitter}
			loop, err := reactloops.NewReActLoop("mixed-batch-test", runtime,
				reactloops.WithFunctionCallMode(protocol == "native"), reactloops.WithFunctionCallActionVariants(),
				reactloops.WithAllowAIForge(false), reactloops.WithAllowRAG(false), reactloops.WithAllowPlanAndExec(false),
				reactloops.WithAllowUserInteract(false), reactloops.WithDisableLoopPerception(true),
				reactloops.WithDisablePeriodicVerification(true),
				reactloops.WithToolInvokeGuard(func(_ string, params aitool.InvokeParams) (bool, string) {
					return params.GetInt("id") != 9, "owner rejected job_9"
				}))
			require.NoError(t, err)
			defer loop.Release()
			owner := aicommon.NewStatefulTaskBase("mixed-batch-owner", "original owner query", ownerCtx, cfg.Emitter, true)
			loop.SetCurrentTask(owner)
			handler, err := loop.GetActionHandler(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
			require.NoError(t, err)
			await := func(signal <-chan struct{}) {
				t.Helper()
				select {
				case <-signal:
				case <-ctx.Done():
					t.Fatal("batch did not reach the expected phase")
				}
			}
			item := func(id int, mode string, async any) map[string]any {
				call := map[string]any{"tool_name": tool.Name, "identifier": fmt.Sprintf("job_%d", id), "reason": "batch integration",
					"params": map[string]any{"id": id, "mode": mode, "async": true}}
				if async != nil {
					call["async"] = async
				}
				return call
			}
			begin := func(items ...map[string]any) (*reactloops.LoopActionHandlerOperator, <-chan struct{}) {
				t.Helper()
				// Top-level scalar and nested business flags do not affect items.
				fields := aitool.InvokeParams{"directly_call_tool_params_group": items, "async_directly_call_tool": true}
				var action *aicommon.Action
				if protocol == "native" {
					action = aicommon.NewSimpleAction(schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL, fields)
				} else {
					var payload any = fields
					if protocol == "text_next_action" {
						payload = map[string]any{"@action": schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL, "next_action": fields}
					} else {
						fields["@action"] = schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL
					}
					encoded, err := json.Marshal(payload)
					require.NoError(t, err)
					action, err = aicommon.ExtractAction(string(encoded), schema.AI_REACT_LOOP_ACTION_DIRECTLY_CALL_TOOL)
					require.NoError(t, err)
				}
				require.NoError(t, handler.ActionVerifier(loop, action))
				op := reactloops.NewActionHandlerOperator(owner)
				done := make(chan struct{})
				go func() { handler.ActionHandler(loop, action, op); close(done) }()
				return op, done
			}
			run := func(items ...map[string]any) *reactloops.LoopActionHandlerOperator {
				t.Helper()
				op, done := begin(items...)
				await(done)
				require.True(t, op.IsContinued())
				return op
			}
			t.Log("1. All-sync keeps the existing path, despite top-level and business async=true")
			op := run(item(0, "instant", nil), item(1, "instant", false))
			require.Equal(t, 2, op.GetExecutedToolCallCount())
			require.Contains(t, op.GetFeedback().String(), "Tool batch settled: 2 calls")
			require.NotContains(t, op.GetFeedback().String(), "execute_id:")
			manager, err := loop.GetAsyncExecutionManager()
			require.NoError(t, err)
			require.Empty(t, manager.List())
			t.Cleanup(func() {
				cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				require.NoError(t, manager.Shutdown(cleanup))
			})
			find := func(id int) asyncexecmanager.Summary {
				t.Helper()
				for _, summary := range manager.List() {
					if summary.Metadata.Attributes["identifier"] == fmt.Sprintf("job_%d", id) {
						return summary
					}
				}
				t.Fatalf("no receipt for job_%d", id)
				return asyncexecmanager.Summary{}
			}
			wait := func(id int, state asyncexecmanager.State) {
				t.Helper()
				summary, err := manager.Wait(ctx, find(id).ID)
				require.NoError(t, err)
				require.Equal(t, state, summary.State)
			}

			t.Log("2. Mixed batch waits only for sync; even at concurrency=1, async cannot starve sync")
			cfg.SetConfig(aicommon.ConfigKeyToolBatchInvokeConcurrency, 1)
			op, done := begin(item(2, "hold", true), item(3, "hold", nil), item(4, "hold", " TRUE "))
			await(started[3])
			select {
			case <-done:
				t.Fatal("batch returned before its synchronous callback settled")
			default:
			}
			select {
			case <-started[2]:
				t.Fatal("async took the only slot before the sync sibling")
			default:
			}
			close(finish[3])
			await(done)
			require.Equal(t, 1, op.GetExecutedToolCallCount())
			require.Contains(t, op.GetFeedback().String(), "async_submitted=2")
			require.Equal(t, 2, strings.Count(op.GetFeedback().String(), "execute_id:"))
			require.NotEqual(t, find(2).ID, find(4).ID)
			require.Equal(t, "0", find(2).Metadata.Attributes["call_index"])
			require.Equal(t, "2", find(4).Metadata.Attributes["call_index"])
			first := 2
			select {
			case <-started[2]:
			case <-started[4]:
				first = 4
			case <-ctx.Done():
				t.Fatal("no asynchronous callback started")
			}
			snapshot, err := manager.Inspect(ctx, find(first).ID)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf("job_%d\n", first), snapshot.Observation.Stdout)
			require.Equal(t, "warning\n", snapshot.Observation.Stderr)
			_, err = manager.Stop(find(first).ID)
			require.NoError(t, err)
			wait(first, asyncexecmanager.StateCancelled)
			second := 6 - first
			await(started[second])
			close(finish[second])
			wait(second, asyncexecmanager.StateCompleted)
			snapshot, err = manager.Inspect(ctx, find(second).ID)
			require.NoError(t, err)
			var result aitool.ToolResult
			require.NoError(t, json.Unmarshal(snapshot.Observation.Result, &result))
			require.True(t, result.Success)
			require.Contains(t, fmt.Sprint(result.Data), "original owner query")
			require.Contains(t, fmt.Sprint(result.Data), fmt.Sprint(second))
			require.Contains(t, cfg.Timeline.Dump(), "original owner query")

			t.Log("3. All-async returns only receipts and leaves tools alive after the action returns")
			cfg.SetConfig(aicommon.ConfigKeyToolBatchInvokeConcurrency, 3)
			op = run(item(5, "hold", true), item(6, "hold", true))
			require.Zero(t, op.GetExecutedToolCallCount())
			await(started[5])
			await(started[6])
			run(item(7, "hold", true), item(8, "hold", true))
			t.Log("4. Capacity and guard rejection affect only their items; the sync sibling still runs")
			op = run(item(9, "instant", true), item(10, "instant", true), item(11, "instant", false))
			require.Equal(t, 1, op.GetExecutedToolCallCount(), op.GetFeedback().String())
			require.Contains(t, op.GetFeedback().String(), "owner rejected job_9")
			require.Contains(t, op.GetFeedback().String(), asyncexecmanager.ErrCapacity.Error())
			require.NotContains(t, op.GetFeedback().String(), "execute_id:")
			for _, id := range []int{9, 10} {
				select {
				case <-started[id]:
					t.Fatalf("rejected job_%d was invoked", id)
				default:
				}
			}
			manager.StopAll()
			for _, id := range []int{5, 6, 7, 8} {
				wait(id, asyncexecmanager.StateCancelled)
			}

			t.Log("5. Async execution failure is visible via the original Inspector and manager")
			op = run(item(12, "fail", true), item(13, "instant", false))
			require.Equal(t, 1, op.GetExecutedToolCallCount())
			wait(12, asyncexecmanager.StateFailed)
			require.Contains(t, find(12).Error, "fixture execution failed")
			t.Log("6. Owner cancellation reaches every background child and waits for cleanup")
			op = run(item(14, "hold", true), item(15, "hold", true))
			require.Zero(t, op.GetExecutedToolCallCount())
			await(started[14])
			await(started[15])
			cancelOwner()
			for _, id := range []int{14, 15} {
				wait(id, asyncexecmanager.StateCancelled)
				snapshot, err := manager.Inspect(ctx, find(id).ID)
				require.NoError(t, err)
				require.Contains(t, snapshot.Observation.Stdout, "cleaned\n")
			}
		})
	}
	t.Run("async_review_barrier", func(t *testing.T) {
		t.Log("7. Background direct_answer still cancels siblings before any callback starts")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var callbacks atomic.Int32
		tool, err := aitool.New("async_batch_review_fixture",
			aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
				callbacks.Add(1)
				return "must not invoke", nil
			}))
		require.NoError(t, err)
		recorder := new(batchHardeningReviewRecorder)
		runtime := newBatchHardeningReplayRuntime(t, fmt.Sprintf("async-review-%d", time.Now().UnixNano()), 0, nil, recorder,
			func(int, batchHardeningReviewMaterial) string { return `{"suggestion":"direct_answer"}` }, tool)
		loop := reactloops.NewMinimalReActLoop(runtime.config, runtime)
		defer loop.Release()
		owner := aicommon.NewStatefulTaskBase("async-review-owner", "review barrier", ctx, runtime.config.Emitter, true)
		loop.SetCurrentTask(owner)
		result, err := runtime.ExecuteToolCallGroup(ctx, owner, &aicommon.ToolCallGroupRequest{Calls: []aicommon.ToolCallGroupCall{
			{ToolName: tool.Name, Params: aitool.InvokeParams{}, Reason: "review first", Async: true},
			{ToolName: tool.Name, Params: aitool.InvokeParams{}, Reason: "review sibling", Async: true},
		}})
		require.NoError(t, err)
		manager, err := loop.GetAsyncExecutionManager()
		require.NoError(t, err)
		defer func() {
			cleanup, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			require.NoError(t, manager.Shutdown(cleanup))
		}()
		for _, outcome := range result.Outcomes {
			require.Equal(t, aicommon.ToolCallStageSubmitted, outcome.Stage)
			require.NotEmpty(t, outcome.ExecuteID)
			summary, err := manager.Wait(ctx, outcome.ExecuteID)
			require.NoError(t, err)
			require.Equal(t, asyncexecmanager.StateCancelled, summary.State)
			require.Equal(t, aicommon.ToolCallStageSubmitted, outcome.Stage, "the returned receipt remains immutable")
		}
		require.EqualValues(t, 1, atomic.LoadInt32(&recorder.count))
		require.Zero(t, callbacks.Load())
	})
}
