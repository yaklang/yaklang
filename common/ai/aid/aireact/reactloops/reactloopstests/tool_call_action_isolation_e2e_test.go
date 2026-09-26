package reactloopstests

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
	"github.com/yaklang/yaklang/common/ai/aid/aireact"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// Exercise the real transaction -> verifier -> execCalls -> tool path. Native
// mode returns both calls in one response; text mode retains one streamed
// action per iteration. Neither may regenerate already valid parameters.
func TestReActLoopToolCallActionIsolationE2E(t *testing.T) {
	for _, native := range []bool{true, false} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			var mu sync.Mutex
			var executed []string
			var decisions atomic.Int32
			newTool := func(name, field string) *aitool.Tool {
				tool, err := aitool.New(name,
					aitool.WithDangerousNoNeedUserReview(true),
					aitool.WithStringParam(field, aitool.WithParam_Required(true)),
					aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
						mu.Lock()
						defer mu.Unlock()
						executed = append(executed, name+":"+params.GetString(field))
						return "ok", nil
					}),
				)
				require.NoError(t, err)
				return tool
			}
			reader := newTool("isolation_reader", "file")
			search := newTool("isolation_search", "pattern")
			react, err := aireact.NewTestReAct(
				aicommon.WithWorkdir(t.TempDir()), aicommon.WithTools(reader, search),
				aicommon.WithAgreeYOLO(), aicommon.WithDisableToolCallerIntervalReview(true),
				aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithAICallback(func(config aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
						response := config.NewAIResponse()
						response.EmitOutputStream(strings.NewReader(`{"@action":"verify-satisfaction","user_satisfied":false,"reasoning":"continue the requested tool sequence"}`))
						response.Close()
						return response, nil
					}
					if !aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
						return nil, fmt.Errorf("unexpected extra AI call (including parameter regeneration)")
					}
					step := decisions.Add(1)
					calls := []*aispec.ToolCall{
						{Index: 0, ID: "call_read", Type: "function", Function: aispec.FuncReturn{Name: "directly_call_tool", Arguments: `{"directly_call_tool_name":"isolation_reader","directly_call_tool_params":{"file":"/first"}}`}},
						{Index: 1, ID: "call_search", Type: "function", Function: aispec.FuncReturn{Name: "directly_call_tool", Arguments: `{"directly_call_tool_name":"isolation_search","directly_call_tool_params":{"pattern":"second"}}`}},
					}
					finishStep := int32(3)
					if native {
						finishStep = 2
					}
					if step > finishStep {
						return nil, fmt.Errorf("unexpected decision %d", step)
					}
					if step == finishStep {
						calls = []*aispec.ToolCall{{ID: "call_finish", Type: "function", Function: aispec.FuncReturn{Name: "finish", Arguments: `{}`}}}
					} else if !native {
						calls = calls[step-1 : step]
					}
					response := config.NewAIResponse()
					if native {
						cfg := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
						cfg.ToolCallCallback(calls)
						cfg.FinishReasonCallback("tool_calls", []byte(`{"choices":[{"finish_reason":"tool_calls"}]}`))
					} else {
						var params map[string]any
						if err := json.Unmarshal([]byte(calls[0].Function.Arguments), &params); err != nil {
							return nil, err
						}
						params["@action"] = calls[0].Function.Name
						body, err := json.Marshal(params)
						if err != nil {
							return nil, err
						}
						response.EmitOutputStream(strings.NewReader(string(body)))
					}
					response.Close()
					return response, nil
				}),
			)
			require.NoError(t, err)
			loop, err := reactloops.NewReActLoop("tool-call-action-isolation", react,
				reactloops.WithFunctionCallMode(native), reactloops.WithDisablePeriodicVerification(true),
				reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(4),
			)
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			require.NoError(t, loop.Execute("isolation-task", ctx, "Read first, then search second, then finish."))
			mu.Lock()
			require.Equal(t, []string{"isolation_reader:/first", "isolation_search:second"}, executed)
			mu.Unlock()
			if native {
				require.EqualValues(t, 2, decisions.Load())
			} else {
				require.EqualValues(t, 3, decisions.Load())
			}
		})
	}
}
