package aireact

import (
	"context"
	"encoding/json"
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
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/yakgrpc/ypb"
)

// Tool-use approval can remain pending after async submission. Replay the GUI's
// direct_answer choice while the foreground model is preparing its next action.
// This is distinct from interval review, whose decisions are continue/detach/cancel.
func TestAsyncGUIBackgroundReviewDirectAnswer(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprint(native), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			defer cancel()
			events := newGUIReplayEvents()
			review := make(chan string, 1)
			var invoked, answers atomic.Int32
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
					return guiRespond(c, req, false, guiReplayStep{"directly_answer", aitool.InvokeParams{"answer_payload": answer}})
				}
				if !aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
					return nil, fmt.Errorf("unexpected model request: %s", req.GetCallerLabel())
				}
				loop = runtime.GetCurrentTask().GetReActLoop().(*reactloops.ReActLoop)
				defer func() { index++ }()
				switch index {
				case 0:
					return guiRespond(c, req, native, guiReplayStep{"require_tool", aitool.InvokeParams{"require_tool_payload": tool.Name}})
				case 1:
					return guiRespond(c, req, native, guiReplayStep{"directly_call_tool", aitool.InvokeParams{"directly_call_tool_name": tool.Name, "directly_call_tool_params": aitool.InvokeParams{}, "async_directly_call_tool": true, "directly_call_reason": "background approval"}})
				case 2:
					var reviewID string
					select {
					case reviewID = <-review:
					case <-ctx.Done():
						return nil, ctx.Err()
					}
					if err := runtime.SendInputEvent(&ypb.AIInputEvent{IsInteractiveMessage: true, InteractiveId: reviewID, InteractiveJSONInput: `{"suggestion":"direct_answer"}`}); err != nil {
						return nil, err
					}
					if err := guiAwait(ctx, loop.AsyncDirectAnswerSignal()); err != nil {
						return nil, err
					}
					// A real pending model response must not execute after the review signal.
					return guiRespond(c, req, native, guiReplayStep{"directly_call_tool", aitool.InvokeParams{"directly_call_tool_name": tool.Name, "directly_call_tool_params": aitool.InvokeParams{}, "directly_call_reason": "must be discarded"}})
				default:
					return nil, fmt.Errorf("continued planning after direct_answer")
				}
			}
			runtime, err = NewTestReAct(
				aicommon.WithContext(ctx),
				aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools(tool)),
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
						if json.Unmarshal(e.Content, &data) != nil {
							return
						}
						id, _ := data["id"].(string)
						if id == "" {
							return
						}
						select {
						case review <- id:
						default:
						}
					}
				}))
			require.NoError(t, err)
			defer func() { cancel(); runtime.WaitLifecycleStopped() }()
			runtime.config.SetWorkDir(runtime.config.Workdir)
			require.NoError(t, runtime.SendInputEvent(&ypb.AIInputEvent{IsFreeInput: true, FreeInput: "异步调用 reviewed_async_tool；审批中选择直接回答时停止后续调用并回答。"}))
			guiWaitCompleted(t, ctx, events)
			runtime.Emitter.WaitForStream()
			require.Equal(t, int32(1), answers.Load())
			require.Zero(t, invoked.Load())
			require.NotNil(t, loop)
			// The stopped loop retains its existing manager for inspecting terminal work.
			manager, err := loop.GetAsyncExecutionManager()
			require.NoError(t, err)
			entries := manager.List()
			require.Len(t, entries, 1)
			final, err := manager.Wait(ctx, entries[0].ID)
			require.NoError(t, err)
			require.Equal(t, asyncexecmanager.StateCancelled, final.State)
			var starts, cancels, bubbles int
			for _, e := range events.snapshot() {
				switch e.Type {
				case schema.EVENT_TOOL_CALL_START:
					starts++
				case schema.EVENT_TOOL_CALL_USER_CANCEL:
					cancels++
				}
				require.NotEqual(t, schema.EVENT_TOOL_CALL_ERROR, e.Type)
				if e.Type == "result" && strings.Contains(fmt.Sprint(guiData(e)["result"]), answer) {
					bubbles++
				}
			}
			require.Equal(t, 1, starts)
			require.Equal(t, 1, cancels)
			require.Equal(t, 1, bubbles, "background approval must produce only one answer bubble")
		})
	}
}
