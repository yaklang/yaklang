package reactloops

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

const focusActionValidationScript = `
__ACTIONS__ = [
    {
        "type": "submit_judgement",
        "options": [
            {"name":"verdict", "type":"string", "required":true, "enum":["valid", "false_positive"]},
            {"name":"reason", "type":"string", "required":true, "min_length":1},
            {"name":"confidence", "type":"number", "required":true, "min":0, "max":1},
            {"name":"confirmed", "type":"boolean", "required":true},
            {"name":"optional_unknown", "type":"future_type"},
        ],
        "verifier": func(loop, action) {
            loop.Set("verified", loop.GetInt("verified") + 1)
            if action.GetString("reason") == "reject" { return "custom rejection" }
            return nil
        },
        "handler": func(loop, action, operator) {
            loop.Set("handled", loop.GetInt("handled") + 1)
            loop.Set("accepted_reason", action.GetString("reason"))
        },
    },
    {
        "type": "without_verifier",
        "options": [{"name":"token", "type":"string", "required":true}],
        "handler": func(loop, action, operator) {},
    },
]
`

func applyFocusActionValidationScript(t *testing.T, loop *ReActLoop, code string) {
	t.Helper()
	caller, err := NewFocusModeYakHookCaller("validation.ai-focus.yak", code)
	require.NoError(t, err)
	t.Cleanup(caller.Close)
	for _, opt := range CollectFocusModeActionOptions(caller, nil) {
		opt(loop)
	}
}

func validFocusJudgementParams() aitool.InvokeParams {
	return aitool.InvokeParams{"verdict": "valid", "reason": "evidence", "confidence": float64(0), "confirmed": false}
}

func TestFocusActionSchemaValidation(t *testing.T) {
	for _, textMode := range []bool{false, true} {
		t.Run(fmt.Sprintf("text=%t", textMode), func(t *testing.T) {
			loop := newMinimalReActLoopForOptionTest()
			applyFocusActionValidationScript(t, loop, focusActionValidationScript)
			registered, ok := loop.actions.Get("submit_judgement")
			require.True(t, ok)
			for _, tc := range []struct {
				name  string
				field string
				value any
			}{
				{"missing_verdict", "verdict", nil},
				{"missing_reason", "reason", nil},
				{"missing_confidence", "confidence", nil},
				{"missing_boolean", "confirmed", nil},
				{"empty_verdict", "verdict", ""},
				{"invalid_verdict", "verdict", "maybe"},
				{"verdict_type", "verdict", 1},
				{"empty_reason", "reason", ""},
				{"reason_type", "reason", []any{"evidence"}},
				{"negative_confidence", "confidence", -0.1},
				{"high_confidence", "confidence", 1.1},
				{"confidence_type", "confidence", "0.5"},
				{"boolean_type", "confirmed", "false"},
				{"optional_unknown_type_fallback", "optional_unknown", 10},
			} {
				t.Run(tc.name, func(t *testing.T) {
					params := validFocusJudgementParams()
					if tc.value == nil {
						delete(params, tc.field)
					} else {
						params[tc.field] = tc.value
					}
					action := aicommon.NewSimpleAction("submit_judgement", params)
					if textMode {
						params["@action"] = "submit_judgement"
						encoded, err := json.Marshal(params)
						require.NoError(t, err)
						action, err = aicommon.ExtractAction(string(encoded), "submit_judgement")
						require.NoError(t, err)
					}
					loop.Set("verified", 0)
					err := registered.ActionVerifier(loop, action)
					require.ErrorContains(t, err, tc.field)
					require.ErrorContains(t, err, "submit_judgement")
					require.Zero(t, loop.GetInt("verified"), "schema must reject before custom verifier")
				})
			}

			params := validFocusJudgementParams()
			// Unknown properties remain allowed; a real params field must not
			// replace the canonical action object during validation.
			params["params"] = map[string]any{"verdict": "invalid"}
			params["optional_unknown"] = "compatible string fallback"
			loop.Set("verified", 0)
			action := aicommon.NewSimpleAction("submit_judgement", params)
			require.NoError(t, registered.ActionVerifier(loop, action))
			require.Equal(t, 1, loop.GetInt("verified"))
			require.Equal(t, float64(0), action.GetParams()["confidence"])
			require.Equal(t, false, action.GetParams()["confirmed"])
			require.ErrorContains(t, registered.ActionVerifier(loop, aicommon.NewSimpleAction("submit_judgement",
				aitool.InvokeParams{"params": validFocusJudgementParams()})), "verdict")
			params["reason"] = "reject"
			require.ErrorContains(t, registered.ActionVerifier(loop, aicommon.NewSimpleAction("submit_judgement", params)), "custom rejection")
		})
	}
}

func TestFocusActionSchemaWithoutCustomVerifier(t *testing.T) {
	loop := newMinimalReActLoopForOptionTest()
	applyFocusActionValidationScript(t, loop, focusActionValidationScript)
	action, ok := loop.actions.Get("without_verifier")
	require.True(t, ok)
	require.NotNil(t, action.ActionVerifier)
	require.ErrorContains(t, action.ActionVerifier(loop, aicommon.NewSimpleAction("without_verifier", nil)), "token")
	// Only the selected action's required fields apply.
	require.NoError(t, action.ActionVerifier(loop, aicommon.NewSimpleAction("without_verifier", aitool.InvokeParams{"token": "ok"})))
}

func TestFocusActionSchemaDuplicateAndOverride(t *testing.T) {
	const original = `
__ACTIONS__ = [
    {"type":"same", "options":[{"name":"original", "required":true}], "handler":func(loop, action, op) {}},
    {"type":"same", "options":[{"name":"duplicate", "required":true}], "handler":func(loop, action, op) {}},
]
`
	for _, override := range []bool{false, true} {
		t.Run(fmt.Sprintf("override=%t", override), func(t *testing.T) {
			code, selected := original, "original"
			if override {
				selected = "replacement"
				code += `
__OVERRIDE_ACTIONS__ = [
    {"type":"same", "options":[{"name":"replacement", "type":"integer", "required":true, "max":0}],
     "verifier":func(loop, action) { loop.Set("verified", true); return nil },
     "handler":func(loop, action, op) { loop.Set("handled", true) }},
]
`
			}
			loop := newMinimalReActLoopForOptionTest()
			applyFocusActionValidationScript(t, loop, code)
			action, ok := loop.actions.Get("same")
			require.True(t, ok)
			require.NotNil(t, action.ActionVerifier)
			require.ErrorContains(t, action.ActionVerifier(loop, aicommon.NewSimpleAction("same", nil)), selected)
			require.NotEqual(t, true, loop.GetVariable("verified"))
			params := aitool.InvokeParams{selected: "ok"}
			if override {
				params[selected] = 0
				require.Error(t, action.ActionVerifier(loop, aicommon.NewSimpleAction("same", aitool.InvokeParams{selected: 1})))
				require.Error(t, action.ActionVerifier(loop, aicommon.NewSimpleAction("same", aitool.InvokeParams{selected: -0.5})))
			}
			accepted := aicommon.NewSimpleAction("same", params)
			require.NoError(t, action.ActionVerifier(loop, accepted))
			if override {
				require.Equal(t, true, loop.GetVariable("verified"))
				op := NewActionHandlerOperator(nil)
				action.ActionHandler(loop, accepted, op)
				require.Equal(t, true, loop.GetVariable("handled"))
			}
		})
	}
}

func TestFocusActionSchemaTransactionCorrection(t *testing.T) {
	for _, functionMode := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%t", functionMode), func(t *testing.T) {
			var attempts int
			loop := newCallAILoopTransactionTestLoop(t, functionMode, func(req *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
				attempts++
				params := validFocusJudgementParams()
				if attempts == 1 {
					delete(params, "verdict")
				} else {
					require.Contains(t, req.GetPrompt(), "parameter validation failed")
					require.Contains(t, req.GetPrompt(), "verdict")
				}
				resp := aicommon.NewAIResponse(nil)
				if !functionMode {
					params["@action"] = "submit_judgement"
				}
				encoded, err := json.Marshal(params)
				require.NoError(t, err)
				if functionMode {
					cfg.ToolCallCallback([]*aispec.ToolCall{{Index: 0, ID: fmt.Sprintf("attempt_%d", attempts), Type: "function",
						Function: aispec.FuncReturn{Name: "submit_judgement", Arguments: string(encoded)}}})
					cfg.FinishReasonCallback("tool_calls", nil)
				} else {
					resp.EmitOutputStream(strings.NewReader(string(encoded)))
					cfg.FinishReasonCallback("stop", nil)
				}
				resp.Close()
				return resp, nil
			})
			loop.config.(*fcTestConfig).SetConfig("AiTransactionAutoRetry", 2)
			applyFocusActionValidationScript(t, loop, focusActionValidationScript)
			drain := func(a, b io.Reader) {
				var wg sync.WaitGroup
				wg.Add(2)
				go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, a) }()
				go func() { defer wg.Done(); _, _ = io.Copy(io.Discard, b) }()
				wg.Wait()
			}
			calls, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "judge risk", "nonce", nil,
				drain, func(_, _ string, a, b io.Reader) { drain(a, b) })
			require.NoError(t, err)
			require.Equal(t, 2, attempts)
			require.Equal(t, 1, loop.GetInt("verified"))
			require.Zero(t, loop.GetInt("handled"))
			require.Len(t, calls, 1)
			require.Equal(t, "valid", calls[0].Action.GetString("verdict"))
			op := NewActionHandlerOperator(nil)
			calls[0].LoopAction.ActionHandler(loop, calls[0].Action, op)
			require.Equal(t, 1, loop.GetInt("handled"))
			require.Equal(t, "evidence", loop.Get("accepted_reason"))
			terminated, err := op.IsTerminated()
			require.False(t, terminated)
			require.NoError(t, err)
		})
	}
}
