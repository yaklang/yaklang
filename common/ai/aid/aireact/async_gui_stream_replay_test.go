package aireact

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/asyncexecmanager"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Drive real model streaming -> action extraction -> ToolCaller -> manager ->
// GUI events. Complete business params (including a business async=true) precede
// the scheduling flag. A displayed thought is the barrier proving that the
// parser consumed that prefix; no sleeps or assumed token arrival rate are used.
func TestAsyncGUIStreamLateSchedulingFlag(t *testing.T) {
	for _, nested := range []bool{false, true} {
		for _, async := range []bool{true, false} {
			t.Run(fmt.Sprintf("nested=%t/async=%t", nested, async), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				defer cancel()
				events := newGUIReplayEvents()
				reader, writer := io.Pipe()
				stopClose := context.AfterFunc(ctx, func() { reader.CloseWithError(ctx.Err()) })
				defer func() { stopClose(); reader.Close(); writer.Close() }()
				started, finish := make(chan struct{}), make(chan struct{})
				var releaseOnce sync.Once
				release := func() { releaseOnce.Do(func() { close(finish) }) }
				var flagSent atomic.Bool
				var calls atomic.Int32
				tool, err := aitool.New("streamed_async_fixture",
					// Explicit false must override an async default; explicit true
					// must override a sync default. Omitting the tail gives the wrong
					// behavior in both cases.
					aitool.WithDefaultAsync(!async),
					aitool.WithDangerousNoNeedUserReview(true),
					aitool.WithStringParam("value", aitool.WithParam_Required(true)),
					aitool.WithBoolParam("async"),
					aitool.WithCallback(func(ctx context.Context, p aitool.InvokeParams, _ *aitool.ToolRuntimeConfig, stdout, _ io.Writer) (any, error) {
						if !flagSent.Load() {
							t.Error("tool callback started before model sent its scheduling flag")
						}
						if calls.Add(1) != 1 {
							return nil, fmt.Errorf("duplicate streamed invocation")
						}
						if p.GetString("value") != "原始参数" || !p.GetBool("async") {
							return nil, fmt.Errorf("streaming changed business params: %+v", p)
						}
						fmt.Fprintln(stdout, "STREAM_TOOL_STARTED")
						close(started)
						select {
						case <-finish:
							fmt.Fprintln(stdout, "STREAM_TOOL_DONE")
							return "STREAM_RESULT", nil
						case <-ctx.Done():
							return nil, ctx.Err()
						}
					}))
				require.NoError(t, err)
				var runtime *ReAct
				var loop *reactloops.ReActLoop
				index := 0
				const answer = "STREAM_SCENARIO_COMPLETE"
				model := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
						return guiRespond(c, req, false, guiReplayStep{"verify-satisfaction", aitool.InvokeParams{"user_satisfied": false, "reasoning": "Read the actual tool result before answering"}})
					}
					if !aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
						return nil, fmt.Errorf("unexpected model request: %s", req.GetCallerLabel())
					}
					defer func() { index++ }()
					if (async && index == 4) || (!async && index == 3) {
						return guiRespond(c, req, false, guiReplayStep{"finish", aitool.InvokeParams{}})
					}
					switch index {
					case 0:
						return guiRespond(c, req, false, guiReplayStep{"require_tool", aitool.InvokeParams{"require_tool_payload": tool.Name}})
					case 1:
						loop = runtime.GetCurrentTask().GetReActLoop().(*reactloops.ReActLoop)
						rsp := c.NewAIResponse()
						rsp.EmitOutputStream(reader)
						rsp.Close()
						return rsp, nil
					case 2:
						manager, err := loop.GetAsyncExecutionManager()
						if err != nil {
							return nil, err
						}
						entries := manager.List()
						if async {
							// The model has received the receipt while the real callback
							// is still held, proving this was not a premature sync call.
							if err := guiAwait(ctx, started); err != nil {
								return nil, err
							}
							entries = manager.List()
							if len(entries) != 1 || entries[0].State != asyncexecmanager.StateRunning ||
								!strings.Contains(req.GetPrompt(), fmt.Sprintf("execute_id: %q", entries[0].ID)) {
								return nil, fmt.Errorf("missing async model receipt: %+v", entries)
							}
							release()
							final, err := manager.Wait(ctx, entries[0].ID)
							if err != nil || final.State != asyncexecmanager.StateCompleted {
								return nil, fmt.Errorf("streamed execution did not complete: %+v, %v", final, err)
							}
							return guiRespond(c, req, false, guiReplayStep{"manage_async_execution_runtime", aitool.InvokeParams{"op": "inspect", "execute_id": entries[0].ID}})
						}
						if len(entries) != 0 || !strings.Contains(req.GetPrompt(), "STREAM_RESULT") {
							return nil, fmt.Errorf("explicit false did not return the synchronous result: %+v", entries)
						}
					case 3:
						feedback := guiTextManagementFeedback(req.GetPrompt())
						if !async || len(feedback) != 1 || feedback[0]["op"] != "inspect" || !strings.Contains(fmt.Sprint(feedback[0]), "STREAM_RESULT") {
							return nil, fmt.Errorf("inspect result did not reach the model")
						}
					default:
						return nil, fmt.Errorf("unexpected extra model action %d", index)
					}
					return guiRespond(c, req, false, guiReplayStep{"directly_answer", aitool.InvokeParams{"answer_payload": answer}})
				}
				runtime, err = NewTestReAct(
					aicommon.WithContext(ctx), aicommon.WithEnableFunctionCallMode(false),
					aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools(tool)),
					aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true),
					aicommon.WithNoOpMemoryTriage(), aicommon.WithDisableToolCallerIntervalReview(true),
					aicommon.WithAICallback(guiMockModel(t, model)),
					aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
					aicommon.WithEventHandler(events.record))
				require.NoError(t, err)
				defer func() {
					cancel()
					release()
					runtime.WaitLifecycleStopped()
					guiShutdownAsyncLoop(t, loop)
				}()
				runtime.config.SetWorkDir(runtime.config.Workdir)
				require.NoError(t, runtime.SendInputEvent(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "边输出说明边调用 streamed_async_fixture，最后才输出异步调度标志；获取真实结果后回答。"}))

				prefix := `{"@action":"directly_call_tool",`
				if nested {
					prefix = `{"@action":"object","next_action":{"type":"directly_call_tool",`
				}
				prefix += `"directly_call_tool_name":"streamed_async_fixture","directly_call_tool_params":{"value":"原始参数","async":true},"directly_call_reason":"stream fixture","human_readable_thought":"STREAM_PREFIX"`
				_, err = io.WriteString(writer, prefix)
				require.NoError(t, err)
				require.NoError(t, events.wait(ctx, func(all []*ypb.AIOutputEvent) bool {
					var thought strings.Builder
					for _, e := range all {
						if e.NodeId == "re-act-loop-thought" {
							thought.Write(e.StreamDelta)
						}
					}
					return strings.Contains(thought.String(), "STREAM_PREFIX")
				}))
				manager, err := loop.GetAsyncExecutionManager()
				require.NoError(t, err)
				require.Empty(t, manager.List(), "partial action must not submit background work")
				require.Zero(t, calls.Load(), "partial action must not invoke synchronously either")
				for _, e := range events.snapshot() {
					require.NotEqual(t, schema.EVENT_TOOL_CALL_START, e.Type, "UI must not show execution before action is complete")
				}
				flagSent.Store(true)
				tail := fmt.Sprintf(`,"async_directly_call_tool":%t}`, async)
				if nested {
					tail += `}`
				}
				_, err = io.WriteString(writer, tail)
				require.NoError(t, err)
				require.NoError(t, writer.Close())
				require.NoError(t, guiAwait(ctx, started))
				if !async {
					require.Empty(t, manager.List(), "late false must override default async")
					release()
				}
				guiWaitCompleted(t, ctx, events)
				runtime.Emitter.WaitForStream()
				require.Equal(t, int32(1), calls.Load())
				validateGUIReplayCards(t, events.snapshot(), "stream_late_flag", answer)
				var cards int
				for _, e := range events.snapshot() {
					if e.Type == schema.EVENT_TOOL_CALL_START {
						cards++
					}
				}
				require.Equal(t, 1, cards)
			})
		}
	}
}
