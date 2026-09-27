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
	"github.com/yaklang/yaklang/common/ai/aispec"
)

// Real loop transaction -> direct action -> tool callback -> cache journal in
// both protocols. A second successful call appends REUSE, not another schema.
// Freeze and serialization must retain one aggregate schema and no Open delta.
func TestTimelineToolCacheExecutionBothProtocols(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			var decisions, executions atomic.Int32
			tool, err := aitool.New("cache_execution_probe",
				aitool.WithDangerousNoNeedUserReview(true),
				aitool.WithStringParam("value", aitool.WithParam_Required(true)),
				aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
					executions.Add(1)
					return params.GetString("value"), nil
				}),
			)
			require.NoError(t, err)
			react, err := NewTestReAct(
				aicommon.WithWorkdir(t.TempDir()), aicommon.WithTools(tool),
				aicommon.WithEnableFunctionCallMode(native), aicommon.WithAgreeYOLO(),
				aicommon.WithDisableToolCallerIntervalReview(true),
				aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
						response := cfg.NewAIResponse()
						response.EmitOutputStream(strings.NewReader(`{"@action":"verify-satisfaction","user_satisfied":false,"reasoning":"continue the two-call sequence"}`))
						response.Close()
						return response, nil
					}
					if !aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
						return nil, fmt.Errorf("unexpected auxiliary request")
					}
					step := decisions.Add(1)
					if step > 3 {
						return nil, fmt.Errorf("unexpected decision %d", step)
					}
					name := "directly_call_tool"
					args := `{"directly_call_tool_name":"cache_execution_probe","directly_call_tool_params":{"value":"ok"}}`
					if step == 3 {
						name, args = "finish", `{}`
					}
					response := cfg.NewAIResponse()
					if native {
						options := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
						options.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("call_%d", step), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: args}}})
						options.FinishReasonCallback("tool_calls", []byte(`{"choices":[{"finish_reason":"tool_calls"}]}`))
					} else {
						var action map[string]any
						if err := json.Unmarshal([]byte(args), &action); err != nil {
							return nil, err
						}
						action["@action"] = name
						body, err := json.Marshal(action)
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
			react.config.Timeline.SetTimelineBucketByteSize(-1)
			loop, err := reactloops.NewReActLoop("cache-execution", react,
				reactloops.WithFunctionCallMode(native), reactloops.WithDisablePeriodicVerification(true),
				reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(4))
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			require.NoError(t, loop.Execute("cache-task", ctx, "Call the probe twice, then finish."))
			require.EqualValues(t, 2, executions.Load())
			require.EqualValues(t, 3, decisions.Load())
			tl := react.config.Timeline
			open := aicommon.RenderTimelineFrozenOpen(tl)
			require.Contains(t, open.Open, "[UPSERT] cache_execution_probe")
			require.Contains(t, open.Open, "[REUSE] cache_execution_probe")
			require.Equal(t, 1, strings.Count(open.Open, "Direct Params Schema"))
			require.Empty(t, open.PromotedSemiDynamic1)
			tl.FreezeAll()
			frozen := aicommon.RenderTimelineFrozenOpen(tl)
			require.Empty(t, frozen.Open)
			require.NotContains(t, frozen.Frozen, "Direct Params Schema")
			require.Equal(t, 1, strings.Count(frozen.PromotedSemiDynamic1, "Direct Params Schema"))
			raw, err := aicommon.MarshalTimeline(tl)
			require.NoError(t, err)
			restored, err := aicommon.UnmarshalTimeline(raw)
			require.NoError(t, err)
			require.Equal(t, frozen.PromotedSemiDynamic1, aicommon.RenderTimelineFrozenOpen(restored).PromotedSemiDynamic1)
		})
	}
}
