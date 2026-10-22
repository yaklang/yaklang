package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/coordinator"
	"github.com/yaklang/yaklang/common/ai/aid/interaction"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/ai/ytoken"
)

// Assert against the actual outbound model prompt in both protocols. Loading
// must not smuggle definitions into replay or Timeline, even after freezing.
func TestMainLoopUISchemasOnDemand(t *testing.T) {
	raw, err := os.ReadFile("../interaction/testdata/catalog.json")
	require.NoError(t, err)
	var catalog interaction.Catalog
	require.NoError(t, json.Unmarshal(raw, &catalog))
	caps, _ := json.Marshal(aicommon.UIClientCapabilities{Protocol: interaction.Protocol, Catalog: raw})
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%v", native), func(t *testing.T) {
			var steps atomic.Int32
			var react *ReAct
			react, err = NewTestReAct(aicommon.WithWorkdir(t.TempDir()), aicommon.WithClientUI(string(caps)),
				aicommon.WithEnableFunctionCallMode(native), aicommon.WithAgreeYOLO(),
				aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					prompt := req.GetPrompt()
					if !aicommon.IsPrimaryDecisionPrompt(prompt) {
						return nil, fmt.Errorf("unexpected auxiliary UI model request: %.200s", prompt)
					}
					step := steps.Add(1)
					name, args := "load_ui_components", `{"names":["BarChart","BarChart"]}`
					require.NotContains(t, prompt, catalog.Definition("LineChart").Description)
					switch step {
					case 1:
						require.Contains(t, prompt, "search_ui_components")
						require.NotContains(t, prompt, "<UI_COMPONENT_DEFINITIONS>")
						require.NotContains(t, prompt, catalog.Definition("BarChart").Description)
						if native {
							options := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
							var uiTools []aispec.Tool
							for _, tool := range options.Tools {
								if tool.Function.Name == "load_ui_components" || tool.Function.Name == "search_ui_components" || tool.Function.Name == "render_ui" {
									uiTools = append(uiTools, tool)
								}
							}
							require.Len(t, uiTools, 3)
							toolsJSON, _ := json.Marshal(uiTools)
							start := strings.Index(prompt, "需要结构化展示时")
							require.GreaterOrEqual(t, start, 0)
							indexStart := start + strings.Index(prompt[start:], "有界组件索引")
							end := indexStart + strings.Index(prompt[indexStart:], "\n")
							fixedTokens := ytoken.CalcTokenCount(prompt[start:end]) + ytoken.CalcTokenCount(string(toolsJSON))
							require.LessOrEqual(t, fixedTokens, 1024, "UI-specific constant instructions and all three tools must stay small")
							t.Logf("UI resident instructions + name index + 3 native tools: %d Qwen BPE tokens", fixedTokens)
						}
					case 2, 3:
						require.Contains(t, prompt, catalog.Definition("BarChart").Description)
						require.Equal(t, 1, strings.Count(prompt, `<UI_COMPONENT_DEFINITIONS>`))
						require.Equal(t, 1, strings.Count(prompt, `"name":"BarChart"`))
						if step == 3 {
							args = `{"names":["Text"]}`
							react.config.Timeline.FreezeAll()
						}
					case 4:
						require.NotContains(t, prompt, catalog.Definition("BarChart").Description)
						require.Contains(t, prompt, catalog.Definition("Text").Description)
						name = "render_ui"
						args = `{"ui":{"id":"probe","catalogVersion":"1.2","fallback":"Verified UI","root":"text","nodes":[{"id":"text","component":"Text","props":{"text":"Verified UI"}}]}}`
					case 5:
						require.Contains(t, prompt, "rendered")
						args = `{"names":[]}`
					case 6:
						require.NotContains(t, prompt, "<UI_COMPONENT_DEFINITIONS>")
						require.NotContains(t, prompt, catalog.Definition("Text").Description)
						name, args = "finish", `{}`
					default:
						return nil, fmt.Errorf("unexpected UI step %d", step)
					}
					timeline := aicommon.RenderTimelineFrozenOpen(react.config.Timeline)
					require.NotContains(t, timeline.Open+timeline.Frozen, "<UI_COMPONENT_DEFINITIONS>")
					require.NotContains(t, timeline.Open+timeline.Frozen, catalog.Definition("BarChart").Description)
					response := cfg.NewAIResponse()
					if native {
						options := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
						options.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("ui_%d", step), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: args}}})
						options.FinishReasonCallback("tool_calls", nil)
					} else {
						var params map[string]any
						require.NoError(t, json.Unmarshal([]byte(args), &params))
						params["@action"] = name
						body, _ := json.Marshal(params)
						response.EmitOutputStream(strings.NewReader(string(body)))
					}
					response.Close()
					return response, nil
				}))
			require.NoError(t, err)
			loop, err := reactloops.CreateLoopByName("default", react, reactloops.WithDisablePeriodicVerification(true),
				reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(8))
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			require.NoError(t, loop.Execute("ui-schema-probe", ctx, "Load component definitions, verify replacement, render a text block and clear the definitions."))
			require.EqualValues(t, 6, steps.Load())

			// Coordinator and workers must not filter out the newly required loader.
			for _, factory := range []func(aicommon.AIInvokeRuntime, ...reactloops.ReActLoopOption) (*reactloops.ReActLoop, error){coordinator.NewLoop, coordinator.NewWorkerLoop} {
				owner := coordinator.New(ctx, nil, 1)
				defer owner.Close()
				child, err := factory(react, coordinator.WithController(owner))
				require.NoError(t, err)
				for _, action := range []string{"load_ui_components", "search_ui_components", "render_ui"} {
					_, err := child.GetActionHandler(action)
					require.NoError(t, err)
				}
				child.Release()
			}
		})
	}
}
