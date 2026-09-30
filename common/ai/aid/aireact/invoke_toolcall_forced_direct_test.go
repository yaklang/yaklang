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

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestForcedDirectToolCallUsesLoopTransaction(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%v", native), func(t *testing.T) {
			var decisions, executions, mutations atomic.Int32
			tool, err := aitool.New("forced_target", aitool.WithDangerousNoNeedUserReview(true),
				aitool.WithStringParam("value", aitool.WithParam_Required(true)),
				aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
					executions.Add(1)
					require.Equal(t, "accepted-mutated", params.GetString("value"))
					return "target result", nil
				}))
			require.NoError(t, err)
			other, err := aitool.New("forced_other", aitool.WithDangerousNoNeedUserReview(true),
				aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
					t.Error("the wrong tool must not execute")
					return nil, nil
				}))
			require.NoError(t, err)
			var react *ReAct
			var parent *reactloops.ReActLoop
			var ownerTask aicommon.AIStatefulTask
			var loopEmitter *aicommon.Emitter
			react, err = NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithTools(tool, other),
				aicommon.WithDisableToolCallerIntervalReview(true), aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					require.Same(t, ownerTask, parent.GetCurrentTask())
					require.Same(t, ownerTask, react.GetCurrentTask())
					require.Same(t, loopEmitter, parent.GetEmitter())
					step := decisions.Add(1)
					prompt := request.GetPrompt()
					require.Contains(t, prompt, "OWNER_INSTRUCTION")
					require.Contains(t, prompt, "the owning task query")
					require.Contains(t, prompt, "CACHE_TOOL_CALL")
					require.Contains(t, prompt, "forced_target")
					tail := strings.LastIndex(prompt, "Call exactly one tool using directly_call_tool.")
					require.Positive(t, tail)
					require.Greater(t, tail, strings.LastIndex(prompt, "<|PROMPT_SECTION_dynamic_END_"))
					require.NotContains(t, prompt, "TEMPORARY_INSTRUCTION")
					projected := aiprojection.ProjectAndObserve("forced-call-test", aiprojection.CreateTemplate(prompt))
					require.NotNil(t, projected)
					require.NotEmpty(t, projected.Messages)
					require.Contains(t, projected.Messages[len(projected.Messages)-1].Content, "Call exactly one tool using directly_call_tool.")
					require.Zero(t, executions.Load())
					require.True(t, react.config.AiToolManager.IsRecentlyUsedTool(tool.Name))
					if step > 1 {
						require.Contains(t, prompt, "No tool was executed")
					}
					actionName := "directly_call_tool"
					arguments := `{"directly_call_tool_name":"forced_target","directly_call_tool_params":{"value":"accepted"}}`
					if step == 1 {
						actionName, arguments = "finish", `{}`
					} else if step == 2 {
						arguments = `{"directly_call_tool_name":"forced_other","directly_call_tool_params":{}}`
					}
					response := config.NewAIResponse()
					options := aispec.NewDefaultAIConfig(request.GetExtraSpecOpts()...)
					if native {
						require.NotNil(t, options.ToolCallCallback)
						require.NotEmpty(t, options.Tools)
						response.EmitOutputStream(strings.NewReader("native content is not an action"))
						midpoint := len(arguments) / 2
						options.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("forced_call_%d", step), Type: "function", Function: aispec.FuncReturn{Name: actionName, Arguments: arguments[:midpoint]}}})
						options.ToolCallCallback([]*aispec.ToolCall{{Function: aispec.FuncReturn{Arguments: arguments[midpoint:]}}})
						options.FinishReasonCallback("tool_calls", []byte(`{"finish_reason":"tool_calls"}`))
					} else {
						require.Nil(t, options.ToolCallCallback)
						var params map[string]any
						require.NoError(t, json.Unmarshal([]byte(arguments), &params))
						if step == 3 {
							params["directly_call_tool_params"] = map[string]any{}
						}
						params["type"] = actionName
						encoded, marshalErr := json.Marshal(map[string]any{"@action": "object", "next_action": params})
						require.NoError(t, marshalErr)
						output := string(encoded)
						if step == 3 {
							output += fmt.Sprintf("\n<|TOOL_PARAM_value_%s|>\naccepted\n<|TOOL_PARAM_value_END_%s|>", aicommon.RecentToolCacheStableNonce, aicommon.RecentToolCacheStableNonce)
						}
						response.EmitOutputStream(strings.NewReader(output))
					}
					response.Close()
					return response, nil
				}))
			require.NoError(t, err)
			parent, err = reactloops.NewReActLoop("default", react, reactloops.WithFunctionCallMode(native),
				reactloops.WithFunctionCallActionVariants(),
				reactloops.WithPersistentContextProvider(func(*reactloops.ReActLoop, string) (string, error) { return "OWNER_INSTRUCTION", nil }),
				reactloops.WithToolInvokeParamsMutator(func(name string, params aitool.InvokeParams) aitool.InvokeParams {
					mutations.Add(1)
					params["value"] = params.GetString("value") + "-mutated"
					return params
				}))
			require.NoError(t, err)
			defer parent.Release()
			parent.RemoveAction("directly_call_tool")
			task := aicommon.NewStatefulTaskBase("forced_owner", "the owning task query", context.Background(), react.Emitter, true)
			ownerTask = task
			parent.SetCurrentTask(task)
			loopEmitter = parent.GetEmitter()
			originalEmitter := task.GetEmitter()
			result, directly, err := react.ExecuteToolRequiredAndCall(context.Background(), tool.Name,
				aicommon.WithToolCaller_Reason("explicit reason"), aicommon.WithToolCaller_CallToolID("forced_execution"))
			require.NoError(t, err)
			require.False(t, directly)
			require.NotNil(t, result)
			require.Equal(t, "forced_execution", result.ToolCallID)
			require.EqualValues(t, 3, decisions.Load())
			require.EqualValues(t, 1, executions.Load())
			require.EqualValues(t, 1, mutations.Load())
			require.Len(t, task.GetAllToolCallResults(), 1)
			require.Same(t, task, parent.GetCurrentTask())
			require.Same(t, task, react.GetCurrentTask())
			require.Same(t, parent, task.GetReActLoop())
			require.Same(t, originalEmitter, task.GetEmitter())
			if native {
				timeline := aicommon.RenderTimelineFrozenOpen(react.config.Timeline)
				require.Contains(t, timeline.Open, "forced_call_1")
				require.Contains(t, timeline.Open, "forced_call_3")
			}
		})
	}
}

func TestForcedDirectToolCallStopsAfterThreeMismatches(t *testing.T) {
	var decisions atomic.Int32
	tool, err := aitool.New("forced_never_called", aitool.WithDangerousNoNeedUserReview(true),
		aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
			t.Error("a finish response must not execute the target tool")
			return nil, nil
		}))
	require.NoError(t, err)
	react, err := NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithTools(tool),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			decisions.Add(1)
			response := config.NewAIResponse()
			response.EmitOutputStream(strings.NewReader(`{"@action":"finish"}`))
			response.Close()
			return response, nil
		}))
	require.NoError(t, err)
	result, directly, err := react.ExecuteToolRequiredAndCall(context.Background(), tool.Name)
	require.ErrorContains(t, err, "after 3 attempts")
	require.Nil(t, result)
	require.False(t, directly)
	require.EqualValues(t, 3, decisions.Load())
}

func TestForcedDirectToolCallCancellationStopsRequest(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%v", native), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			var decisions atomic.Int32
			tool, err := aitool.New("forced_cancelled", aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
				t.Error("cancelled call must not execute")
				return nil, nil
			}))
			require.NoError(t, err)
			react, err := NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithTools(tool),
				aicommon.WithEnableFunctionCallMode(native),
				aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					decisions.Add(1)
					cancel()
					<-request.GetContext().Done()
					return nil, request.GetContext().Err()
				}))
			require.NoError(t, err)
			_, _, err = react.ExecuteToolRequiredAndCall(ctx, tool.Name)
			require.True(t, errors.Is(err, context.Canceled))
			require.EqualValues(t, 1, decisions.Load())
		})
	}
}

func TestForcedDirectToolCallRejectsNativeExtraCalls(t *testing.T) {
	var decisions atomic.Int32
	var tools []*aitool.Tool
	for _, name := range []string{"forced_single", "forced_unrequested"} {
		tool, err := aitool.New(name, aitool.WithDangerousNoNeedUserReview(true),
			aitool.WithSimpleCallback(func(aitool.InvokeParams, io.Writer, io.Writer) (any, error) {
				t.Error("an entire response containing extra calls must be rejected before execution")
				return nil, nil
			}))
		require.NoError(t, err)
		tools = append(tools, tool)
	}
	react, err := NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithTools(tools...),
		aicommon.WithEnableFunctionCallMode(true),
		aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, request *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			step := decisions.Add(1)
			response := config.NewAIResponse()
			options := aispec.NewDefaultAIConfig(request.GetExtraSpecOpts()...)
			var calls []*aispec.ToolCall
			for index, tool := range tools {
				calls = append(calls, &aispec.ToolCall{Index: index, ID: fmt.Sprintf("extra_%d_%d", step, index), Type: "function",
					Function: aispec.FuncReturn{Name: "directly_call_tool", Arguments: fmt.Sprintf(`{"directly_call_tool_name":%q,"directly_call_tool_params":{}}`, tool.Name)}})
			}
			options.ToolCallCallback(calls)
			options.FinishReasonCallback("tool_calls", []byte(`{"finish_reason":"tool_calls"}`))
			response.Close()
			return response, nil
		}))
	require.NoError(t, err)
	_, _, err = react.ExecuteToolRequiredAndCall(context.Background(), tools[0].Name)
	require.ErrorContains(t, err, "after 3 attempts")
	require.EqualValues(t, 3, decisions.Load())
}
