package aireact

import (
	"context"
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

// Default native loop: load -> observe schemas -> reuse -> reject invalid
// arguments -> execute single/batch -> answer/finish. Any auxiliary parameter
// generation request is an error, so a hidden legacy fallback cannot pass.
func TestNativeMainLoopLoadSchemasThenDirectExecution(t *testing.T) {
	var decisions, executions atomic.Int32
	var react *ReAct
	var tools []*aitool.Tool
	for _, name := range []string{"schema_probe_a", "schema_probe_b"} {
		tool, err := aitool.New(name, aitool.WithDangerousNoNeedUserReview(true),
			aitool.WithStringParam("value", aitool.WithParam_Required(true)),
			aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
				executions.Add(1)
				return params.GetString("value"), nil
			}))
		require.NoError(t, err)
		tools = append(tools, tool)
	}
	var err error
	react, err = NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithTools(tools...),
		aicommon.WithEnableFunctionCallMode(true), aicommon.WithAgreeYOLO(),
		aicommon.WithDisableToolCallerIntervalReview(true), aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(2),
		aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
			if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
				response := cfg.NewAIResponse()
				response.EmitOutputStream(strings.NewReader(`{"@action":"verify-satisfaction","user_satisfied":false,"reasoning":"continue the probe sequence"}`))
				response.Close()
				return response, nil
			}
			if !aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
				return nil, fmt.Errorf("unexpected auxiliary model request during schema load/direct execution")
			}
			step := decisions.Add(1)
			var calls []*aispec.ToolCall
			add := func(name, args string) {
				calls = append(calls, &aispec.ToolCall{ID: fmt.Sprintf("call_%d_%d", step, len(calls)), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: args}})
			}
			switch step {
			case 1:
				require.Contains(t, req.GetPrompt(), "Load business-tool parameter schemas")
				add("require_tool", `{"tool_require_calls":[{"tool_name":"schema_probe_a"},{"tool_name":"schema_probe_b"}]}`)
			case 2:
				require.Zero(t, executions.Load())
				open := aicommon.RenderTimelineFrozenOpen(react.config.Timeline)
				require.Contains(t, open.Open, "[UPSERT] schema_probe_a")
				require.Equal(t, 2, strings.Count(open.Open, "Direct Params Schema"))
				require.Empty(t, open.PromotedSemiDynamic1)
				add("require_tool", `{"tool_require_payload":"schema_probe_a"}`)
			case 3:
				require.Zero(t, executions.Load())
				require.Contains(t, aicommon.RenderTimelineFrozenOpen(react.config.Timeline).Open, "[REUSE] schema_probe_a")
				add("directly_call_tool", `{"directly_call_tool_name":"schema_probe_a","directly_call_tool_params":{}}`)
			case 4:
				require.Zero(t, executions.Load())
				require.Contains(t, req.GetPrompt(), "never generates parameters")
				add("directly_call_tool", `{"directly_call_tool_name":"schema_probe_a","directly_call_tool_params":{"value":"single"}}`)
			case 5:
				require.EqualValues(t, 1, executions.Load())
				add("directly_call_tool", `{"directly_call_reason":"independent probes","directly_call_tool_calls":[{"tool_name":"schema_probe_a","params":{"value":"batch-a"}},{"tool_name":"schema_probe_b","params":{"value":"batch-b"}}]}`)
			case 6:
				require.EqualValues(t, 3, executions.Load())
				add("directly_answer", `{"answer_payload":"Both schemas loaded; single and batch execution completed."}`)
				add("finish", `{}`)
			default:
				return nil, fmt.Errorf("unexpected main-loop step %d", step)
			}
			response := cfg.NewAIResponse()
			options := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
			options.ToolCallCallback(calls)
			options.FinishReasonCallback("tool_calls", []byte(`{"choices":[{"finish_reason":"tool_calls"}]}`))
			response.Close()
			return response, nil
		}))
	require.NoError(t, err)
	react.config.Timeline.SetTimelineBucketByteSize(-1)
	loop, err := reactloops.CreateLoopByName("default", react,
		reactloops.WithDisablePeriodicVerification(true), reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(8))
	require.NoError(t, err)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	require.NoError(t, loop.Execute("schema-load-probe", ctx, "Load the probe schemas, then execute and report."))
	require.EqualValues(t, 6, decisions.Load())
	require.EqualValues(t, 3, executions.Load())
	// Constructing another loop must not inherit the experiment via shared actions.
	for _, tc := range []struct {
		name   string
		native bool
	}{{"default", false}, {"pe_task", true}} {
		other, err := reactloops.CreateLoopByName(tc.name, react, reactloops.WithFunctionCallMode(tc.native))
		require.NoError(t, err)
		action, err := other.GetActionHandler("require_tool")
		require.NoError(t, err)
		require.Contains(t, action.Description, "生成参数")
	}
}
