package reactloops

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

func TestActionExecutionStateBelongsToLoop(t *testing.T) {
	first, second := &ReActLoop{}, &ReActLoop{}
	a := aicommon.NewSimpleAction("same_name", aitool.InvokeParams{"target": "model value"})
	b := aicommon.NewSimpleAction("same_name", aitool.InvokeParams{})
	require.Nil(t, first.GetActionExecutionValue(a, "target"))
	first.SetActionExecutionValue(a, "target", "first invocation")
	first.SetActionExecutionValue(b, "target", "second invocation")
	second.SetActionExecutionValue(a, "target", "other loop")
	require.Equal(t, "first invocation", first.GetActionExecutionValue(a, "target"))
	require.Equal(t, "second invocation", first.GetActionExecutionValue(b, "target"))
	require.Equal(t, "other loop", second.GetActionExecutionValue(a, "target"))
	require.Equal(t, "model value", a.GetParams().GetString("target"))
	require.NotContains(t, a.DumpRawParams(), "invocation")
	first.ClearActionExecutionValues(a)
	require.Len(t, first.actionExecutionValues, 1)
	require.Equal(t, "other loop", second.GetActionExecutionValue(a, "target"))
	first.SetActionExecutionValue(b, "target", nil)
	require.Empty(t, first.actionExecutionValues)
	second.Release()
	require.Empty(t, second.actionExecutionValues)
}

func TestActionExecutionStateClearedAfterCalls(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(fmt.Sprintf("fail=%t", fail), func(t *testing.T) {
			loop, _, task := newActionExecutionTestLoop(t)
			var calls []LoopCall
			for i := 0; i < 2; i++ {
				i := i
				call := actionExecutionTestCall(i, fmt.Sprint(i), "inspect", func(r *ReActLoop, a *aicommon.Action, op *LoopActionHandlerOperator) {
					require.Equal(t, i, r.GetActionExecutionValue(a, "target"))
					if fail {
						require.Zero(t, i, "remaining calls must be skipped")
						op.Fail(fmt.Errorf("execution failed"))
					} else {
						op.Continue()
					}
				})
				loop.SetActionExecutionValue(call.Action, "target", i)
				calls = append(calls, call)
			}
			result := loop.execCalls(calls, 1, task, "prompt", utils.NewOnce(), nil)
			if fail {
				require.Error(t, result.err)
			} else {
				require.NoError(t, result.err)
			}
			require.Empty(t, loop.actionExecutionValues, "executed and skipped calls must both release state")
		})
	}
}

func TestActionExecutionStateClearedAfterRejectedTransaction(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			loop := newCallAILoopTransactionTestLoop(t, native, func(_ *aicommon.AIRequest, cfg *aispec.AIConfig) (*aicommon.AIResponse, error) {
				response := aicommon.NewAIResponse(nil)
				if native {
					cfg.ToolCallCallback([]*aispec.ToolCall{
						{ID: "accepted", Type: "function", Function: aispec.FuncReturn{Name: "inspect", Arguments: `{"reject":false}`}},
						{Index: 1, ID: "rejected", Type: "function", Function: aispec.FuncReturn{Name: "inspect", Arguments: `{"reject":true}`}},
					})
					cfg.FinishReasonCallback("tool_calls", nil)
				} else {
					response.EmitOutputStream(strings.NewReader(`{"@action":"inspect","reject":true}`))
				}
				response.Close()
				return response, nil
			})
			loop.actions.Set("inspect", &LoopAction{ActionType: "inspect", ActionVerifier: func(r *ReActLoop, a *aicommon.Action) error {
				r.SetActionExecutionValue(a, "target", "prepared")
				if a.GetBool("reject") {
					return fmt.Errorf("rejected invocation")
				}
				return nil
			}})
			_, _, _, err := loop.callAILoopTransaction(&sync.WaitGroup{}, "prompt", "nonce", nil,
				func(io.Reader, io.Reader) {}, func(string, string, io.Reader, io.Reader) {})
			require.Error(t, err)
			require.Empty(t, loop.actionExecutionValues, "failure must also release earlier valid calls")
		})
	}
}
