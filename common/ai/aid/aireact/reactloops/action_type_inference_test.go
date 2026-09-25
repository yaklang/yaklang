package reactloops

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon/mock"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func parseActionForInference(t *testing.T, raw string) *aicommon.Action {
	t.Helper()
	action, err := aicommon.ExtractActionFromStream(context.Background(), strings.NewReader(raw), "object")
	require.NoError(t, err)
	action.WaitParse(context.Background())
	action.WaitStream(context.Background())
	return action
}

func TestInferActionTypeFromPayload_UsesNestedPayloadWhenTypeMissing(t *testing.T) {
	action := parseActionForInference(t, `{"@action":"object","next_action":{"answer_payload":"hello"}}`)
	require.Equal(t, "directly_answer", inferActionTypeFromPayload(action, ""))
}

func TestInferActionTypeFromPayload_UsesPlanPayloadWhenTypeMissing(t *testing.T) {
	action := parseActionForInference(t, `{"@action":"object","next_action":{"plan_request_payload":"inspect project auth flow"}}`)
	require.Equal(t, "request_plan_and_execution", inferActionTypeFromPayload(action, ""))
}

func TestInferActionTypeFromPayload_UsesFinalAnswerTagAsFallback(t *testing.T) {
	action := aicommon.NewSimpleAction("", nil)
	require.Equal(t, "directly_answer", inferActionTypeFromPayload(action, "## final answer"))
}

func TestInferActionTypeFromPayload_DoesNotPromoteNestedType(t *testing.T) {
	for _, test := range []struct {
		name string
		raw  string
		want string
	}{
		{"plain_action", `{"action":"directly_call_tool","directly_call_tool_calls":[{"tool_name":"dig","params":{"type":"A"}}]}`, "directly_call_tool"},
		{"missing_action", `{"directly_call_tool_calls":[{"tool_name":"dig","params":{"type":"A"}}]}`, "directly_call_tool"},
		{"object_action", `{"@action":"object","directly_call_tool_calls":[{"tool_name":"dig","params":{"type":"A"}}]}`, "directly_call_tool"},
		{"nested_only", `{"params":{"type":"A","next_action":{"type":"finish"}}}`, ""},
		{"legacy_type", `{"params":{"type":"A"},"type":"require_tool"}`, "require_tool"},
		{"legacy_next_action", `{"next_action":{"params":{"type":"A"},"type":"require_tool"}}`, "require_tool"},
	} {
		t.Run(test.name, func(t *testing.T) {
			action, err := aicommon.ExtractActionFromStream(context.Background(), strings.NewReader(test.raw), "object",
				aicommon.WithActionAlias("directly_call_tool", "require_tool"))
			require.NoError(t, err)
			require.NoError(t, action.WaitParseResult(context.Background()))
			require.Equal(t, test.want, inferActionTypeFromPayload(action, ""))
		})
	}
}

func TestCallAITransaction_ActionKeyAliasBatch(t *testing.T) {
	for _, actionName := range []string{"directly_call_tool", "unsupported"} {
		for _, functionCall := range []bool{false, true} {
			mode := "text"
			if functionCall {
				mode = "functioncall"
			}
			t.Run(actionName+"/"+mode, func(t *testing.T) {
				baseConfig := mock.NewMockedAIConfig(context.Background()).(*mock.MockedAIConfig)
				baseConfig.SetConfig("AiTransactionAutoRetry", 1)
				var calls, verifications atomic.Int32
				config := &fcTestConfig{
					MockedAIConfig: baseConfig,
					aiCallback: func(req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						calls.Add(1)
						resp := aicommon.NewAIResponse(baseConfig)
						if functionCall {
							cfg := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
							cfg.ToolCallCallback([]*aispec.ToolCall{{
								Index: 0, ID: "call_batch", Type: "function",
								Function: aispec.FuncReturn{Name: actionName, Arguments: `{"identifier":"initial_recon",
									"directly_call_tool_calls":[
										{"tool_name":"dig","params":{"domain":"example.invalid","type":"A"}},
										{"tool_name":"dig","params":{"domain":"example.invalid","type":"CNAME"}}
									]}`},
							}})
							cfg.FinishReasonCallback("tool_calls", nil)
							resp.EmitOutputStream(strings.NewReader("native function call"))
						} else {
							resp.EmitOutputStream(strings.NewReader(`{"action":"` + actionName + `","identifier":"initial_recon",
							"directly_call_tool_calls":[
								{"tool_name":"dig","params":{"domain":"example.invalid","type":"A"}},
								{"tool_name":"dig","params":{"domain":"example.invalid","type":"CNAME"}}
							]}`))
						}
						resp.Close()
						return resp, nil
					},
				}
				invoker := mock.NewMockInvoker(context.Background())
				invoker.SetConfig(config)
				loop := NewMinimalReActLoop(config, invoker)
				loop.functionCallMode = functionCall
				loop.actions = omap.NewEmptyOrderedMap[string, *LoopAction]()
				loop.actions.Set("directly_call_tool", &LoopAction{
					ActionType: "directly_call_tool",
					ActionVerifier: func(_ *ReActLoop, action *aicommon.Action) error {
						verifications.Add(1)
						batch, exists, err := action.GetCanonicalObjectArray("directly_call_tool_calls")
						require.NoError(t, err)
						require.True(t, exists)
						require.Len(t, batch, 2)
						require.Equal(t, "A", batch[0].GetObject("params").GetString("type"))
						require.Equal(t, "CNAME", batch[1].GetObject("params").GetString("type"))
						return nil
					},
				})
				loopCalls, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "test prompt", "nonce", nil,
					loop.emitLoopGeneralOutput, loop.emitLoopFunctionCallOutput)
				require.EqualValues(t, 1, calls.Load())
				if actionName == "unsupported" {
					require.ErrorContains(t, err, `requested="unsupported"`)
					require.Zero(t, verifications.Load(), "an explicit unknown alias must not trigger legacy inference")
					return
				}
				require.NoError(t, err)
				require.Len(t, loopCalls, 1)
				action, handler := loopCalls[0].Action, loopCalls[0].LoopAction
				require.EqualValues(t, 1, verifications.Load())
				require.Equal(t, "directly_call_tool", action.ActionType())
				require.Equal(t, "directly_call_tool", handler.ActionType)
			})
		}
	}
}

func TestActionTypeResolutionError_ExplainsRequestedAvailableAndReason(t *testing.T) {
	err := actionTypeResolutionError(
		"save_evidence",
		[]string{"finish", "require_tool"},
		"a non-empty @action value did not match",
	)
	require.ErrorContains(t, err, `requested="save_evidence"`)
	require.ErrorContains(t, err, "matcher=exact registered action or alias")
	require.ErrorContains(t, err, "available_actions=[finish require_tool]")
	require.ErrorContains(t, err, "reason=a non-empty @action value did not match")
}

func TestActionTypeResolutionError_DistinguishesMissingAction(t *testing.T) {
	err := actionTypeResolutionError("", []string{"finish"}, "no non-empty @action value was found")
	require.ErrorContains(t, err, `requested="<missing>"`)
	require.ErrorContains(t, err, "reason=no non-empty @action value was found")
}

func TestRequiredRegisteredLoopActionError_ReportsRegistrySnapshot(t *testing.T) {
	_, err := requireRegisteredLoopAction("__missing_action_for_test__", "test capability enabled")
	require.Error(t, err)
	require.ErrorContains(t, err, `requested="__missing_action_for_test__"`)
	require.ErrorContains(t, err, `enabled_by="test capability enabled"`)
	require.ErrorContains(t, err, "registered_actions=")
	require.ErrorContains(t, err, "no action is registered under the requested key")
}
