package aireact

import (
	"context"
	"encoding/json"
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

// Tool-use approval can remain pending after async submission. Both a GUI
// response and the configured approval assistant can request direct_answer.
// Interval review is a different contract: continue/detach/cancel, not answer.
func TestAsyncGUIBackgroundReviewDirectAnswer(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, origin := range []string{"gui", "assistant"} {
			t.Run(fmt.Sprintf("native=%t/%s", native, origin), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
				defer cancel()
				events := newGUIReplayEvents()
				review := make(chan string, 1)
				approve := make(chan struct{})
				started, cancelled, cleanup := make(chan struct{}), make(chan struct{}), make(chan struct{})
				var releaseOnce sync.Once
				releaseCleanup := func() { releaseOnce.Do(func() { close(cleanup) }) }
				var invoked, answers, siblingCalls, assistantCalls atomic.Int32
				var siblingCallID string
				sibling, err := aitool.New("running_async_sibling", aitool.WithDangerousNoNeedUserReview(true),
					aitool.WithCallback(func(ctx context.Context, _ aitool.InvokeParams, cfg *aitool.ToolRuntimeConfig, stdout, _ io.Writer) (any, error) {
						siblingCalls.Add(1)
						siblingCallID = cfg.RuntimeID
						fmt.Fprintln(stdout, "SIBLING_STARTED")
						close(started)
						<-ctx.Done()
						close(cancelled)
						<-cleanup
						fmt.Fprintln(stdout, "SIBLING_CLEANUP")
						return nil, ctx.Err()
					}))
				require.NoError(t, err)
				tool, err := aitool.New("reviewed_async_tool", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
					invoked.Add(1)
					return "must not run", nil
				}))
				require.NoError(t, err)
				var runtime *ReAct
				var loop *reactloops.ReActLoop
				index := 0
				answer := "REVIEW_REQUESTED_ANSWER"
				model := func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if req.GetCallerLabel() == "directly-answer" {
						answers.Add(1)
						// Prove review stopped the already-running sibling, before test
						// teardown cancels the runtime. Hold cleanup to observe stopping.
						if err := guiAwait(ctx, cancelled); err != nil {
							return nil, err
						}
						manager, err := loop.GetAsyncExecutionManager()
						if err != nil {
							return nil, err
						}
						entries := manager.List()
						if len(entries) != 2 || entries[0].State != asyncexecmanager.StateStopping {
							return nil, fmt.Errorf("sibling must be stopping until cleanup finishes: %+v", entries)
						}
						for _, e := range events.snapshot() {
							if guiTerminal(e) && (e.CallToolID == siblingCallID || guiData(e)["call_tool_id"] == siblingCallID) {
								return nil, fmt.Errorf("sibling card ended before cleanup: %s", e.Type)
							}
						}
						releaseCleanup()
						return guiRespond(c, req, false, guiReplayStep{"directly_answer", aitool.InvokeParams{"answer_payload": answer}})
					}
					if !aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
						return nil, fmt.Errorf("unexpected model request: %s", req.GetCallerLabel())
					}
					loop = runtime.GetCurrentTask().GetReActLoop().(*reactloops.ReActLoop)
					defer func() { index++ }()
					switch index {
					case 0:
						return guiRespond(c, req, native, guiReplayStep{"require_tool", aitool.InvokeParams{"require_tool_payload": sibling.Name}})
					case 1:
						return guiRespond(c, req, native, guiReplayStep{"directly_call_tool", aitool.InvokeParams{"directly_call_tool_name": sibling.Name, "directly_call_tool_params": aitool.InvokeParams{}, "async_directly_call_tool": true, "directly_call_reason": "start sibling before review"}})
					case 2:
						if err := guiAwait(ctx, started); err != nil {
							return nil, err
						}
						return guiRespond(c, req, native, guiReplayStep{"require_tool", aitool.InvokeParams{"require_tool_payload": tool.Name}})
					case 3:
						return guiRespond(c, req, native, guiReplayStep{"directly_call_tool", aitool.InvokeParams{"directly_call_tool_name": tool.Name, "directly_call_tool_params": aitool.InvokeParams{}, "async_directly_call_tool": true, "directly_call_reason": "background approval"}})
					case 4:
						var reviewID string
						select {
						case reviewID = <-review:
						case <-ctx.Done():
							return nil, ctx.Err()
						}
						if origin == "gui" {
							if err := runtime.SendInputEvent(&ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: reviewID, InteractiveJSONInput: `{"suggestion":"direct_answer"}`}); err != nil {
								return nil, err
							}
						} else {
							close(approve)
						}
						if err := guiAwait(ctx, loop.AsyncDirectAnswerSignal()); err != nil {
							return nil, err
						}
						// The in-flight foreground model response must be discarded.
						return guiRespond(c, req, native, guiReplayStep{"directly_call_tool", aitool.InvokeParams{"directly_call_tool_name": tool.Name, "directly_call_tool_params": aitool.InvokeParams{}, "directly_call_reason": "must be discarded"}})
					default:
						return nil, fmt.Errorf("continued planning after direct_answer")
					}
				}
				opts := []aicommon.ConfigOption{
					aicommon.WithContext(ctx),
					aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools(sibling, tool)),
					aicommon.WithEnableFunctionCallMode(native),
					aicommon.WithWorkdir(t.TempDir()),
					aicommon.WithDisableCreateDBRuntime(true),
					aicommon.WithNoOpMemoryTriage(),
					aicommon.WithDisableToolCallerIntervalReview(true),
					aicommon.WithAICallback(guiMockModel(t, model)),
					aicommon.WithAIAutoRetry(1),
					aicommon.WithAITransactionAutoRetry(1),
					aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
						events.record(e)
						if e.Type == schema.EVENT_TYPE_TOOL_USE_REVIEW_REQUIRE {
							var data map[string]any
							if json.Unmarshal(e.Content, &data) != nil || data["tool"] != tool.Name {
								return
							}
							id, _ := data["id"].(string)
							if id != "" {
								select {
								case review <- id:
								default:
								}
							}
						}
					}),
				}
				if origin == "assistant" {
					opts = append(opts, aicommon.WithAgreeManualCallback(func(ctx context.Context, _ *aicommon.Config) (aitool.InvokeParams, error) {
						assistantCalls.Add(1)
						if err := guiAwait(ctx, approve); err != nil {
							return nil, err
						}
						return aitool.InvokeParams{"suggestion": "direct_answer"}, nil
					}))
				}
				runtime, err = NewTestReAct(opts...)
				require.NoError(t, err)
				defer func() {
					cancel()
					releaseCleanup()
					runtime.WaitLifecycleStopped()
					guiShutdownAsyncLoop(t, loop)
				}()
				runtime.config.SetWorkDir(runtime.config.Workdir)
				require.NoError(t, runtime.SendInputEvent(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "先启动一个后台工具，再异步提交另一个工具。审批要求直接回答时，停止已运行的工具、丢弃前台待执行操作，只回答一次。"}))
				guiWaitCompleted(t, ctx, events)
				require.Equal(t, int32(1), answers.Load())
				require.Equal(t, int32(1), siblingCalls.Load())
				require.Zero(t, invoked.Load())
				if origin == "assistant" {
					require.Equal(t, int32(1), assistantCalls.Load())
				}
				require.NotNil(t, loop)
				manager, err := loop.GetAsyncExecutionManager()
				require.NoError(t, err)
				entries := manager.List()
				require.Len(t, entries, 2)
				for _, entry := range entries {
					final, err := manager.Wait(ctx, entry.ID)
					require.NoError(t, err)
					require.Equal(t, asyncexecmanager.StateCancelled, final.State)
				}
				runtime.Emitter.WaitForStream()
				callID := siblingCallID
				starts, cancels := map[string]int{}, map[string]int{}
				var bubbles int
				var stdout strings.Builder
				for _, e := range events.snapshot() {
					id := e.CallToolID
					if id == "" {
						id, _ = guiData(e)["call_tool_id"].(string)
					}
					switch e.Type {
					case schema.EVENT_TOOL_CALL_START:
						starts[id]++
					case schema.EVENT_TOOL_CALL_USER_CANCEL:
						cancels[id]++
						if id == callID {
							require.Contains(t, stdout.String(), "SIBLING_CLEANUP", "cleanup must reach GUI before terminal card")
						}
					}
					if id == callID && strings.HasSuffix(e.NodeId, "-stdout") {
						stdout.Write(e.StreamDelta)
					}
					require.NotEqual(t, schema.EVENT_TOOL_CALL_ERROR, e.Type)
					require.NotEqual(t, schema.EVENT_TOOL_CALL_DONE, e.Type)
					if e.Type == "result" && strings.Contains(fmt.Sprint(guiData(e)["result"]), answer) {
						bubbles++
					}
				}
				require.Len(t, starts, 2, "discarded foreground response must not create a third card")
				require.Equal(t, starts, cancels)
				for _, count := range starts {
					require.Equal(t, 1, count)
				}
				require.Equal(t, 1, bubbles, "background approval must produce only one answer bubble")
			})
		}
	}
}
