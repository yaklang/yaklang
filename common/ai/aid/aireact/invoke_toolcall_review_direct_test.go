package aireact

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/segmentio/ksuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestToolCallReviewRequestsDirectProposalBeforeExecution(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, suggestion := range []string{"wrong_tool", "wrong_params"} {
			t.Run(fmt.Sprintf("%s/native=%v", suggestion, native), func(t *testing.T) {
				var executions, requests atomic.Int32
				var approved atomic.Bool
				callback := func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
					require.True(t, approved.Load(), "a new proposal must be approved before execution")
					require.EqualValues(t, 42, params.GetInt("id"))
					executions.Add(1)
					return params.GetInt("id"), nil
				}
				original, err := aitool.New("review_direct_original",
					aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
					aitool.WithSimpleCallback(func(params aitool.InvokeParams, stdout, stderr io.Writer) (any, error) {
						require.Equal(t, "wrong_params", suggestion, "the rejected original tool must not execute")
						return callback(params, stdout, stderr)
					}))
				require.NoError(t, err)
				replacement, err := aitool.New("review_direct_replacement",
					aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
					aitool.WithSimpleCallback(callback))
				require.NoError(t, err)
				target := original
				for _, tool := range []*aitool.Tool{original, replacement} {
					tool.InputSchema.AllOf = []any{map[string]any{
						"type": "object", "additionalProperties": false,
						"properties": map[string]any{
							"id":         map[string]any{"type": "integer"},
							"runtime_id": map[string]any{"type": "string"},
						},
					}}
				}
				if suggestion == "wrong_tool" {
					target = replacement
				}
				recorder := new(batchHardeningReviewRecorder)
				react := newBatchHardeningReplayRuntime(t, "review-direct-"+ksuid.New().String(), 18000,
					func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						if aicommon.IsToolCallReasonLiteForgePrompt(request.GetPrompt()) {
							return batchHardeningAIResponse(config, aicommon.MockedToolCallReasonActionJSON)
						}
						requests.Add(1)
						require.Zero(t, executions.Load())
						require.Contains(t, request.GetPrompt(), "Call exactly one tool using directly_call_tool.")
						require.Contains(t, request.GetPrompt(), "human_review_feedback")
						if suggestion == "wrong_tool" {
							require.Contains(t, request.GetPrompt(), "do not reuse the old tool's parameters")
						}
						if !native {
							return batchHardeningAIResponse(config, fmt.Sprintf(`{"@action":"directly_call_tool","directly_call_tool_name":%q,"directly_call_identifier":"review_generated","directly_call_expectations":"fresh proposal","directly_call_tool_params":{"id":42}}`, target.Name))
						}
						response := config.NewAIResponse()
						options := aispec.NewDefaultAIConfig(request.GetExtraSpecOpts()...)
						options.ToolCallCallback([]*aispec.ToolCall{{ID: "review_proposal", Type: "function",
							Function: aispec.FuncReturn{Name: "directly_call_tool", Arguments: fmt.Sprintf(`{"directly_call_tool_name":%q,"directly_call_identifier":"review_generated","directly_call_expectations":"fresh proposal","directly_call_tool_params":{"id":42}}`, target.Name)}}})
						options.FinishReasonCallback("tool_calls", []byte(`{"finish_reason":"tool_calls"}`))
						response.Close()
						return response, nil
					}, recorder,
					func(index int, material batchHardeningReviewMaterial) string {
						require.Zero(t, executions.Load())
						if index == 0 {
							return fmt.Sprintf(`{"suggestion":%q,"extra_prompt":"human_review_feedback"}`, suggestion)
						}
						require.Equal(t, target.Name, material.Tool)
						require.EqualValues(t, 42, material.Params.GetInt("id"))
						approved.Store(true)
						return `{"suggestion":"continue"}`
					}, original, replacement)
				react.config.EnableFunctionCallMode = native
				react.config.SetConfig("EnableFunctionCallMode", native)
				ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
				defer cancel()
				result, directly, err := react.ExecuteToolRequiredAndCallWithoutRequired(ctx, original.Name, aitool.InvokeParams{"id": 1},
					aicommon.WithToolCaller_Reason("original proposal"),
					aicommon.WithToolCaller_ReviewWrongTool(func(context.Context, *aitool.Tool, string, string) (*aitool.Tool, bool, error) {
						return replacement, false, nil
					}))
				require.NoError(t, err)
				require.False(t, directly)
				require.NotNil(t, result)
				require.Equal(t, target.Name, result.Name)
				require.EqualValues(t, 1, executions.Load())
				require.EqualValues(t, 1, requests.Load())
				require.Len(t, recorder.snapshot(), 2)
				require.Len(t, react.config.DefaultTask.GetAllToolCallResults(), 1)
			})
		}
	}
}

func TestToolCallReviewParameterRequestDoesNotExecuteTool(t *testing.T) {
	tool, err := aitool.New("review_empty_proposal",
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			t.Error("requesting a proposal must not execute a tool")
			return nil, nil
		}))
	require.NoError(t, err)
	react, err := NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithTools(tool),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			require.True(t, strings.HasSuffix(request.GetPrompt(), "review_request_only"))
			return batchHardeningAIResponse(config, `{"@action":"directly_call_tool","directly_call_tool_name":"review_empty_proposal","directly_call_tool_params":{}}`)
		}))
	require.NoError(t, err)
	params, err := react._invokeToolCall_ReviewWrongParamForTask(context.Background(), react.config.DefaultTask, tool, nil, "review_request_only")
	require.NoError(t, err)
	require.Empty(t, params)
	require.Empty(t, react.config.DefaultTask.GetAllToolCallResults())
}

func TestToolCallReviewManualReplacementUsesActualTool(t *testing.T) {
	for _, scenario := range []string{"unchanged_params", "mutator", "guard"} {
		t.Run(scenario, func(t *testing.T) {
			var executions atomic.Int32
			original, err := aitool.New("manual_review_original",
				aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
				aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
					t.Error("a rejected original tool must not execute")
					return nil, nil
				}))
			require.NoError(t, err)
			replacement, err := aitool.New("manual_review_replacement",
				aitool.WithIntegerParam("id", aitool.WithParam_Required(true)),
				aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
					executions.Add(1)
					return params.GetInt("id"), nil
				}))
			require.NoError(t, err)
			mutations := make(map[string]int)
			reviews := new(batchHardeningReviewRecorder)
			react := newBatchHardeningReplayRuntime(t, "manual-review-"+ksuid.New().String(), 19000,
				func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					require.True(t, isToolCallReasonLiteForgePrompt(request.GetPrompt()), "explicit review arguments must not request another proposal")
					return batchHardeningAIResponse(config, mockedToolCallReasonActionJSON)
				}, reviews,
				func(index int, material batchHardeningReviewMaterial) string {
					require.Zero(t, executions.Load())
					if index == 0 {
						return `{"suggestion":"wrong_tool","params":{"id":1}}`
					}
					require.Equal(t, replacement.Name, material.Tool)
					if scenario == "mutator" {
						require.EqualValues(t, 11, material.Params.GetInt("id"))
					}
					return `{"suggestion":"continue"}`
				}, original, replacement)
			loop, err := reactloops.NewReActLoop("manual-review-owner", react,
				reactloops.WithToolInvokeParamsMutator(func(toolName string, params aitool.InvokeParams) aitool.InvokeParams {
					mutations[toolName]++
					if scenario == "mutator" && toolName == replacement.Name {
						params["id"] = params.GetInt("id") + 10
					}
					return params
				}),
				reactloops.WithToolInvokeGuard(func(toolName string, params aitool.InvokeParams) (bool, string) {
					if scenario == "guard" && toolName == replacement.Name && params != nil {
						return false, "manual replacement rejected by guard"
					}
					return true, ""
				}))
			require.NoError(t, err)
			defer loop.Release()
			react.config.DefaultTask.SetReActLoop(loop)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			caller, err := react.newToolCallerForCall(ctx, react.config.DefaultTask, original.Name,
				aicommon.WithToolCaller_Reason("original explicit proposal"),
				aicommon.WithToolCaller_ReviewWrongTool(func(context.Context, *aitool.Tool, string, string) (*aitool.Tool, bool, error) {
					return replacement, false, nil
				}))
			require.NoError(t, err)
			result, directly, err := caller.CallToolWithExistedParams(original, aitool.InvokeParams{"id": 1})
			require.False(t, directly)
			require.Len(t, reviews.snapshot(), 2, "changing tools never inherits approval even with identical arguments")
			require.Equal(t, map[string]int{original.Name: 1, replacement.Name: 1}, mutations)
			if scenario == "guard" {
				require.ErrorContains(t, err, "manual replacement rejected by guard")
				require.Nil(t, result)
				require.Zero(t, executions.Load())
			} else {
				require.NoError(t, err)
				require.NotNil(t, result)
				require.Equal(t, replacement.Name, result.Name)
				require.EqualValues(t, 1, executions.Load())
			}
		})
	}
}

func TestToolCallReviewParameterRequestIncludesToolCache(t *testing.T) {
	for _, frozen := range []bool{false, true} {
		t.Run(fmt.Sprintf("frozen=%v", frozen), func(t *testing.T) {
			tool := aitool.NewWithoutCallback("review_cached_tool", aitool.WithDescription("REVIEW_CACHE_SCHEMA_MARKER"))
			react, err := NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithTools(tool),
				aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					require.Contains(t, request.GetPrompt(), "REVIEW_CACHE_SCHEMA_MARKER")
					require.Contains(t, request.GetPrompt(), "REVIEW_TIMELINE_FACT")
					require.True(t, strings.HasSuffix(request.GetPrompt(), "review_cache_feedback"))
					return batchHardeningAIResponse(config, `{"@action":"directly_call_tool","directly_call_tool_name":"review_cached_tool","directly_call_tool_params":{}}`)
				}))
			require.NoError(t, err)
			react.config.GetTimeline().SetTimelineBucketByteSize(-1)
			require.NotNil(t, react.config.RecordRecentlyUsedTool(tool).Upsert)
			react.AddToTimeline("note", "REVIEW_TIMELINE_FACT")
			if frozen {
				react.config.GetTimeline().FreezeAll()
			}
			params, err := react._invokeToolCall_ReviewWrongParamForTask(context.Background(), react.config.DefaultTask, tool, nil, "review_cache_feedback")
			require.NoError(t, err)
			require.Empty(t, params)
			require.Empty(t, react.config.DefaultTask.GetAllToolCallResults())
		})
	}
}

func TestToolCallParameterRequestPreservesReason(t *testing.T) {
	for _, scenario := range []struct {
		name, reason, thought, expected string
	}{
		{name: "explicit", reason: "call reason", thought: "model thought", expected: "call reason"},
		{name: "fallback", thought: "model thought", expected: "model thought"},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			tool := aitool.NewWithoutCallback("proposal_reason_tool")
			react, err := NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithTools(tool),
				aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					return batchHardeningAIResponse(config, fmt.Sprintf(`{"@action":"directly_call_tool","directly_call_tool_name":%q,"directly_call_tool_params":{},"directly_call_reason":%q,"human_readable_thought":%q}`, tool.Name, scenario.reason, scenario.thought))
				}))
			require.NoError(t, err)
			_, reason, err := react.requestToolCallParamsForTask(context.Background(), react.config.DefaultTask, tool, "")
			require.NoError(t, err)
			require.Equal(t, scenario.expected, reason)
			require.Empty(t, react.config.DefaultTask.GetAllToolCallResults())
		})
	}
}
