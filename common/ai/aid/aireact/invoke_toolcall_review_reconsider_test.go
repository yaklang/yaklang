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
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// Replace the old review-owned reselection/repair/abandon sub-AI scenarios:
// the real owning main/worker loop must see rejection before any corrected call.
func TestToolReviewReturnsToOwningLoopBothProtocols(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, worker := range []bool{false, true} {
			for _, kind := range []string{"wrong_tool", "wrong_params"} {
				t.Run(fmt.Sprintf("native=%t/worker=%t/%s", native, worker, kind), func(t *testing.T) {
					var decisions, executions, auxiliary atomic.Int32
					makeTool := func(name string) *aitool.Tool {
						tool, err := aitool.New(name, aitool.WithStringParam("value", aitool.WithParam_Required(true)),
							aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
								executions.Add(1)
								return params.GetString("value"), nil
							}))
						require.NoError(t, err)
						return tool
					}
					original, replacement := makeTool("review_original"), makeTool("review_replacement")
					reviews := new(batchHardeningReviewRecorder)
					react := newBatchHardeningReplayRuntime(t, fmt.Sprintf("review-owner-%t-%t-%s-%d", native, worker, kind, time.Now().UnixNano()), 17000,
						func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
							if req.GetCallerLabel() == "directly-answer" {
								return batchHardeningAIResponse(config, "The corrected probe completed.")
							}
							if !aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
								auxiliary.Add(1)
								return nil, fmt.Errorf("unexpected auxiliary request: %s", req.GetCallerLabel())
							}
							step := decisions.Add(1)
							name, args := "directly_call_tool", fmt.Sprintf(`{"directly_call_tool_name":%q,"directly_call_tool_params":{"value":"initial"},"directly_call_reason":"probe"}`, original.Name)
							if step == 2 {
								require.Zero(t, executions.Load())
								require.Contains(t, req.GetPrompt(), "tool_review_reconsider")
								require.Contains(t, req.GetPrompt(), "OWNER_FEEDBACK")
								require.NotContains(t, req.GetPrompt(), "[TOOL_PROTOCOL_ERROR]")
								require.NotContains(t, req.GetPrompt(), "Tool Compose")
								require.True(t, config.GetAiToolManager().IsRecentlyUsedTool(original.Name))
								target := original.Name
								if kind == "wrong_tool" {
									target = replacement.Name
									require.True(t, config.GetAiToolManager().IsRecentlyUsedTool(target))
								}
								args = fmt.Sprintf(`{"directly_call_tool_name":%q,"directly_call_tool_params":{"value":"corrected"},"directly_call_reason":"corrected probe"}`, target)
							} else if step == 3 {
								name, args = "finish", `{}`
								if worker {
									name, args = "submit_task_result", `{"summary":"corrected probe completed"}`
								}
							} else if step == 4 && worker {
								name, args = "finish", `{}`
							} else if step > 3 {
								return nil, fmt.Errorf("unexpected decision %d", step)
							}
							response := config.NewAIResponse()
							if native {
								opts := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
								opts.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("call_%d", step), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: args}}})
								opts.FinishReasonCallback("tool_calls", nil)
							} else {
								var params map[string]any
								_ = json.Unmarshal([]byte(args), &params)
								params["@action"] = name
								body, _ := json.Marshal(params)
								response.EmitOutputStream(strings.NewReader(string(body)))
							}
							response.Close()
							return response, nil
						}, reviews, func(index int, material batchHardeningReviewMaterial) string {
							if index == 0 {
								return fmt.Sprintf(`{"suggestion":%q,"suggestion_tool":%q,"extra_prompt":"OWNER_FEEDBACK"}`, kind, replacement.Name)
							}
							return `{"suggestion":"continue"}`
						}, original, replacement)
					require.NoError(t, aicommon.WithEnableFunctionCallMode(native)(react.config))
					react.config.Timeline.SetTimelineBucketByteSize(-1)
					opts := []reactloops.ReActLoopOption{reactloops.WithFunctionCallMode(native), reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(4)}
					var loop *reactloops.ReActLoop
					var err error
					if worker {
						loop, err = coordinator.NewWorkerLoop(react, opts...)
					} else {
						loop, err = reactloops.CreateLoopByName("default", react, opts...)
					}
					require.NoError(t, err)
					_, composeErr := loop.GetActionHandler("tool_compose")
					require.Error(t, composeErr, "removed tool DAG must not be registered in main or worker loops")
					ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
					defer cancel()
					require.NoError(t, loop.Execute("owner-task", ctx, "Execute a probe; honor review feedback."))
					expected := 3
					if worker {
						expected = 4
					}
					require.EqualValues(t, expected, decisions.Load())
					require.EqualValues(t, 1, executions.Load())
					require.Zero(t, auxiliary.Load())
					require.EqualValues(t, 2, reviews.count)
					require.Equal(t, "corrected", reviews.snapshot()[1].Params.GetString("value"))
				})
			}
		}
	}
}

func TestToolReviewReconsiderUsesExplicitOwnerAndHonorsCancellation(t *testing.T) {
	tool := aitool.NewWithoutCallback("review_owner_schema", aitool.WithStringParam("value"))
	candidate := aitool.NewWithoutCallback("review_keyword_schema", aitool.WithDescription("owner_schema_keyword"))
	var auxiliary atomic.Int32
	react, err := NewTestReAct(aicommon.WithTools(tool, candidate), aicommon.WithWorkdir(t.TempDir()), aicommon.WithAICallback(func(aicommon.AICallerConfigIf, *aicommon.AIRequest) (*aicommon.AIResponse, error) {
		auxiliary.Add(1)
		return nil, fmt.Errorf("review must not request auxiliary decisions")
	}))
	require.NoError(t, err)
	owner := aicommon.NewStatefulTaskBase("owner-task", "owner query", context.Background(), react.Emitter, true)
	ownerLoop := reactloops.NewMinimalReActLoop(react.config, react)
	owner.SetReActLoop(ownerLoop)
	decoy := aicommon.NewStatefulTaskBase("decoy-task", "decoy query", context.Background(), react.Emitter, true)
	react.SetCurrentTask(decoy)
	hint := react.reconsiderToolReviewForTask(context.Background(), owner, tool, aitool.InvokeParams{"value": "old"}, aitool.InvokeParams{"suggestion": "wrong_params", "extra_prompt": "use new"})
	require.Contains(t, hint, "owner-task")
	require.NotContains(t, hint, "decoy-task")
	hint = react.reconsiderToolReviewForTask(context.Background(), owner, tool, nil, aitool.InvokeParams{"suggestion": " WRONG_TOOL ", "suggestion_tool_keyword": "OWNER_SCHEMA_KEYWORD"})
	require.Contains(t, hint, candidate.Name)
	require.True(t, react.config.GetAiToolManager().IsRecentlyUsedTool(candidate.Name))
	require.Zero(t, auxiliary.Load())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	caller, err := react.newToolCallerForCall(ctx, owner, tool.Name)
	require.NoError(t, err)
	_, _, callErr := caller.CallToolWithExistedParams(tool, aitool.InvokeParams{"value": "old"})
	require.ErrorIs(t, callErr, context.Canceled)
}
