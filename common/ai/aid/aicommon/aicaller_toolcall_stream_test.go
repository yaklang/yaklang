package aicommon

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/ai/aispec"
	"github.com/yaklang/yaklang/common/utils"
)

func TestAIChatToAICallbackType_PreservesToolCallArguments(t *testing.T) {
	for _, tc := range []struct {
		name   string
		raw    string
		action string
	}{
		{
			name:   "formatted JSON with newline before array",
			raw:    "{\r\n\t\"@action\": \"search_logs\",\n\t\"queries\":\n\t[\"ERROR\", \"WARN\"],\n\t\"offset\": 0\n}",
			action: "search_logs",
		},
		{
			// The tolerant action parser also accepts unescaped control bytes.
			name:   "literal multiline argument",
			raw:    "{\"@action\":\"directly_answer\",\"answer_payload\":\"line1\n\tline2\r\nline3\"}",
			action: "directly_answer",
		},
		{
			name:   "escaped multiline argument",
			raw:    `{"@action":"directly_answer","answer_payload":"line1\n\tline2\r\nline3"}`,
			action: "directly_answer",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			cfg := NewTestConfig(ctx)
			cb := AIChatToAICallbackType(func(_ string, opts ...aispec.AIConfigOption) (string, error) {
				ac := aispec.NewDefaultAIConfig(opts...)
				if ac.ToolCallArgumentsStreamHandler == nil {
					return "", io.ErrUnexpectedEOF
				}
				ac.ToolCallArgumentsStreamHandler(iotest.OneByteReader(strings.NewReader(tc.raw)))
				return "", nil
			})
			resp, err := cb(cfg, NewAIRequest("test", WithAIRequest_EnableToolCallArgumentsStream()))
			require.NoError(t, err)
			got, err := io.ReadAll(resp.GetUnboundStreamReader(false))
			require.NoError(t, err)
			require.NoError(t, resp.GetError())
			require.Equal(t, tc.raw, string(got), "the adapter must preserve tool argument bytes")

			action, err := ExtractValidActionFromStream(ctx, strings.NewReader(string(got)), tc.action)
			require.NoError(t, err)
			if tc.action == "search_logs" {
				require.Equal(t, []string{"ERROR", "WARN"}, action.GetStringSlice("queries"))
			} else {
				require.Equal(t, "line1\n\tline2\r\nline3", action.GetString("answer_payload"))
			}
		})
	}
}

func TestAIChatToAICallbackType_SSEToolCallWhitespace(t *testing.T) {
	// Outer SSE JSON escapes argument fragments; after decoding it, structural
	// whitespace is literal but escapes inside argument strings must remain.
	raw := " \r\n{\t\"@action\" \t:\r\n \"search_logs\",\n\"queries\" :\r\n\t[\"error code\", \"WARN\"],\n\"pattern\" : \"line1\\n\\tline2\"\n}\r\n "
	require.True(t, json.Valid([]byte(raw)))
	var sse strings.Builder
	sse.WriteString("HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\n")
	for _, ch := range raw {
		// One character per delta splits inner JSON escape sequences across SSE
		// events and includes events whose entire arguments fragment is whitespace.
		payload, err := json.Marshal(map[string]any{
			"choices": []any{map[string]any{"delta": map[string]any{"tool_calls": []any{map[string]any{
				"index": 0, "id": "call_whitespace", "type": "function",
				"function": map[string]any{"name": "execute_action", "arguments": string(ch)},
			}}}}},
		})
		require.NoError(t, err)
		sse.WriteString("data: " + string(payload) + "\n\n")
	}
	sse.WriteString("data: [DONE]\n\n")
	host, port := utils.DebugMockHTTP([]byte(sse.String()))
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cb := AIChatToAICallbackType(func(prompt string, opts ...aispec.AIConfigOption) (string, error) {
		ac := aispec.NewDefaultAIConfig(opts...)
		return aispec.ChatBase("http://example.com/v1/chat/completions", "test-model", prompt,
			aispecWithMockServer(host, port),
			aispec.WithChatBase_ToolCallCallback(func([]*aispec.ToolCall) {}),
			aispec.WithChatBase_ToolCallArgumentsStreamHandler(ac.ToolCallArgumentsStreamHandler),
		)
	})
	resp, err := cb(NewTestConfig(ctx), NewAIRequest("test", WithAIRequest_EnableToolCallArgumentsStream()))
	require.NoError(t, err)
	var received bytes.Buffer
	action, err := ExtractValidActionFromStream(ctx, io.TeeReader(resp.GetUnboundStreamReader(false), &received), "search_logs")
	require.NoError(t, err)
	require.NoError(t, resp.GetError())
	require.Equal(t, raw, received.String())
	require.Equal(t, []string{"error code", "WARN"}, action.GetStringSlice("queries"))
	require.Equal(t, "line1\n\tline2", action.GetString("pattern"))
}
