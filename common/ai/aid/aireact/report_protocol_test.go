package aireact

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aid/aiprojection"
	"github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops"
	_ "github.com/yaklang/yaklang/common/ai/aid/aireact/reactloops/loop_report_generating"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/schema"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// Exercise the actual HTTP boundary, native parser, file edits and finish gate.
// A native parent must remain native in the report child, including old history.
func TestReportSessionProtocolHTTP(t *testing.T) {
	for _, native := range []bool{false, true} {
		for _, streaming := range []bool{false, true} {
			t.Run(fmt.Sprintf("native=%t/stream=%t", native, streaming), func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				defer cancel()
				workdir := t.TempDir()
				referencePath := filepath.Join(workdir, "verified.json")
				reportPath := filepath.Join(workdir, "report.md")
				require.NoError(t, os.WriteFile(referencePath, []byte(`{"evidence":"saved verification"}`), 0600))
				initial := "# 审计报告\n\n原始说明。\n\n" + strings.Repeat("中文𠮷、引号\"、反斜杠\\n。  \n", 1800)
				replacement := "核对后的说明：`code` 和 \"引号\"。"
				inserted := "## 验证结果\n\n资料已读取。\n\n"
				expected := "# 审计报告\n\n" + inserted + strings.TrimPrefix(strings.Replace(initial, "原始说明。", replacement, 1), "# 审计报告\n\n")
				type step struct {
					name string
					args map[string]any
					body string
				}
				steps := []step{
					{"read_reference_file", map[string]any{"file_path": referencePath}, ""},
					{"write_section", nil, initial},
					{"modify_section", map[string]any{"old_snippet": "原始说明。"}, replacement},
					{"insert_section", map[string]any{"insert_line": 3}, inserted},
					{"finish", nil, ""},
				}
				var calls atomic.Int32
				requests := make(chan aispec.ChatMessage, 12)
				position := 0 // Only the synchronous loop sends requests to this server.
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					var request aispec.ChatMessage
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						http.Error(w, err.Error(), http.StatusBadRequest)
						return
					}
					requests <- request
					calls.Add(1)
					if position >= len(steps) {
						http.Error(w, "unexpected retry", http.StatusBadRequest)
						return
					}
					count := 1
					if native && position == 2 {
						count = 2
					} // Different bodies in one native batch.
					var toolCalls []any
					content := ""
					for n := 0; n < count; n++ {
						current := steps[position]
						args := map[string]any{"identifier": fmt.Sprintf("report_step_%d", position)}
						for key, value := range current.args {
							args[key] = value
						}
						if native {
							if current.body != "" {
								args["content"] = current.body
							}
							encoded, _ := json.Marshal(args)
							toolCalls = append(toolCalls, map[string]any{"id": fmt.Sprintf("report_call_%d", position), "type": "function", "function": map[string]any{"name": current.name, "arguments": string(encoded)}})
						} else {
							args["@action"] = current.name
							encoded, _ := json.Marshal(args)
							content = string(encoded)
							if current.body != "" {
								content += "\n<|GEN_REPORT_CURRENT_NONCE|>\n" + current.body + "\n<|GEN_REPORT_END_CURRENT_NONCE|>"
							}
						}
						position++
					}
					message := map[string]any{"role": "assistant", "content": content}
					finish := "stop"
					if native {
						message["tool_calls"] = toolCalls
						finish = "tool_calls"
					}
					if streaming {
						w.Header().Set("Content-Type", "text/event-stream")
						send := func(delta any, reason any) {
							raw, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": reason}}})
							fmt.Fprintf(w, "data: %s\n\n", raw)
							w.(http.Flusher).Flush()
						}
						chunks := func(value string, emit func(string)) {
							chars := []rune(value)
							for len(chars) > 0 {
								n := min(257, len(chars))
								emit(string(chars[:n]))
								chars = chars[n:]
							}
						}
						if native {
							for i, value := range toolCalls {
								call := value.(map[string]any)
								function := call["function"].(map[string]any)
								send(map[string]any{"tool_calls": []any{map[string]any{"index": i, "id": call["id"], "type": "function", "function": map[string]any{"name": function["name"], "arguments": ""}}}}, nil)
								chunks(function["arguments"].(string), func(chunk string) {
									send(map[string]any{"tool_calls": []any{map[string]any{"index": i, "function": map[string]any{"arguments": chunk}}}}, nil)
								})
							}
						} else {
							chunks(content, func(chunk string) { send(map[string]any{"content": chunk}, nil) })
						}
						send(map[string]any{}, finish)
						fmt.Fprint(w, "data: [DONE]\n\n")
						return
					}
					w.Header().Set("Content-Type", "application/json")
					_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": message, "finish_reason": finish}}})
				}))
				defer server.Close()
				var finishes atomic.Int32
				react, err := NewTestReAct(aicommon.WithWorkdir(workdir),
					aicommon.WithAICallback(func(cfg aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
						wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
						if (wire.ToolCallCallback != nil) != native {
							return nil, fmt.Errorf("report changed session protocol")
						}
						output, err := aispec.ChatBase(server.URL, "local-report-test", req.GetPrompt(),
							aispec.WithChatBase_DisableStream(!streaming), aispec.WithChatBase_Tools(wire.Tools),
							aispec.WithChatBase_ToolChoice(wire.ToolChoice), aispec.WithChatBase_ToolCallCallback(wire.ToolCallCallback),
							aispec.WithChatBase_ToolCallArgumentsStreamHandler(wire.ToolCallArgumentsStreamHandler),
							aispec.WithChatBase_FinishReasonCallback(wire.FinishReasonCallback),
							aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) { return nil, nil }))
						if err != nil {
							return nil, err
						}
						resp := cfg.NewAIResponse()
						resp.EmitOutputStream(strings.NewReader(output))
						resp.Close()
						return resp, nil
					}), aicommon.WithEnableFunctionCallMode(native),
					aicommon.WithEventHandler(func(e *schema.AiOutputEvent) {
						if e.Type == schema.EVENT_TYPE_REPORT_FINISH {
							finishes.Add(1)
						}
					}))
				require.NoError(t, err)
				if native {
					for _, id := range []string{"parent_a", "parent_b"} {
						history, err := aiprojection.CreateActionResponse(json.RawMessage(fmt.Sprintf(`[{"role":"assistant","content":"","tool_calls":[{"id":%q,"type":"function","function":{"name":"read_file","arguments":"{}"}}]},{"role":"tool","tool_call_id":%q,"content":"PARENT_EVIDENCE"}]`, id, id)))
						require.NoError(t, err)
						react.AddToTimelineWithPromptProjection("FUNCTION_CALL_ACTION_RESPONSE", "PARENT_EVIDENCE", history)
					}
				}
				loop, err := reactloops.CreateLoopByName(schema.AI_REACT_LOOP_NAME_REPORT_GENERATING, react,
					reactloops.WithAllowToolCall(false), reactloops.WithAllowRAG(false), reactloops.WithAllowAIForge(false),
					reactloops.WithAllowPlanAndExec(false), reactloops.WithAllowUserInteract(false),
					reactloops.WithDisableLoopPerception(true), reactloops.WithDisablePeriodicVerification(true),
					reactloops.WithDisableIncreaseIteration(true), reactloops.WithMaxIterations(8),
					reactloops.WithInitTask(func(l *reactloops.ReActLoop, _ aicommon.AIStatefulTask, op *reactloops.InitTaskOperator) {
						l.Set("report_filename", reportPath)
						l.Set("user_requirements", "读取证据并编写报告")
						op.Continue()
					}))
				require.NoError(t, err)
				defer loop.Release()
				require.Equal(t, native, loop.FunctionCallModeEnabled())
				task := aicommon.NewStatefulTaskBase("report-protocol", "Generate audit report", ctx, react.config.GetEmitter(), true)
				react.SetCurrentTask(task)
				require.NoError(t, loop.ExecuteWithExistedTask(task))
				react.config.GetEmitter().WaitForStream()
				require.Equal(t, "true", loop.Get("report_finished"))
				require.EqualValues(t, 1, finishes.Load())
				written, err := os.ReadFile(reportPath)
				require.NoError(t, err)
				require.Equal(t, expected, string(written))
				wantCalls := 5
				if native {
					wantCalls = 4
				}
				require.EqualValues(t, wantCalls, calls.Load())
				close(requests)
				var firstTools []aispec.Tool
				requestIndex := 0
				for request := range requests {
					requestIndex++
					if !native {
						require.Empty(t, request.Tools)
						require.Contains(t, fmt.Sprint(request.Messages), `"@action": "write_section"`)
						require.Contains(t, fmt.Sprint(request.Messages), "<|GEN_REPORT_")
						for _, message := range request.Messages {
							require.Empty(t, message.ToolCalls)
							require.NotEqual(t, "tool", message.Role)
						}
						continue
					}
					require.NotEmpty(t, request.Tools)
					toolJSON, err := json.Marshal(request.Tools)
					require.NoError(t, err)
					require.NotContains(t, string(toolJSON), "AITAG", "native tool definitions must not describe text-tag transport")
					require.NotContains(t, string(toolJSON), "GEN_REPORT")
					if firstTools == nil {
						firstTools = request.Tools
						contentActions := 0
						for _, tool := range firstTools {
							switch tool.Function.Name {
							case "write_section", "modify_section", "insert_section":
								encoded, err := json.Marshal(tool.Function.Parameters)
								require.NoError(t, err)
								var parameters map[string]any
								require.NoError(t, json.Unmarshal(encoded, &parameters))
								require.Contains(t, parameters["properties"], "content")
								require.Contains(t, parameters["required"], "content")
								contentActions++
							}
						}
						require.Equal(t, 3, contentActions)
					} else {
						require.Equal(t, firstTools, request.Tools, "report tools must remain stable across edits")
					}
					parents := map[string]bool{}
					for _, message := range request.Messages {
						require.NotContains(t, fmt.Sprint(message.Content), "GEN_REPORT", "native report prompts must not request text-tag edits")
						require.NotContains(t, fmt.Sprint(message.Content), "AITAG", "native prompts must omit text-tag protocol instructions")
						for _, call := range message.ToolCalls {
							parents[call.ID] = true
						}
					}
					rawRequest, _ := json.Marshal(request)
					require.True(t, parents["parent_a"], "request=%d, native IDs=%v, parent evidence=%t, replay tag=%t", requestIndex, parents, strings.Contains(string(rawRequest), "PARENT_EVIDENCE"), strings.Contains(string(rawRequest), "FUNCTION_CALL_ACTION_RESPONSE"))
					require.True(t, parents["parent_b"], "old native calls must not be rewritten as user text")
				}
			})
		}
	}
}
