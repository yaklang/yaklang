package liteforge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aid/aicommon"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/lowhttp/poc"
)

// Exercise Execute -> provider callback -> ChatBase projection -> real HTTP SSE.
// The server cannot finish its response until a field callback starts, so this
// catches buffered arguments, late binding, and non-streaming text regressions.
func TestProtocolsProjectAtSendAndStreamBeforeResponseEnds(t *testing.T) {
	for _, native := range []bool{true, false} {
		t.Run(fmt.Sprintf("native_%v", native), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			started, fields := make(chan struct{}), make(chan string, 1)
			bodies, providerErrors := make(chan []byte, 1), make(chan error, 1)
			var tailSent atomic.Bool
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				body, _ := io.ReadAll(req.Body)
				bodies <- body
				w.Header().Set("Content-Type", "text/event-stream")
				write := func(delta any, finish any) {
					frame, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"index": 0, "delta": delta, "finish_reason": finish}}})
					_, _ = fmt.Fprintf(w, "data: %s\n\n", frame)
					w.(http.Flusher).Flush()
				}
				prefix := `{"summary":"hello`
				if native {
					write(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "id": "call_result", "type": "function", "function": map[string]any{"name": "result", "arguments": prefix}}}}, nil)
				} else {
					prefix = `{"@action":"result","summary":"hello`
					write(map[string]any{"content": prefix}, nil)
				}
				select {
				case <-started:
				case <-ctx.Done():
					return
				}
				tailSent.Store(true)
				if native {
					write(map[string]any{"tool_calls": []any{map[string]any{"index": 0, "function": map[string]any{"arguments": ` world","payload":null}`}}}}, nil)
					write(map[string]any{}, "tool_calls")
				} else {
					write(map[string]any{"content": ` world","payload":null}`}, "stop")
				}
				_, _ = fmt.Fprint(w, "data: [DONE]\n\n")
			}))
			defer server.Close()
			schema := `{"type":"object","properties":{"@action":{"const":"result"},"summary":{"type":"string"},"payload":{}},"required":["@action","summary","payload"],"additionalProperties":true}`
			request := Request{ActionName: "result", Schema: schema, Prompt: "输入材料", FieldCallbacks: []FieldCallback{{Keys: []string{"summary"},
				Callback: func(_ string, reader io.Reader, _ *aicommon.Emitter) {
					if tailSent.Load() {
						fields <- "field callback started after response tail"
						return
					}
					close(started)
					data, _ := io.ReadAll(utils.JSONStringReader(reader))
					fields <- string(data)
				}}}}
			options := []aicommon.ConfigOption{aicommon.WithAITransactionAutoRetry(1), aicommon.WithAIAutoRetry(1),
				aicommon.WithAICallback(func(c aicommon.AICallerConfigIf, req *aicommon.AIRequest) (*aicommon.AIResponse, error) {
					wire := aispec.NewDefaultAIConfig(req.GetExtraSpecOpts()...)
					require.Empty(t, wire.Tools, "tools must come from the pre-send projection")
					resp := c.NewAIResponse()
					go func() {
						_, err := aispec.ChatBase(server.URL, "liteforge-stream-test", req.GetPrompt(),
							aispec.WithChatBase_ToolChoice(wire.ToolChoice),
							aispec.WithChatBase_StreamHandler(resp.EmitOutputStream),
							aispec.WithChatBase_ReasonStreamHandler(resp.EmitReasonStream),
							aispec.WithChatBase_ToolCallCallback(wire.ToolCallCallback),
							aispec.WithChatBase_ToolCallArgumentsStreamHandler(wire.ToolCallArgumentsStreamHandler),
							aispec.WithChatBase_FinishReasonCallback(wire.FinishReasonCallback),
							aispec.WithChatBase_RawHTTPResponseHeaderCallback(func(header []byte) {
								if wire.RawHTTPResponseHeaderCallback != nil {
									wire.RawHTTPResponseHeaderCallback(header)
								}
								resp.SetHeaderReady()
							}),
							aispec.WithChatBase_PoCOptions(func() ([]poc.PocConfigOption, error) {
								return []poc.PocConfigOption{poc.WithContext(req.GetContext())}, nil
							}),
						)
						resp.SetError(err)
						resp.Close()
						providerErrors <- err
					}()
					return resp, nil
				})}
			// Native exercises the actual default, including a custom callback.
			if !native {
				options = append(options, aicommon.WithEnableFunctionCallMode(false))
			}
			result, err := Execute(ctx, request, options...)
			require.NoError(t, err)
			require.NoError(t, <-providerErrors)
			require.Equal(t, "hello world", <-fields)
			require.Equal(t, "hello world", result.Action.GetString("summary"))
			require.Equal(t, "result", result.Action.ActionType())
			var sent aispec.ChatMessage
			require.NoError(t, json.Unmarshal(<-bodies, &sent))
			require.Equal(t, "system", sent.Messages[0].Role)
			encodedMessages, err := json.Marshal(sent.Messages)
			require.NoError(t, err)
			require.NotContains(t, string(encodedMessages), "FUNCTION_CALL_ACTION_SCHEMA")
			require.Equal(t, native, len(sent.Tools) == 1)
			if native {
				require.Equal(t, "result", sent.Tools[0].Function.Name)
				require.NotContains(t, fmt.Sprint(sent.Tools[0].Function.Parameters), "@action")
				require.NotContains(t, string(encodedMessages), `"properties"`)
				require.Equal(t, map[string]any{"type": "function", "function": map[string]any{"name": "result"}}, sent.ToolChoice)
			} else {
				require.Nil(t, sent.ToolChoice)
				require.Contains(t, strings.ReplaceAll(string(encodedMessages), `\"`, `"`), `"@action"`)
			}
		})
	}
}
