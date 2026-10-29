package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	"github.com/yaklang/yaklang/common/ai/aid/aitool"
	"github.com/yaklang/yaklang/common/ai/aispec"
)

func compressionHistoryIDFromPrompt(t *testing.T, prompt, marker string) string {
	t.Helper()
	index := strings.Index(prompt, marker)
	require.GreaterOrEqual(t, index, 0, "the selected record must be visible to the model")
	matches := regexp.MustCompile(`\[item_id:(\d+)\]`).FindAllStringSubmatch(prompt[:index], -1)
	require.NotEmpty(t, matches, "the model must be able to read the selected record's ID")
	return matches[len(matches)-1][1]
}

// compress_context is a default main-loop action. An ordinary turn built by
// the real prompt manager advertises it, and a model-chosen call compacts the
// live timeline before the loop continues.
func TestReAct_CompressContextAvailableOnNormalTurn(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			var decisions atomic.Int32
			var secondPrompt atomic.Value
			oldHistory, keptHistory, summary := uuid.NewString(), uuid.NewString(), uuid.NewString()
			lateHistory := uuid.NewString()
			reservedHistory := uuid.NewString()
			var reservedID int64
			var react *ReAct
			var err error
			react, err = NewTestReAct(
				aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithEnableFunctionCallMode(native),
				aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
						response := cfg.NewAIResponse()
						response.EmitOutputStream(strings.NewReader(`{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"done"}`))
						response.Close()
						return response, nil
					}
					if !aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
						return nil, fmt.Errorf("unexpected auxiliary request %q", req.GetCallerLabel())
					}
					require.NotEqual(t, aicommon.CallerLabelContextCompress, req.GetCallerLabel(),
						"the prompt is under the limit, so this is an ordinary turn")
					options := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
					if native {
						var names []string
						for _, tool := range options.Tools {
							names = append(names, tool.Function.Name)
						}
						require.Contains(t, names, aicommon.ContextCompressionAction)
					} else {
						require.Contains(t, req.GetPrompt(), aicommon.ContextCompressionAction)
						require.Contains(t, req.GetPrompt(), "retain_timeline")
					}
					require.NotContains(t, req.GetPrompt(), "# 本轮只压缩上下文",
						"an ordinary turn keeps the normal dynamic section")

					name, args := "finish", `{}`
					switch decisions.Add(1) {
					case 1:
						name = aicommon.ContextCompressionAction
						params, _ := json.Marshal(map[string]any{
							"summary":             summary,
							"retain_timeline":     compressionHistoryIDFromPrompt(t, req.GetPrompt(), keptHistory),
							"retire_skill_names":  []string{},
							"retire_evidence_ids": []string{},
						})
						args = string(params)
						require.NotContains(t, req.GetPrompt(), lateHistory)
						react.config.Timeline.PushText(react.config.AcquireId(), "%s", lateHistory)
						react.config.Timeline.PushToolResult(&aitool.ToolResult{ID: reservedID, Name: "delayed-tool", Success: true, Data: reservedHistory})
					case 2:
						secondPrompt.Store(req.GetPrompt())
					default:
						return nil, fmt.Errorf("unexpected decision %d", decisions.Load())
					}
					response := cfg.NewAIResponse()
					if native {
						options.ToolCallCallback([]*aispec.ToolCall{{ID: fmt.Sprintf("call_%d", decisions.Load()), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: args}}})
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
			tl := react.config.Timeline
			tl.PushText(react.config.AcquireId(), "%s", oldHistory)
			reservedID = react.config.AcquireId()
			tl.PushText(react.config.AcquireId(), "%s", keptHistory)

			loop, err := reactloops.NewReActLoop("compress-context", react,
				reactloops.WithFunctionCallMode(native), reactloops.WithDisablePeriodicVerification(true),
				reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(4))
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			require.NoError(t, loop.Execute("compress-context-task", ctx, "Compact the history, then finish."))

			require.EqualValues(t, 2, decisions.Load(), "the loop continues after the compression action")
			view := aicommon.RenderTimelineFrozenOpen(tl).PromptText()
			require.Contains(t, view, summary)
			require.Contains(t, view, keptHistory)
			require.NotContains(t, view, oldHistory)
			prompt, _ := secondPrompt.Load().(string)
			require.Contains(t, prompt, summary)
			require.Contains(t, prompt, keptHistory)
			require.Contains(t, prompt, lateHistory, "an ordinary compression action must preserve unseen results")
			require.Contains(t, prompt, reservedHistory, "reserved IDs below the request watermark must also survive")
			require.Equal(t, 1, strings.Count(prompt, summary), "maintenance arguments must not replay a duplicate summary")
			require.NotContains(t, prompt, oldHistory)
		})
	}
}

// When the real assembled prompt exceeds the limit, the loop switches only the
// dynamic section, requires compress_context on that turn, and resumes normal
// turns after the action applies.
func TestReAct_CompressContextOnPromptOverflow(t *testing.T) {
	for _, native := range []bool{false, true} {
		t.Run(fmt.Sprintf("native=%t", native), func(t *testing.T) {
			var compressionTurns, normalTurns atomic.Int32
			var compressionPrompt, normalPrompt atomic.Value
			var history []string
			summary := uuid.NewString()
			lateHistory := uuid.NewString()
			reservedHistory := uuid.NewString()
			var reservedID int64
			var react *ReAct
			respond := func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest, name, args string) (*aicommon.AIResponse, error) {
				response := cfg.NewAIResponse()
				if native {
					options := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
					options.ToolCallCallback([]*aispec.ToolCall{{ID: uuid.NewString(), Type: "function", Function: aispec.FuncReturn{Name: name, Arguments: args}}})
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
			}
			var err error
			react, err = NewTestReAct(
				aicommon.WithWorkdir(t.TempDir()), aicommon.WithDisableCreateDBRuntime(true),
				aicommon.WithEnableFunctionCallMode(native), aicommon.WithTimelineContentLimit(60*1024),
				aicommon.WithAIAutoRetry(1), aicommon.WithAITransactionAutoRetry(1),
				aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					if req.GetCallerLabel() == aicommon.CallerLabelContextCompress {
						require.Equal(t, int32(1), compressionTurns.Add(1), "one compression restores the prompt below the limit")
						compressionPrompt.Store(req.GetPrompt())
						_, instructions, found := strings.Cut(req.GetPrompt(), "# 压缩 Timeline")
						require.True(t, found)
						require.NotContains(t, instructions, "{{", "compression instructions must be rendered before assembly")
						if native {
							require.NotContains(t, instructions, "@action", "native compression must only describe native calls")
						} else {
							require.Contains(t, instructions, `"@action":"compress_context"`)
						}
						options := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
						require.Equal(t, "none", options.ThinkingLevel, "compression turns do not spend model thinking")
						if native {
							var names []string
							for _, tool := range options.Tools {
								names = append(names, tool.Function.Name)
							}
							require.Contains(t, names, aicommon.ContextCompressionAction)
							require.Contains(t, names, "finish", "the full tool list stays in the request")
						}
						params, err := json.Marshal(map[string]any{
							"summary": summary, "retain_timeline": compressionHistoryIDFromPrompt(t, req.GetPrompt(), history[len(history)-1]),
							"retire_skill_names": []string{}, "retire_evidence_ids": []string{},
						})
						if err != nil {
							return nil, err
						}
						require.NotContains(t, req.GetPrompt(), lateHistory)
						react.config.Timeline.PushText(react.config.AcquireId(), "%s", lateHistory)
						react.config.Timeline.PushToolResult(&aitool.ToolResult{ID: reservedID, Name: "delayed-tool", Success: true, Data: reservedHistory})
						return respond(cfg, req, aicommon.ContextCompressionAction, string(params))
					}
					if aicommon.IsVerifySatisfactionPrompt(req.GetPrompt()) {
						response := cfg.NewAIResponse()
						response.EmitOutputStream(strings.NewReader(`{"@action":"verify-satisfaction","user_satisfied":true,"reasoning":"done"}`))
						response.Close()
						return response, nil
					}
					if !aicommon.IsPrimaryDecisionPrompt(req.GetPrompt()) {
						return nil, fmt.Errorf("unexpected auxiliary request %q", req.GetCallerLabel())
					}
					require.Equal(t, int32(1), compressionTurns.Load(), "an oversized prompt must not reach the normal model")
					require.Equal(t, int32(1), normalTurns.Add(1))
					normalPrompt.Store(req.GetPrompt())
					return respond(cfg, req, "finish", `{}`)
				}),
			)
			require.NoError(t, err)
			reservedID = react.config.AcquireId()
			for i := 0; i < 4; i++ {
				marker := uuid.NewString()
				id := react.config.AcquireId()
				history = append(history, marker)
				react.config.Timeline.PushText(id, "%s", marker+" "+strings.Repeat(uuid.NewString()+" ", 400))
			}
			loop, err := reactloops.NewReActLoop("compress-context-overflow", react,
				reactloops.WithFunctionCallMode(native), reactloops.WithDisablePeriodicVerification(true),
				reactloops.WithDisableLoopPerception(true), reactloops.WithMaxIterations(4))
			require.NoError(t, err)
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			require.NoError(t, loop.Execute("compress-context-overflow-task", ctx, "Continue after the context is compacted."))

			require.EqualValues(t, 1, compressionTurns.Load())
			require.EqualValues(t, 1, normalTurns.Load())
			compressed, _ := compressionPrompt.Load().(string)
			normal, _ := normalPrompt.Load().(string)
			require.Contains(t, compressed, "# 本轮只压缩上下文")
			for _, marker := range history {
				require.Contains(t, compressed, marker, "the compression turn sees the real history in the prefix")
			}
			require.Greater(t, aicommon.MeasureTokens(compressed), 60*1024)
			require.Less(t, aicommon.MeasureTokens(normal), 60*1024)
			require.NotContains(t, normal, "# 本轮只压缩上下文")
			require.Contains(t, normal, summary)
			require.Contains(t, normal, lateHistory, "a forced compression turn must preserve unseen results")
			require.Contains(t, normal, reservedHistory, "a forced compression turn must preserve late reserved IDs")
			require.Contains(t, normal, history[len(history)-1])
			for _, marker := range history[:len(history)-1] {
				require.NotContains(t, normal, marker)
			}
		})
	}
}
