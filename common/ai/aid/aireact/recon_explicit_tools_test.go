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
	"github.com/yaklang/yaklang/common/ai/aid/aitool/buildinaitools"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func TestReconPreloadsAndExplicitlyCallsSixToolsBothProtocols(t *testing.T) {
	names := []string{"scan_port", "simple_crawler", "banner_grab", "dig", "subdomain_scan", "network_space_search"}
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			var decisions, executions atomic.Int32
			var tools []*aitool.Tool
			for _, name := range names {
				tool, err := aitool.New(name, aitool.WithDangerousNoNeedUserReview(true), aitool.WithStringParam("target", aitool.WithParam_Required(true)), aitool.WithUsage("[[- if .FunctionCallMode -]]NATIVE_ARGUMENTS[[- else -]]TEXT_ACTION[[- end -]]"), aitool.WithSimpleCallback(func(params aitool.InvokeParams, _, _ io.Writer) (any, error) {
					executions.Add(1)
					require.Equal(t, "127.0.0.1", params.GetString("target"))
					return "recon_probe_result", nil
				}))
				require.NoError(t, err)
				tools = append(tools, tool)
			}
			react, err := NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithAiToolManagerOptions(buildinaitools.WithOnlyTools(tools...)), aicommon.WithEnableFunctionCallMode(native), aicommon.WithDisableToolCallerIntervalReview(true),
				aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if req.GetCallerLabel() == "directly-answer" {
						return batchHardeningAIResponse(cfg, "Recon complete.")
					}
					if !aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
						return nil, fmt.Errorf("unexpected recon auxiliary request: %s", req.GetCallerLabel())
					}
					step := int(decisions.Add(1))
					name, args := "finish", `{}`
					if step <= len(names) {
						if step == 1 {
							require.Zero(t, executions.Load())
							for _, tool := range names {
								require.True(t, cfg.GetAiToolManager().IsRecentlyUsedTool(tool))
							}
						}
						marker, absent := "TEXT_ACTION", "NATIVE_ARGUMENTS"
						if native {
							marker, absent = absent, marker
						}
						require.Contains(t, req.GetPrompt(), marker)
						require.NotContains(t, req.GetPrompt(), absent)
						name = "directly_call_tool"
						args = fmt.Sprintf(`{"directly_call_tool_name":%q,"directly_call_tool_params":{"target":"127.0.0.1"},"directly_call_reason":"local recon probe"}`, names[step-1])
					}
					if step == len(names)+1 {
						name, args = "save_evidence", `{"evidence_id":"recon_probe","evidence_content":"Six explicit local recon probes completed with recon_probe_result."}`
						if !native {
							args = `{"evidence_id":"recon_probe","evidence_content":"Six explicit local recon probes completed with recon_probe_result.","todo_delta":{"add":[{"id":"recon_probe","text":"Verify six local probes"}],"close":[{"id":"recon_probe","outcome":"resolved","reason":"All six explicit probes returned recon_probe_result.","refs":["recon_probe"]}]}}`
						}
					}
					if native && step == len(names)+2 {
						name, args = "adjust_todolist", `{"todo_delta":{"add":[{"id":"recon_probe","text":"Verify six local probes"}],"close":[{"id":"recon_probe","outcome":"resolved","reason":"All six explicit probes returned recon_probe_result.","refs":["recon_probe"]}]}}`
					}
					rsp := cfg.NewAIResponse()
					if native {
						opts := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
						opts.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("recon_%d", step), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: args}}})
						opts.FinishReasonCallback("tool_calls", nil)
					} else {
						var params map[string]any
						_ = json.Unmarshal([]byte(args), &params)
						params["@action"] = name
						body, _ := json.Marshal(params)
						rsp.EmitOutputStream(strings.NewReader(string(body)))
					}
					rsp.Close()
					return rsp, nil
				}))
			require.NoError(t, err)
			react.config.Timeline.SetTimelineBucketByteSize(-1)
			loop, err := reactloops.CreateLoopByName("infosec_recon", react, reactloops.WithDisableLoopPerception(true), reactloops.WithDisablePeriodicVerification(true))
			require.NoError(t, err)
			for _, name := range []string{"save_evidence", "load_capability"} {
				_, err = loop.GetActionHandler(name)
				require.NoError(t, err, "recon must retain generic loop actions")
			}
			if native {
				_, err = loop.GetActionHandler("adjust_todolist")
				require.NoError(t, err, "recon must retain the native TODO action")
			}
			for _, name := range append(append([]string{}, names...), "tool_compose") {
				_, err = loop.GetActionHandler(name)
				require.Error(t, err, "no recon action may implicitly generate arguments")
			}
			ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
			defer cancel()
			require.NoError(t, loop.Execute("recon-explicit", ctx, "Use the six local recon probes with explicit target parameters."))
			require.EqualValues(t, 6, executions.Load())
			expectedDecisions := 8
			if native {
				expectedDecisions++
			}
			require.EqualValues(t, expectedDecisions, decisions.Load())
			require.Contains(t, react.config.GetSessionEvidenceRendered(), "Six explicit local recon probes completed")
			frozen := aicommon.RenderTimelineFrozenOpen(react.config.Timeline)
			for _, name := range names {
				require.Equal(t, 1, strings.Count(frozen.Open, "[UPSERT] "+name))
				require.Contains(t, frozen.Open, "[REUSE] "+name)
			}
		})
	}
}
